# Pare-feu WAF

Un pare-feu applicatif web basé sur [Coraza](https://coraza.io/) et OWASP CRS v4, rattachable aux sites des [Services Web](/fr/docs/modules/web).

## Fonctions

- Jeu de règles OWASP CRS v4 intégré, dont les fichiers de règles s'activent individuellement ;
- Règles SecLang personnalisées ;
- Les groupes de règles peuvent être réordonnés ; les journaux de détection enregistrent l'IP du client, le domaine, le chemin et l'ID de règle, filtrables par domaine, IP ou mots-clés ;
- Mode blocage et mode détection seule ;
- Limite d'inspection du corps de requête réglable : au-delà, la requête est rejetée ou seul le début est inspecté ; l'inspection du corps peut être ignorée par chemin ;
- Mode de compatibilité WebDAV : autorise les méthodes, les en-têtes et tout type de contenu d'envoi utilisés par les clouds personnels et WebDAV ;
- Peut utiliser un moteur de détection SafeLine auto-hébergé, seul ou avec des groupes de règles ;
- Plusieurs instances, référencées par les sites selon les besoins.

## Recommandations

1. Pour un nouveau site, commencez par « Détection seule » et surveillez les journaux pendant un certain temps pour vous assurer qu'il n'y a pas de faux positifs ;
2. Ajustez les fichiers de règles ou ajoutez des exceptions personnalisées en cas de faux positifs ;
3. Passez ensuite en « Blocage ».

## Ordre des groupes de règles

Les groupes de règles d'une instance se chargent de haut en bas ; les boutons monter et descendre permettent de les réordonner. Dans une même phase, les règles placées avant s'exécutent d'abord :

- Les règles de blocage (deny, etc.) fonctionnent à n'importe quelle position ;
- Les règles d'autorisation ou d'exclusion à l'exécution (`allow`, `ctl:ruleRemoveById`, `ctl:ruleRemoveTargetById`, etc.) doivent être au-dessus du groupe OWASP CRS, sinon CRS peut bloquer la requête avant ;
- Les directives de configuration comme `SecRuleRemoveById` et `SecRuleUpdateTargetById` n'agissent que sur les règles déjà chargées : placez-les sous le groupe CRS, au-dessus elles sont sans effet.

## Sites recevant de gros fichiers

Par défaut, le WAF n'inspecte que les corps de requête jusqu'à 12,5 Mo et rejette les requêtes plus volumineuses (413) : les clouds personnels, WebDAV ou gestionnaires de fichiers ne peuvent alors pas recevoir de gros fichiers. Réglez-le dans « Inspection du corps de requête » de l'instance :

- **Limite d'inspection** : 0 correspond à la valeur par défaut de 12,5 Mo, jusqu'à 1024 Mo ;
- **Au-delà de la limite** : « Rejeter la requête » conserve le comportement précédent ; « Inspecter uniquement le début » n'inspecte que la partie sous la limite et transmet le reste tel quel au backend, ce qui convient aux sites recevant de gros envois ;
- **Compatible WebDAV et envois** : par défaut, OWASP CRS n'autorise que GET, HEAD, POST et OPTIONS, et bloque les en-têtes `Lock-Token`, `If` et `Content-Range` ainsi que les types de contenu hors de sa liste autorisée ; les clients WebDAV ne peuvent donc ni lister les dossiers, ni envoyer, ni verrouiller des fichiers. Une fois l'option activée, PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK et d'autres méthodes, ainsi que ces en-têtes, sont autorisés, et le type de contenu des envois n'est plus restreint ; le corps des requêtes PUT et `application/octet-stream` est traité comme contenu de fichier et n'est plus inspecté comme paramètres de formulaire. Les contrôles d'injection, de traversée de chemin et des autres attaques restent actifs ;
- **Chemins sans inspection du corps** : un préfixe de chemin par ligne (par exemple `/api/upload`, `/remote.php/dav/`). Le corps des requêtes correspondantes n'est pas inspecté ; les en-têtes, l'URL et les paramètres restent inspectés normalement.

Pour les clouds personnels et les sites WebDAV, créez de préférence une instance dédiée : activez « Compatible WebDAV et envois », choisissez « Inspecter uniquement le début » et, si nécessaire, ajoutez les points d'envoi aux chemins sans inspection ; les autres sites gardent les réglages par défaut.

curl, rclone et d'autres clients envoient l'en-tête `Expect: 100-continue` pour les envois volumineux, en-tête que CRS 4 bloque par défaut. Toutes les instances de Gately autorisent cet en-tête (Gately gère lui-même le 100-continue) : aucun réglage supplémentaire n'est nécessaire.

## Utiliser SafeLine

[SafeLine](https://waf.chaitin.com/) est un pare-feu applicatif web de Chaitin Tech qui détecte les attaques grâce à un moteur d’analyse sémantique et produit peu de faux positifs. Gately peut envoyer les requêtes à votre moteur de détection SafeLine auto-hébergé (safeline-detector) et les autoriser ou les bloquer selon le résultat ; les points d’entrée, les certificats, l’authentification et les bannissements restent gérés par Gately, SafeLine n’a donc plus besoin d’être une couche de proxy inverse supplémentaire.

1. Installez SafeLine Community Edition en suivant sa documentation officielle ; le répertoire d’installation par défaut est `/data/safeline`.
2. Choisissez comment vous connecter au moteur de détection :
   - **Socket Unix** (Gately et SafeLine sur la même machine) : le moteur écoute par défaut sur `/data/safeline/resources/detector/snserver.sock`, utilisez donc l’adresse `unix:/data/safeline/resources/detector/snserver.sock`. Si Gately tourne dans Docker, montez le répertoire `/data/safeline/resources/detector` dans le conteneur Gately et utilisez le chemin dans le conteneur.
   - **TCP** : ajoutez les deux lignes ci-dessous à `/data/safeline/resources/detector/detector.yml`, puis exécutez `docker restart safeline-detector`. Une fois passé en TCP, le moteur n’écoute plus sur le socket Unix.

     ```yaml
     bind_addr: 0.0.0.0
     listen_port: 8000
     ```

     Utilisez comme adresse l’IP et le port du conteneur safeline-detector, par exemple `172.22.222.5:8000` ; `docker inspect safeline-detector | grep IPAddress` affiche l’IP du conteneur. Si Gately tourne sur un réseau bridge Docker, connectez-le au réseau de SafeLine avec `docker network connect safeline-ce <nom du conteneur Gately>`. Le moteur de détection n’a pas d’authentification : n’exposez pas le port 8000 sur Internet.
3. Dans l’instance WAF, activez « Utiliser SafeLine », saisissez l’adresse et cliquez sur « Tester la connexion ». Gately envoie un battement puis une requête d’injection SQL d’exemple à inspecter ; « l’attaque d’exemple a été détectée » signifie que le moteur fonctionne. Cette attaque d’exemple apparaît dans l’historique des attaques de la console SafeLine avec la source 127.0.0.1 et le domaine gately-test.invalid.
4. Référencez l’instance depuis un site du Service Web.

Remarques :

- Avec SafeLine connecté, les groupes de règles peuvent rester vides ; si vous les conservez, les requêtes passent d’abord par SafeLine puis par les groupes de règles. En mode « Détection seule », les attaques détectées par SafeLine sont seulement journalisées et la requête continue.
- Requêtes bloquées : les navigateurs voient une page de blocage avec l’ID d’événement, les autres clients reçoivent le même JSON que les plugins officiels de SafeLine (avec `event_id`). Le code d’état est celui indiqué par le moteur, généralement 403. Les blocages sont comptés dans les journaux d’accès (`deny=waf`), les statistiques du site et les événements de blocage du WAF, et le préréglage « Blocages du WAF » de la Protection contre les intrusions s’applique aussi.
- Les attaques sont également enregistrées dans la console SafeLine et peuvent être retrouvées par ID d’événement ; ces enregistrements n’appartiennent à aucun site SafeLine.
- Par défaut, jusqu’à 1024 Ko du corps de la requête sont envoyés pour inspection ; le reste n’est pas inspecté et est transmis tel quel au backend. « Chemins sans inspection du corps » et « Compatible WebDAV et envois » s’appliquent aussi à SafeLine ; les corps gRPC de longueur inconnue transmis en HTTP/2 ne sont pas inspectés.
- Le délai de détection est de 1 seconde par défaut. Si le moteur est indisponible (échec de connexion ou délai dépassé), les requêtes sont autorisées par défaut, l’instance affiche « SafeLine indisponible » et une erreur est journalisée ; avec « Rejeter les requêtes », ces requêtes reçoivent 503. Juste après son démarrage, le moteur peut renvoyer pendant quelques secondes des résultats non reconnus, traités de la même façon.
- Seules les requêtes sont inspectées ; les réponses ne sont pas envoyées à SafeLine.

## Tests

Le WAF est testé avec de vraies charges d'attaque, qui couvrent le blocage de la traversée de chemin et d'attaques similaires.
