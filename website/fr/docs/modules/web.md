# Services Web

Servez un proxy inverse, des redirections, des fichiers statiques et du texte fixe par domaine. C'est le point d'entrée web unifié de Gately. Emplacement dans le menu : barre latérale « Web → Services Web ».

## Structure : écouteurs, sites et site par défaut

La configuration comporte trois niveaux :

1. Écouteur : un port d'écoute. Les champs comprennent le réseau (`tcp` double pile, `tcp4`, `tcp6`), l'adresse d'écoute (vide pour toutes les adresses), le port, l'interrupteur HTTPS, la version TLS minimale (1.0 / 1.1 / 1.2 / 1.3) et l'acceptation ou non du protocole PROXY.
2. Site : un site rattaché à un écouteur, sélectionné par domaine. Un domaine peut être un domaine exact, `*.example.com` (correspond aux sous-domaines à n'importe quelle profondeur) ou `*` (correspond à tout). Priorité de correspondance : domaine exact, puis joker, puis `*`.
3. Site par défaut : utilisé lorsqu'aucun site ne correspond. S'il n'est pas activé, une erreur 404 est renvoyée. Le site par défaut peut aussi être de type « Fermer la connexion », pour que les requêtes vers des domaines inconnus ne reçoivent aucune réponse. Une fois configuré, le site par défaut apparaît en dernier dans la liste des sites ; tant qu'il n'est pas configuré, cliquez sur « Configurer le site par défaut » sur la carte de l'écouteur pour le configurer.

Enregistrer une modification d'un site ou du site par défaut remplace la liste des sites sur place : l'écouteur continue de tourner, les connexions ouvertes (WebSocket compris) restent actives et chaque site conserve ses statistiques. Si la nouvelle configuration est invalide, les sites précédents continuent de servir. Modifier les paramètres de l'écouteur lui-même (réseau, adresse, port, HTTPS, TLS minimal, HTTP/3, protocole PROXY) redémarre cet écouteur.

Un site peut recevoir un « Groupe » (64 caractères au maximum ; le site par défaut ne peut pas être groupé). Sous chaque écouteur, la liste affiche les groupes sous forme de sections repliables : les groupes apparaissent dans l'ordre de leur première occurrence, les sites sans groupe viennent ensuite et le site par défaut reste toujours en dernier. L'état replié de chaque groupe est conservé dans le navigateur. Cliquez sur « Trier » pour passer en mode tri : vous pouvez monter et descendre les sites, les faire glisser au sein de leur groupe, déplacer des groupes entiers et réordonner les écouteurs. L'ordre concerne uniquement l'affichage de la liste : la correspondance des domaines suit la priorité ci-dessus quel que soit l'ordre des sites, et l'enregistrement du nouvel ordre remplace la liste des sites sur place sans couper les connexions. Le champ de recherche en haut de la page filtre les sites par domaine, nom, groupe, backend ou remarque.

En mode tri, vous pouvez aussi cocher plusieurs sites (la case de l’en-tête d’un groupe sélectionne tout le groupe). Faire glisser l’un des sites cochés les déplace tous, et les déposer sur un site ou l’en-tête d’un autre groupe les place dans ce groupe. La barre d’outils place d’un coup les sites cochés dans un groupe, ou les retire de leur groupe si le nom est laissé vide ; les sites cochés sous différents listeners sont enregistrés par listener.

Survolez l’en-tête d’un groupe et cliquez sur le crayon pour le renommer : tous ses éléments passent sous le nouveau nom et gardent leur position et leur ordre. Un nom déjà utilisé fusionne les deux groupes après confirmation.

## Types de sites

| Type | Description |
| --- | --- |
| Proxy inverse | Transmet à un ou plusieurs backends. Les adresses de backend ont la forme `http(s)://host:port`, et WebSocket est pris en charge |
| Redirection | Redirige vers une URL complète avec le code de statut 301 / 302 / 307 / 308 (302 par défaut), en conservant éventuellement le chemin et la chaîne de requête d'origine. L'URL peut contenir `{host}`, remplacé par le domaine utilisé par le visiteur : `https://{host}` avec « Conserver le chemin et la requête » envoie chaque requête HTTP vers HTTPS sur le même domaine. Des variables comme `{path}`, `{1}` et `{STUN_NomRègle_PORT}` sont aussi disponibles ; voir [Variables de redirection](#redirect-vars). |
| Fichiers statiques | Le répertoire doit être un chemin absolu. Avec « Liste du répertoire » désactivée, une requête sur un répertoire n'obtient de réponse que si `index.html` existe. « URL sans extension » essaie `path.html` et `path/index.html` lorsqu'un fichier est introuvable (pour VitePress et les générateurs similaires, comme `try_files $uri $uri.html $uri/index.html` de nginx). « Page 404 » est un chemin à l'intérieur du répertoire, comme `404.html`, renvoyé avec le statut 404 |
| Texte fixe | Contenu personnalisé, `Content-Type` (`text/plain; charset=utf-8` par défaut) et code de statut (100-599, 200 par défaut) |
| Fermer la connexion | N'envoie rien et ferme la connexion, comme `return 444` de nginx. Sert surtout de site par défaut pour que les requêtes qui sondent des domaines inconnus ne reçoivent aucune réponse ; le journal d'accès les enregistre avec le code 444 ; ne passe ni par l'authentification, ni par le contrôle d'accès, ni par le WAF, ni par les réécritures : les autres réglages du site sont sans effet |

Les sites de tous types peuvent définir un journal d'accès, le cross-origin (CORS), des en-têtes de requête et de réponse personnalisés (un `Key: Value` par ligne ; une Value vide supprime cet en-tête), et peuvent référencer une instance de [Pare-feu WAF](/fr/docs/modules/waf) (si l'instance n'existe pas ou est désactivée, les requêtes passent sans filtrage). Voir [En-têtes de requête et de réponse](#headers) pour la modification des en-têtes de requête, des en-têtes de réponse et du Content-Type.

Les entrées du journal d'accès enregistrent l'IP du client, la méthode, le domaine et le chemin, le code d'état, la durée et le nom du site, suivis du Referer et du User-Agent au format nginx (`-` s'ils manquent) : les appareils situés derrière une même IP publique se distinguent ainsi par leur User-Agent. Le journal peut être filtré par site, code d'état, IP source, etc. Voir [consulter et filtrer les journaux d'exécution](/fr/docs/guide/configuration#log-filter).

### Variables de redirection {#redirect-vars}

La cible d'un site de redirection peut utiliser les variables suivantes :

| Variable | Signification |
| --- | --- |
| `{host}` | Domaine demandé par le visiteur, sans le port |
| `{port}` | Port par lequel la requête est arrivée |
| `{hostAndPort}` | Domaine et port, identiques au Host de la requête |
| `{path}` / `{PATH}` | Chemin de la requête ; `{PATH}` sans le `/` initial |
| `{args}` / `{Args}` | Chaîne de requête ; `{args}` inclut le `?` initial (vide sans chaîne de requête), `{Args}` non |
| `{domainPartN}` / `{pathPartN}` | La N-ième partie du domaine (séparé par `.`) ou du chemin (séparé par `/`), à partir de 1 |
| `{ClientIP}` / `{RemoteIP}` | IP du client |
| `{time}` | Heure actuelle au format `2006-01-02 15:04:05`, l'espace étant écrit `%20` (`2006-01-02%2015:04:05`) pour que l'adresse de redirection reste une URL valide |
| `{1}` | La partie couverte par `*` dans un domaine de site comme `*.example.com` ; vide si un domaine exact correspond, le domaine complet si `*` ou le site par défaut correspond |
| `{STUN_NomRègle_PORT}` / `{STUN_NomRègle_IP}` / `{STUN_NomRègle_ADDR}` | Port, IP et `IP:port` publics actuels d'une règle de [Traversée STUN](/fr/docs/modules/stun) ; le nom est celui affiché dans la liste et peut contenir des tirets bas |

- Avec « Conserver le chemin », une cible contenant `{path}`, `{PATH}` ou `{pathPartN}` ne reçoit plus le chemin d'origine, et une cible contenant `{args}`, `{Args}` ou sa propre chaîne de requête ne reçoit plus la requête d'origine ; l'autre partie est toujours ajoutée ;
- Quand le port STUN change, inutile de modifier le site : la requête suivante va vers le nouveau port. Si la règle référencée n'existe pas, est désactivée, n'a pas encore réussi la traversée ou est en cours de nouvelle tentative, le site renvoie 503 et le journal d'exécution en indique la raison ;
- Si la cible n'a plus d'hôte une fois les variables remplacées (par exemple `{1}` est vide parce qu'un domaine exact correspond, alors que la cible est seulement `https://{1}`), le site renvoie 503 ; si l'hôte du visiteur contient des caractères absents des noms de domaine, comme `&`, `=`, `/` ou `@`, il renvoie 400 sans l'insérer dans la cible ;
- Les domaines de site n'acceptent qu'un seul `*`, il n'existe donc que `{1}` ;
- Les cibles de redirection des règles de réécriture de chemin ne gèrent toujours que `{host}` ; un `{xxx}` inconnu est conservé tel quel.

Exemple : le CDN Cloudflare envoie `*.example.com` vers le port 2053 de cette machine, et cet écouteur a un site de redirection pour `*.example.com` avec la cible `https://{1}.v4.example.com:{STUN_web_PORT}{path}{args}`. Visiter `zly.example.com/a?b=1` redirige vers `https://zly.v4.example.com:<port STUN>/a?b=1` ; la règle STUN web transfère vers un autre écouteur, où des sites de proxy inverse sont créés pour les domaines sous `*.v4.example.com`.

### Options du proxy inverse {#proxy-options}

Ce sont les « Options » de l'onglet « Général » de l'éditeur de site. L'icône point d'interrogation à côté affiche une courte explication dans l'interface et renvoie à cette section.

| Option | Effet | Quand l'activer |
| --- | --- | --- |
| Ignorer la vérification du certificat du backend | Ne vérifie pas le certificat lors de la connexion à un backend HTTPS | Le backend utilise un certificat auto-signé ou expiré, comme le port https d'un NAS ou de l'interface d'un routeur. L'identité du backend ne peut plus être confirmée : à réserver aux backends du réseau local |
| Utiliser l'adresse du backend comme Host | Remplace l'en-tête Host envoyé au backend par l'adresse du backend ; désactivé (par défaut), le Host de la requête du visiteur est conservé | Le backend n'accepte que sa propre adresse, par ex. erreur 400 / 403, redirection vers une mauvaise adresse ou Host non reconnu ; généralement nécessaire vers des sites publics ou des CDN. Laissez désactivé si le backend distingue les sites par domaine ou génère des liens avec le domaine visité |
| Désactiver la réutilisation des connexions | Ouvre une nouvelle connexion au backend pour chaque requête (Keep-Alive désactivé) | Le backend réinitialise parfois les connexions, des requêtes se bloquent ou il ne gère pas les connexions persistantes. Ajoute un peu de latence ; laissez désactivé en temps normal |
| Ajouter X-Forwarded-* | Écrit `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host` / `X-Forwarded-Port` et `X-Real-IP` pour transmettre au backend l'IP réelle, le protocole d'origine, le domaine et le port du visiteur | Le backend journalise les vraies IP ou doit savoir que l'extérieur est en HTTPS pour générer des liens corrects (sinon il peut revenir en http ou boucler après connexion). Recommandé pour la plupart des applications |
| Envoyer PROXY v2 au backend | Ajoute un en-tête de protocole PROXY v2 au début de la connexion TCP pour transmettre l'IP réelle du client. Une fois activée, chaque requête ouvre une nouvelle connexion et HTTP/2 n'est plus utilisé vers le backend. Si le backend est Gately, activez la réception du protocole PROXY sur son écoute et ajoutez cette machine à ses sources PROXY de confiance. | Le backend ne lit pas les en-têtes HTTP mais gère le protocole PROXY : un autre Gately, haproxy ou nginx avec `proxy_protocol`. Si le backend n'est pas configuré pour l'accepter, toutes les requêtes échouent ; laissez désactivé en cas de doute |
| Utiliser le domaine visité comme SNI | Backends HTTPS uniquement : envoie lors de la négociation le domaine utilisé par le visiteur et vérifie le certificat avec, comme nginx `proxy_ssl_server_name on; proxy_ssl_name $host` | Le backend choisit certificat ou routage selon le domaine sur un même port, comme frp `vhost_https` ou un nginx hébergeant plusieurs sites HTTPS. Chaque backend a son propre pool de connexions, jamais partagé entre domaines |
| Transmettre les cookies de connexion Gately | Transmet tels quels au backend les cookies de connexion propres à Gately (authentification unique `gately_sso`, session du site, nonce de connexion tierce) ; désactivé (par défaut), ces cookies ne sont pas envoyés au backend afin de ne pas divulguer d'identifiants | Le backend est un autre Gately chargé de l'authentification et ce site n'en a aucune. Par exemple un Gately public qui ne fait que WAF et contrôle d'accès et relaie vers un Gately du réseau local, chargé de l'authentification web et de l'authentification unique. Indisponible tant que ce site a une authentification ; à n'utiliser que si le backend est votre propre Gately |

Combinaisons courantes :

- Proxy vers une application web du réseau local (par ex. `http://192.168.1.10:5000`) : activez seulement « Ajouter X-Forwarded-* » ;
- Le backend est une adresse https avec certificat auto-signé : activez aussi « Ignorer la vérification du certificat du backend » ;
- Proxy vers un site public ou un service tiers : activez « Utiliser l'adresse du backend comme Host » ;
- Un service placé devant le backend route selon le SNI (par ex. frp `vhost_https`) : activez « Utiliser le domaine visité comme SNI ».
- Gately public uniquement pour le WAF, Gately du réseau local chargé de la connexion : pas d'authentification sur le site public, activez « Transmettre les cookies de connexion Gately » et, pour l'IP réelle, « Envoyer PROXY v2 au backend », le Gately local acceptant le protocole PROXY et faisant confiance au Gately public.

### En-têtes de requête et de réponse {#headers}

Se règlent dans l'onglet « En-têtes » de l'éditeur de site. Les en-têtes de requête sont envoyés au backend et ne s'appliquent qu'aux sites en proxy inverse ; les en-têtes de réponse sont renvoyés au client et s'appliquent à tous les types de site.

- Édition par ligne : un clic dans le champ du nom liste les en-têtes courants et leur usage ; en choisir un remplit une valeur courante, et le champ valeur propose d'autres valeurs courantes. Les deux champs acceptent aussi la saisie libre. Une valeur vide supprime l'en-tête, et une ligne `Key: Value` collée dans le champ du nom est découpée automatiquement ;
- « Ajouter les en-têtes de sécurité courants » ajoute `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN` et `Referrer-Policy: strict-origin-when-cross-origin`, sans doubler les en-têtes déjà présents ;
- Le « Mode texte » permet de coller plusieurs lignes `Key: Value` d'un coup ; le contenu est conservé en revenant à l'édition par ligne ;
- Les en-têtes de requête supplémentaires de chaque chemin dans « Backends par chemin » se modifient de la même façon et s'appliquent après ceux du site ; à nom égal, le chemin l'emporte.

Chaque ligne reçoit un retour immédiat. Le rouge signale un enregistrement qui échouera ou un en-tête sans effet : nom manquant, nom contenant des espaces, deux-points ou autres caractères interdits, deux-points absent en mode texte. L'orange signale un réglage enregistrable mais au résultat peut-être inattendu :

- `Host` défini comme en-tête de requête n'a aucun effet ; pour le réécrire, activez « Utiliser l'adresse du backend comme Host » ;
- Une valeur fixe pour `X-Forwarded-For` ou `X-Real-IP` écrase l'IP réelle du client ; pour la transmettre, activez « Ajouter X-Forwarded-* » ;
- `Content-Length`, `Connection`, `Transfer-Encoding` et en-têtes similaires sont gérés par la connexion ; les définir à la main est ignoré ou perturbe le transfert ;
- Les valeurs sont envoyées telles quelles, les variables nginx comme `$host` ne sont pas prises en charge ; les espaces réservés comme `<token>` des valeurs proposées doivent être remplacés ;
- Les en-têtes de même nom s'appliquent dans l'ordre, le dernier l'emporte.

Pour les sites « Texte fixe », le `Content-Type` se choisit parmi les types courants ou se saisit librement. L'interface vérifie le format `type/sous-type; paramètre=valeur`, signale qu'un `text/*` sans `charset` peut mal afficher le texte non anglais, et utilise `text/plain; charset=utf-8` si le champ est vide.

## Transport et protocoles

- HTTPS : utilise un certificat du module [Certificats SSL](/fr/docs/modules/ssl) qui couvre le domaine ;
- HTTP sur le même port : un écouteur HTTPS peut aussi accepter le HTTP en clair, le protocole étant reconnu au premier octet de chaque connexion. *Rediriger vers HTTPS* renvoie `http://hôte:port` vers `https://hôte:port` (301 pour GET/HEAD, 308 pour les autres méthodes), utile quand un seul port est disponible et que les visiteurs peuvent se tromper de schéma ; *Servir* répond en HTTP non chiffré. Les validations ACME HTTP-01 restent prises en charge et modifier cette option redémarre l'écouteur ;
- HTTP/3 (QUIC) : à activer sur un écouteur HTTPS pour servir aussi HTTP/3 en UDP sur le même numéro de port, annoncé au navigateur par l'en-tête de réponse `Alt-Svc` ; ce port UDP doit être autorisé dans le pare-feu et la redirection de ports ;
- WebSocket : pris en charge automatiquement par le proxy inverse ;
- Protocole PROXY : un écouteur peut accepter les en-têtes PROXY envoyés par des composants en amont (comme haproxy et frp). Il ne fait confiance qu'aux IP ou sous-réseaux listés dans « Paramètres globaux → Sources PROXY de confiance » ; une liste vide ne fait confiance à aucune source. Le proxy inverse peut envoyer des en-têtes PROXY au backend. `X-Forwarded-Proto: https` sur les connexions d'une source de confiance est aussi pris en compte : les cookies de session reçoivent Secure, les ports qui redirigent le HTTP en clair ne redirigent plus et, avec « Ajouter X-Forwarded-* », https est transmis au backend.

## Authentification

À configurer dans la page « Sécurité » de l'éditeur de site :

- Authentification web : le navigateur affiche une page de connexion ;
- Authentification Basic : HTTP Basic ;
- Utilisateurs : un `username:password` par ligne, le mot de passe pouvant être un hash bcrypt. L'authentification Basic exige au moins un utilisateur lorsqu'elle est activée ; l'authentification web exige au moins un utilisateur ou une identité de connexion tierce lorsqu'elle est activée ;
- Connexion tierce : GitHub, Google, QQ et OIDC sont pris en charge, à partir de vos propres applications OAuth. Les identités autorisées s'écrivent une par ligne sous la forme `provider:account`, par exemple `github:octocat` et `google:me@gmail.com`. Un `#` en fin de ligne ajoute un commentaire, et `github:*` désigne n'importe quel compte de ce fournisseur.

Les fournisseurs de connexion tierce se configurent dans « Paramètres globaux » : saisissez votre propre Client ID / Secret et activez le fournisseur, puis définissez aussi une adresse de rappel (elle doit être servie par l'un des écouteurs de Gately, par exemple `https://auth.example.com`). L'URL de rappel à déclarer auprès de GitHub / Google / QQ est :

```text
https://auth.example.com/__gately_auth/oauth/callback
```

La connexion tierce pour l'interface d'administration elle-même se configure séparément dans « Paramètres → Compte et sécurité ».

### Authentification déléguée (Authelia et similaires)

Confiez l'authentification à un service externe, comme `auth_request` de nginx ou `forwardAuth` de Traefik. Cochez « Authentification déléguée » dans la page « Sécurité ». Elle ne peut pas être combinée avec l'authentification Basic ou web sur le même site.

| Champ | Description |
| --- | --- |
| `url` | Le point de vérification, comme `http://127.0.0.1:9091/api/authz/auth-request` pour Authelia |
| `loginUrl` | Où envoyer les visiteurs lorsque le point répond 401, comme `https://auth.example.com/?rd={url}&rm={method}`. `{url}` est l'adresse d'origine (encodée en URL) et `{method}` la méthode de la requête. Lorsqu'il est vide, la réponse du point est renvoyée telle quelle |
| `copyHeaders` | En-têtes d'identité copiés de la réponse du point vers la requête envoyée au backend ; par défaut `Remote-User`, `Remote-Groups`, `Remote-Email` et `Remote-Name` |

Fonctionnement :

- Chaque requête est d'abord envoyée à `url` en GET sans corps, avec les en-têtes du visiteur (cookies, `Authorization`, etc.) plus `X-Original-Method`, `X-Original-URL`, `X-Forwarded-Method`, `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-Uri`, `X-Forwarded-For` et `X-Real-Ip`. Le délai d'attente est de 10 secondes ;
- Une réponse 2xx laisse passer la requête, avec les en-têtes d'identité ajoutés ; un 401 redirige vers `loginUrl` (statut 302) ; les autres réponses, comme 403 ou une redirection, sont renvoyées au visiteur ; un service injoignable donne 502 ;
- Les en-têtes d'identité envoyés par les visiteurs sont toujours supprimés, ils ne peuvent donc pas être falsifiés ;
- Elle fonctionne avec les règles de chemin et le « Périmètre d'authentification », de sorte que seuls certains chemins (par exemple `/admin`) exigent une connexion, ainsi qu'avec les liens de partage temporaires.

### Utilisateurs partagés : TOTP et authentification unique {#shared-users}

Définissez les comptes une seule fois et utilisez-les sur plusieurs sites, avec une vérification en deux étapes TOTP facultative. Cela couvre ce pour quoi beaucoup de gens font tourner Authelia.

Sur la page « Web → Utilisateurs d'auth » de la barre latérale (la boîte de dialogue des paramètres globaux du service web n'affiche qu'un résumé et un lien) :

- Ajoutez des utilisateurs avec un nom d'utilisateur et un mot de passe. Les mots de passe sont stockés sous forme de hash bcrypt et ne sont jamais renvoyés par l'API. Pour ajouter le TOTP, cliquez sur « Activer le TOTP », scannez le QR code avec une application d'authentification (Google Authenticator, 1Password, etc.) puis enregistrez. Régénérer la clé invalide l'ancienne ;
- Indiquez éventuellement une adresse e-mail, un nom affiché et des groupes ; l'[authentification unique OIDC](/fr/docs/modules/oidc) les transmet aux applications sous forme de revendications `email`, `name` et `groups`, et les utilisateurs autorisés d'un client OIDC peuvent accorder l'accès par groupe avec `group:nom` ;
- Domaine SSO, comme `example.com` : après connexion sur n'importe quel site de ce domaine ou de ses sous-domaines, les autres sites n'exigent plus de connexion. Il repose sur un cookie de domaine parent (`gately_sso`, valable 7 jours) qui n'est jamais transmis aux backends. Laissez vide pour vous connecter séparément sur chaque site.

Sur un site, cochez « Utiliser les utilisateurs partagés » à côté de l'authentification web ou Basic. Les utilisateurs propres au site continuent de fonctionner. Vous pouvez aussi cocher des sites en masse dans la liste des sites de la page Utilisateurs d'auth, regroupée par écoute et filtrable : cocher un site sans authentification active aussi l'auth Web ; un site qui ne s'authentifie qu'avec les utilisateurs partagés ne peut pas être décoché ici, pour qu'il ne devienne jamais public par erreur (pour le rendre public, désactivez l'authentification dans les paramètres du site) ; les sites en authentification déléguée ne peuvent pas utiliser les utilisateurs partagés. Les modifications des utilisateurs, d'OIDC et des sites sur la page s'appliquent ensemble quand vous cliquez sur Enregistrer en bas ; si l'une d'elles échoue à la validation, rien n'est enregistré, et quitter la page avec des modifications non enregistrées demande d'abord une confirmation.

Comportement :

- La page de connexion affiche un champ de code de vérification dès qu'un utilisateur partagé a le TOTP. Chaque code ne sert qu'une fois ;
- Changer le mot de passe ou la clé TOTP d'un utilisateur met fin à ses sessions ;
- L'authentification Basic n'accepte que les utilisateurs partagés sans TOTP, car l'invite de mot de passe du navigateur ne permet pas de saisir un code ;
- Les échecs comptent dans la même limitation de débit et le même bannissement automatique que les autres connexions.

Pour que Gitea, OpenList, Grafana et les autres applications sachent quel utilisateur s'est connecté, proposez-leur les utilisateurs partagés via l'[authentification unique OIDC](/fr/docs/modules/oidc).

### Changement de mot de passe en libre-service {#self-service-password}

Activez Changement de mot de passe en libre-service sur la page Utilisateurs d'auth : les utilisateurs partagés peuvent alors changer leur mot de passe sans passer par l'administrateur.

- Rendez-vous sur `https://domaine-du-site/__gately_auth/password` sur un site où Utiliser les utilisateurs partagés est activé ; la page de connexion affiche aussi un lien Changer le mot de passe. Les utilisateurs de l'[authentification unique OIDC](/fr/docs/modules/oidc) peuvent utiliser `/__gately_oidc/password` sur le domaine émetteur ;
- Saisissez le nom d'utilisateur, le mot de passe actuel et deux fois le nouveau (8 à 128 caractères, différent de l'actuel) ; les utilisateurs avec TOTP saisissent aussi un code de vérification ;
- Ensuite, les sessions de l'utilisateur sur tous les sites prennent fin immédiatement et il se reconnecte avec le nouveau mot de passe ; les jetons d'actualisation OIDC émis sont révoqués et les anciens Access Tokens ne permettent plus de lire les informations de l'utilisateur ; les jetons que les applications valident elles-mêmes restent valables jusqu'à leur expiration, et les sessions propres aux applications sont conservées ;
- Après un changement réussi, le même compte ne peut pas changer à nouveau son mot de passe pendant 1 minute, pour éviter que des changements répétés fassent recharger sans cesse FTP et SMB et coupent les connexions des autres ; quand une vérification du service refuse l'enregistrement, le détail est écrit uniquement dans le journal du service web et la page invite à contacter l'administrateur ;
- Un mot de passe actuel ou un code erroné compte dans la même limitation et le même bannissement automatique que les échecs de connexion, et les formulaires envoyés depuis d'autres sites sont refusés ; le journal du service web enregistre l'utilisateur et l'IP source ;
- La même page change aussi le mot de passe des [utilisateurs de fichiers partagés](/fr/docs/modules/tools#shared-users). Chaque type de compte a son propre interrupteur ; si les deux sont activés, la page permet de choisir le type de compte. Les comptes des deux types portant le même nom sont indépendants.

## Contrôle d'accès

### Liste d'autorisation et liste de blocage d'IP

Le « Contrôle d'accès par IP » de la page « Sécurité » du site peut être désactivé, en liste d'autorisation ou en liste de blocage. La liste accepte une IP ou un sous-réseau par ligne (comme `192.168.1.0/24`), et peut aussi référencer des listes d'IP abonnées.

### En cas de refus

**En cas de refus**, dans l'onglet Sécurité du site, détermine le traitement des requêtes rejetées par un bannissement, le contrôle d'accès par IP (règles de chemin comprises) ou le filtre par pays : **Renvoyer 403** par défaut ; **Fermer la connexion** n'envoie rien et coupe la requête, journalisée en 444, comme un site Fermer la connexion. Associé à une liste d'autorisation, cela laisse passer vos propres IP et coupe toutes les autres. La limitation de débit (429) et les blocages du WAF ne changent pas.

Fermer la connexion, comme type de site ou comme action de refus, retient seulement la réponse HTTP et ne masque pas que le service fonctionne :

- la poignée de main TCP est terminée, le client sait donc que le port écoute ;
- HTTP/1.x ferme la connexion, alors que HTTP/2 et HTTP/3 ne réinitialisent que la requête et gardent la connexion ouverte, réutilisable ;
- en HTTPS, les domaines sans certificat échouent déjà pendant la poignée de main TLS : les requêtes qui atteignent le site par défaut ont reçu le certificat et les noms de domaine qu'il contient.

Pour que les scanners ne voient ensuite que des délais d'attente, bannissez leurs adresses dans [Prévention d'intrusion](/fr/docs/modules/guard) ; avec la synchronisation du pare-feu, leur trafic est rejeté dans le noyau.

### Listes d'IP abonnées

Ajoutez des listes dans « Paramètres globaux → Listes d'IP abonnées » : indiquez l'URL de téléchargement et l'intervalle d'actualisation (24 heures par défaut). Gately télécharge la liste périodiquement et la met en cache dans le répertoire de configuration, elle reste donc utilisable après un redémarrage sans accès à Internet. Le contenu comporte une IP, un CIDR ou une plage `début-fin` par ligne ; tout ce qui suit `#`, `;` ou `//` est un commentaire. Pour les plages d'IP des provinces et villes chinoises, vous pouvez par exemple utiliser `https://metowolf.github.io/iplist/data/cncity/440000.txt` (province du Guangdong) de [metowolf/iplist](https://github.com/metowolf/iplist).

- Les listes d'autorisation et de blocage des sites et des règles de chemin peuvent référencer plusieurs listes, vérifiées avec les plages saisies à la main : correspondre à l'une d'elles suffit pour être dans la liste ;
- Si le contenu téléchargé est illisible (par exemple une page d'erreur), la liste précédente est conservée, la raison est affichée sur la liste et un nouvel essai a lieu une heure plus tard ;
- Tant qu'une liste n'a pas été téléchargée, elle est traitée comme vide : une liste d'autorisation refuse alors l'accès, et une liste de blocage ne bloque rien de plus ;
- Une liste encore référencée par un site ne peut pas être supprimée.

### Règles de chemin

Dans « Règles de chemin » de la page « Sécurité », remplacez l'authentification et les paramètres d'IP du site par préfixe de chemin :

| Champ | Valeurs |
| --- | --- |
| `prefix` | Un préfixe de chemin commençant par `/`, comme `/admin` |
| `auth` | Hériter du site (vide), `off` (aucune authentification), `on` (authentification requise) |
| `ipMode` / `ipList` | Hériter du site (vide), `whitelist` ou `blacklist`, avec une liste d'IP ou de sous-réseaux |

Règles de correspondance :

- Le préfixe le plus long l'emporte ;
- La correspondance respecte les frontières de segments de chemin : `/admin` correspond à `/admin` et `/admin/x`, mais pas à `/administrator` ;
- Le chemin de la requête est d'abord normalisé (`.`, `..` et les barres obliques en double sont supprimés) puis comparé, de sorte que `/public/../admin` est traité comme `/admin` ;
- Une seule règle peut être configurée pour un même préfixe.

L'option « Portée de l'authentification » (`authOptional`) : une fois cochée, l'authentification ne s'applique qu'aux chemins marqués « Requis » dans les règles, et tous les autres chemins sont publics. Cette option nécessite d'activer d'abord l'authentification Basic ou web.

### Liens de partage temporaires

Une fois l'authentification Basic ou web activée, vous pouvez créer des liens de partage pour un site afin que des visiteurs y accèdent sans compte :

| Champ | Description |
| --- | --- |
| Nom | Une étiquette, par exemple « Pour Alice » |
| Expiration | Laissez vide pour aucune expiration |
| Utilisations `maxUses` | Nombre maximal de fois où le lien peut être utilisé ; 0 signifie illimité. Le compteur est réinitialisé après un redémarrage |
| Limite de chemin `pathPrefix` | Lorsqu'il n'est pas vide, le lien ne donne accès qu'aux chemins sous ce préfixe ; il doit commencer par `/` |

Un visiteur ouvre `https://domain/any-path?_gs=token` pour obtenir une session. Après une utilisation réussie, le serveur dépose un cookie d'accès sans connexion et redirige vers l'adresse sans le paramètre `_gs`. La session dure au plus 24 heures et jamais au-delà de l'expiration du lien. Un jeton invalide est traité comme une visite normale et ne révèle pas la cause de l'échec. Les sessions déjà émises deviennent immédiatement invalides une fois le lien supprimé. Le jeton est généré après l'enregistrement (16 caractères au minimum), et le lien complet peut être copié dans l'interface.

### Limitation de débit des échecs d'authentification et bannissement automatique

- Après 10 échecs d'authentification depuis la même IP source en 15 minutes, les tentatives suivantes sont rejetées pendant un certain temps ;
- Sous « Paramètres globaux → Bannissement automatique → Bannir après échec d'authentification », saisissez un nombre de minutes (0 à 43200) ; l'IP source est bannie à 10 échecs. 0 signifie aucun bannissement, limitation de débit uniquement ;
- Un événement `AuthBanned` est publié lorsqu'un bannissement automatique se déclenche, et il peut être envoyé via le [Centre de notifications](/fr/docs/modules/notify) ;
- Les adresses IPv6 sont comptées et bannies par sous-réseau /64.

### Liste de bannissement

La liste de bannissement dans « Paramètres globaux » affiche les IP actuellement bannies, la raison (échecs d'authentification répétés ou bannissement manuel) et l'heure d'expiration, et chaque entrée peut être levée avec « Débannir ». Vous pouvez aussi bannir manuellement : saisissez une IP et un nombre d'heures (1 à 720, soit 30 jours au maximum) ; l'API utilise 24 heures par défaut.

La liste de bannissement est partagée avec la [prévention d'intrusion](/fr/docs/modules/guard) : quand ce module est activé, cette section n'affiche que le nombre de bannissements et renvoie à sa page pour les gérer. Les bannissements s'appliquent aussi à la redirection de ports, à STUN et aux autres services, et sont rejetés dans le pare-feu du système quand la synchronisation est activée ; avec « Ignorer les adresses du LAN », ces adresses ne sont jamais bannies automatiquement.

### Filtre de pays GeoIP

Le « Filtre de pays » d'un site propose trois modes : désactivé, autoriser uniquement et refuser. Saisissez des codes pays ISO à deux lettres séparés par des virgules, comme `CN, HK, US`. Au moins un pays est requis lorsque le filtre est activé.

- Le filtrage dépend d'une base GeoIP, que vous devez d'abord importer sous « Paramètres globaux → Base GeoIP » : téléversez un fichier `.mmdb`, ou saisissez une URL de téléchargement. MaxMind GeoLite2, DB-IP et d'autres bases de pays ou de villes sont pris en charge ; la limite de téléversement est de 129 Mo ;
- Lorsqu'aucune base n'est chargée, le filtrage n'a aucun effet (tous les visiteurs passent) et l'interface affiche un avertissement ;
- Les adresses de réseaux privés sont toujours autorisées ;
- En mode « Refuser », les adresses dont le pays est introuvable dans la base sont autorisées.

## Backends par chemin

Pour un site en proxy inverse, envoyez les requêtes sous un préfixe de chemin vers d'autres backends, comme `location /api/ { proxy_pass ...; }` de nginx. Ajoutez-les sous « Backends par chemin » dans la page « Général » de l'éditeur de site.

| Champ | Description |
| --- | --- |
| `prefix` | Un préfixe de chemin comme `/api/`. La correspondance respecte les frontières de segments de chemin : `/api` et `/api/` correspondent tous deux à `/api` et `/api/x`, mais pas à `/apix` |
| `targets` | Les backends de ce chemin, écrits comme les backends du site. Lorsque le champ est vide, les backends du site sont utilisés et seuls les en-têtes ci-dessous sont ajoutés |
| `requestHeaders` | En-têtes de requête ajoutés pour ce chemin, un `Key: Value` par ligne. Ils sont appliqués après ceux du site, et un en-tête du même nom remplace celui du site |

- Le préfixe le plus long l'emporte, et le chemin est transmis tel quel (le préfixe n'est pas retiré) ;
- Les autres réglages du site, comme l'option Host, la vérification du certificat, les en-têtes transmis, le protocole PROXY, la politique de répartition de charge et les vérifications d'état, s'appliquent à chaque backend par chemin ;
- La compression, la limitation de débit, le cache et les réécritures s'appliquent à l'ensemble du site.

## Répartir par utilisateur {#user-routes}

Pour un site en proxy inverse, envoyez les requêtes de chaque utilisateur connecté vers ses propres backends, comme `map $remote_user $backend` de nginx. Le site doit d'abord avoir l'authentification Basic, l'authentification web ou l'authentification déléguée activée. Ajoutez ensuite des règles sous « Répartir par utilisateur » dans la page « Sécurité » de l'éditeur de site : chaque règle indique un ensemble d'utilisateurs et les backends de ces utilisateurs, écrits comme les backends du site.

Les utilisateurs sont reconnus par ces identifiants :

- Le nom d'un utilisateur du site ou d'un [utilisateur partagé](#shared-users) ;
- Pour une connexion tierce, `fournisseur-key:identifiant`, par exemple `github:octocat` ; les autres identifiants de cette identité correspondent aussi ;
- Pour l'authentification déléguée, le `Remote-User` copié depuis la réponse du service d'authentification.

Répartition :

- Les utilisateurs non listés, ainsi que les visiteurs non connectés sur les chemins publics (chemins qui ne demandent aucune authentification), utilisent les backends du site. Sur les chemins publics, un visiteur déjà connecté par l'authentification web est lui aussi réparti selon son identité, si bien que les pages d'une même application arrivent sur le même backend ;
- Les backends par chemin qui ont leurs propres backends (voir la section précédente) sont prioritaires ; ceux qui ajoutent seulement des en-têtes utilisent les backends de l'utilisateur ;
- Lorsque « Transmettre le nom d’utilisateur » contient un nom d'en-tête comme `X-Remote-User`, le nom de l'utilisateur connecté est placé dans cet en-tête pour le backend. Le même en-tête envoyé par les visiteurs est toujours supprimé, il ne peut donc pas être falsifié ;
- Avec le cache de réponses en mémoire activé, la clé de cache inclut la règle utilisateur choisie, ainsi que le nom d'utilisateur lorsque « Transmettre le nom d’utilisateur » est actif ; les réponses de backends différents ou destinées à des utilisateurs différents restent ainsi séparées.

L'enregistrement vérifie que l'authentification est activée sur le site, que chaque règle comporte au moins un utilisateur et des backends valides, qu'aucun utilisateur ne figure dans deux règles, et que le nom d'en-tête est valide et ne fait pas partie des en-têtes qui influent sur la transmission ou l'authentification, comme `Host`, `Cookie`, `Authorization` ou `X-Forwarded-For`. Dans la liste des sites, un site doté de règles par utilisateur affiche une étiquette avec le nombre de règles.

## Améliorations du proxy inverse

Ces options se trouvent dans la page « Trafic » de l'éditeur de site.

### Backends, poids et secours

La liste des backends accepte une adresse par ligne, éventuellement suivie de paramètres :

```text
http://10.0.0.2:8080 weight=3
http://10.0.0.3:8080 backup
```

- `weight=N` : poids, un entier de 1 à 100, 1 par défaut ;
- `backup` : un backend de secours, utilisé uniquement lorsque tous les backends hors secours sont indisponibles. Lorsque tous les backends sont indisponibles, tous ceux hors secours sont réessayés.

### Politique de répartition de charge

| Option de l'interface | `lbPolicy` | Description |
| --- | --- | --- |
| Tourniquet (pondéré) | Vide | Par défaut |
| Aléatoire (pondéré) | `random` | |
| Hash d'IP (sessions persistantes) | `ip_hash` | Une même IP cliente va toujours vers le même backend |
| Moins de connexions | `least_conn` | Choisit la plus petite valeur de connexions / poids |

### Vérifications d'état

L'interrupteur de vérification active est désactivé par défaut. Champs et valeurs par défaut une fois activée :

| Champ | Valeur par défaut | Plage |
| --- | --- | --- |
| Mode `mode` | `http` | `http` (requête sur le chemin de vérification ; un code de statut inférieur à 400 est considéré comme sain) ou `tcp` (teste seulement la connexion) |
| Chemin `path` | `/` | Commence par `/`, mode http uniquement |
| Intervalle `interval` | 10 secondes | 1 à 3600 |
| Délai d'attente `timeout` | 3 secondes | 1 à 60 |
| Échecs consécutifs `fails` | 3 échecs le marquent indisponible | 1 à 20 |
| Succès consécutifs `passes` | 2 succès le rétablissent | 1 à 20 |

Lorsque l'envoi du protocole PROXY au backend est activé, les vérifications HTTP passent automatiquement en TCP.

Les vérifications passives sont toujours actives : parmi plusieurs backends, un backend ayant subi 3 échecs de connexion consécutifs est mis en pause 30 secondes avant d'être sondé de nouveau (un backend unique sans vérification active n'est jamais retiré). Une fois les vérifications actives activées, le rétablissement dépend entièrement de leurs résultats. Lorsque l'état d'un backend change, des événements upstream-down et upstream-recovered sont publiés, et ils peuvent être envoyés via le [Centre de notifications](/fr/docs/modules/notify).

### Compression

La compression peut être activée pour les sites en proxy inverse, en fichiers statiques et en texte fixe (pas pour les sites de redirection). Selon `Accept-Encoding`, `br` est préféré, puis `gzip`.

- Seul le contenu de type texte est compressé : `text/*`, JSON, JavaScript, XML, XHTML, RSS / Atom, WebAssembly et SVG ; `text/event-stream` est exclu ;
- Les réponses dont le `Content-Length` déclaré est inférieur à 256 octets sont laissées telles quelles ;
- Les réponses ayant déjà un `Content-Encoding` sont laissées telles quelles ; les requêtes de mise à niveau WebSocket passent directement ;
- Après compression, les ETag forts sont convertis en ETag faibles.

### Limitation de débit par IP

Limitation de débit à jeton (token bucket) par IP cliente. Champs :

| Champ | Description |
| --- | --- |
| `rate` | Requêtes autorisées par seconde |
| `burst` | Capacité de rafale. Lorsque `rate` est défini et que `burst` vaut 0, il prend par défaut la valeur de `rate` arrondie à l'inférieur plus 1 |
| `maxConns` | Nombre maximal de requêtes simultanées par IP ; 0 signifie illimité |

Lorsque la limitation de débit est activée, au moins l'un de `rate` et `maxConns` doit être défini.

### Cache de réponses en mémoire

S'applique uniquement aux sites en proxy inverse. Le cache réside en mémoire et utilise l'éviction LRU.

| Champ | Valeur par défaut | Plage |
| --- | --- | --- |
| `ttl` durée par défaut | 0 | 0 à 30 jours (en secondes). Lorsque l'amont ne déclare aucune directive de cache, les réponses 200 sont mises en cache pendant cette durée ; 0 signifie que seules les réponses explicitement autorisées par l'amont sont mises en cache |
| `maxMb` capacité du cache | 64 | 1 à 4096 Mo |

Règles de mise en cache :

- Seules les requêtes GET sont mises en cache, et la requête ne doit contenir ni en-tête `Authorization`, `Cookie` ou `Range`, ni `Cache-Control: no-cache` ;
- Seules les réponses de statut 200 sont mises en cache ; une réponse est ignorée si elle contient `Set-Cookie`, `Vary: *`, ou un `Cache-Control` valant `no-store` / `private` / `no-cache` ;
- `s-maxage` est respecté en priorité, puis `max-age` ;
- Une entrée unique est limitée à 4 Mo et à au plus un quart de la capacité totale ;
- Un succès (hit) renvoie les en-têtes `X-Cache: HIT` et `Age`, et un échec (miss) renvoie `X-Cache: MISS` ;
- La clé de cache inclut Host, l'URI complète de la requête et `Accept-Encoding` ; avec la [répartition par utilisateur](#user-routes), elle inclut aussi la règle utilisateur choisie, ainsi que le nom d'utilisateur lorsque « Transmettre le nom d’utilisateur » est actif.

### Réécriture et redirection de chemin

Les règles sont évaluées dans l'ordre, et l'évaluation s'arrête à la première correspondance.

| Champ | Description |
| --- | --- |
| `pattern` | Une expression régulière comparée au chemin de la requête, comme `^/old/(.*)$` |
| `to` | Le résultat de remplacement ; utilisez `$1` pour référencer les groupes |
| `redirect` | Désactivé pour une réécriture interne (le chemin réécrit est envoyé au backend, et la cible doit commencer par `/`) ; activé pour une redirection |
| `code` | Code de statut de redirection 301 / 302 / 307 / 308, 302 par défaut |

Les redirections conservent les paramètres de requête d'origine. La cible d'une redirection peut contenir `{host}`, le domaine utilisé par le visiteur. Les règles de redirection s'exécutent avant l'authentification, comme `location = / { return 302 ...; }` de nginx, de sorte qu'un chemin tel que `/` peut renvoyer les visiteurs ailleurs sans leur demander de se connecter.

## Statistiques d'accès et supervision

### Statistiques de site

Pour un écouteur en cours d'exécution, cliquez sur « Statistiques » sur la ligne d'un site pour ouvrir la fenêtre de statistiques, où vous pouvez choisir la dernière heure, les 6 dernières heures ou les 24 dernières heures :

- Statistiques d'accès à la minute : un point par minute, conservées sur les dernières 24 heures, avec une courbe du nombre de requêtes et des courbes d'erreurs 4xx / 5xx ;
- Répartition des codes de statut (2xx / 3xx / 4xx / 5xx) et trafic cumulé des réponses ;
- Top 10 des IP sources et top 10 des pays (les statistiques par pays nécessitent une base GeoIP importée) ;
- Les sites en proxy inverse affichent l'état de santé, le poids, l'indicateur de secours et le nombre de connexions actuelles de chaque backend.

Les statistiques sont conservées en mémoire et repartent de zéro après un redémarrage ou après la modification des paramètres de l'écouteur lui-même. Modifier un site conserve ses compteurs. Les classements suivent au plus 5000 sources distinctes.

### Métriques Prometheus

Le point de terminaison global `GET /api/metrics` regroupe les métriques de tous les modules ; voir [Métriques de supervision](/fr/docs/reference/monitoring). Le point de terminaison du module décrit ci-dessous ne contient que les métriques des services Web.

Le point de terminaison des métriques est `GET /api/webservice/metrics`, au format texte Prometheus, et il exige des identifiants de connexion ou un [jeton d'API](/en/docs/reference/api-reference).

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/webservice/metrics
```

| Métrique | Type | Description |
| --- | --- | --- |
| `gately_site_requests_total` | counter | Requêtes cumulées d'un site |
| `gately_site_errors_total` | counter | Erreurs de backend d'un site |
| `gately_site_active_requests` | gauge | Requêtes en cours |
| `gately_site_response_bytes_total` | counter | Octets de réponse cumulés |
| `gately_site_responses_total` | counter | Réponses comptées par classe de statut (étiquette `class` : `2xx` à `5xx`) |
| `gately_upstream_up` | gauge | Indique si un backend est sain : 1 sain, 0 indisponible (avec l'étiquette `upstream`) |

Toutes les métriques portent les étiquettes `listener` (nom de l'écouteur#clé de l'écouteur) et `site` (nom du site, ou premier domaine s'il n'a pas de nom).

## Importer une configuration nginx {#import-nginx}

Le bouton « Importer nginx » en haut de la page convertit des sites nginx existants en écouteurs et sites Gately. Le plus simple est d'exécuter `nginx -T` sur l'hôte nginx et d'en coller la sortie complète : elle contient tous les fichiers chargés par include, et Gately développe les include comme nginx.

- Aperçu d'abord : les sites sont regroupés par port d'écoute, avec pour chacun le type, le backend ou le répertoire, le fichier source et le numéro de ligne, ainsi que les points d'attention (directives utilisant des variables, location en expression régulière, domaines sans certificat, etc.) ; les sites non convertibles sont listés avec la raison ;
- Les sites d'un même port sont ajoutés à l'écouteur existant, sinon un nouvel écouteur est créé ; les domaines déjà présents sont ignorés, l'import peut donc être relancé sans risque ;
- Par défaut, les sites sont importés désactivés, pour être vérifiés un par un en parallèle de nginx avant activation ; cochez « Activer dès l'import » pour qu'ils s'appliquent immédiatement (si nginx occupe encore le même port, le nouvel écouteur ne démarre pas) ;
- Éléments convertis : `proxy_pass` (y compris poids, secours et politique de répartition de charge des upstream), répertoires statiques `root` / `alias` (`autoindex`, `try_files $uri.html`, `error_page 404`), redirections et contenu fixe `return`, redirections `rewrite` de tout le site, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, backends par chemin, contrôle d'accès et redirections au niveau des location, `listen ... proxy_protocol` et `quic` ;
- `auth_request` est converti en authentification déléguée : l'adresse de vérification est reprise du `proxy_pass` de la location de la sous-requête d'authentification (variables `set` prises en charge), et la redirection de connexion de `error_page 401 =302` ; les configurations Authelia courantes se convertissent directement. Si l'authentification n'est active que sur certaines location, le « Périmètre d'authentification » est limité aux chemins concernés ;
- `auth_basic` est converti en authentification Basic avec les utilisateurs partagés ; le fichier d'utilisateurs de nginx n'est pas lu, et personne ne peut passer tant que les utilisateurs n'ont pas été ajoutés ;
- Les redirections TCP/UDP du bloc `stream` se configurent dans la [Redirection de ports](/fr/docs/modules/portforward) ; les certificats ne sont pas importés : quand un site HTTPS n'a pas de certificat, l'aperçu indique les fichiers de certificat utilisés par nginx, que vous pouvez ajouter par chemin de fichier dans la page [Certificats](/fr/docs/modules/ssl).

## Remplacer haproxy et nginx

Le routage par protocole et les Services Web réunis peuvent remplacer un duo haproxy + nginx. Une organisation courante : une règle de [Routage par protocole](/fr/docs/modules/protoroute) sur le port 443 qui envoie le VPN et les autres services TLS directement vers leurs backends et tout le reste vers un écouteur de Services Web acceptant le protocole PROXY, plus un écouteur de Services Web sur le port 80 avec un site de redirection (`https://{host}`).

Conversion des réglages nginx :

| nginx | Gately |
| --- | --- |
| `server` avec `server_name` | un site avec ces domaines |
| `location` avec un `proxy_pass` différent | backends par chemin |
| `auth_request` (Authelia) | authentification déléguée, ou utilisateurs partagés avec TOTP |
| `proxy_ssl_name $host` | utiliser le domaine visité comme SNI |
| `return 301 https://$host$request_uri` | un site de redirection avec `https://{host}` et « Conserver le chemin et la requête » |
| `return 302 /admin` dans `location = /` | une réécriture avec « Redirection » activée, motif `^/$` |
| `try_files $uri $uri.html $uri/index.html` et `error_page 404` | fichiers statiques avec « URL sans extension » et « Page 404 » |
| `add_header` | en-têtes de réponse |
| `return 444` | un site Fermer la connexion |
| `error_page 497 https://$host:$server_port$request_uri` dans un server HTTPS | « HTTP sur le même port » d'un nouvel écouteur réglé sur « Rediriger vers HTTPS » ; en cas de fusion avec un écouteur existant, seul un avertissement s'affiche et ses réglages restent inchangés |

## Conseil de déploiement

Les Services Web doivent écouter sur des ports de l'hôte tels que 80 / 443. Pour un déploiement Docker, `--network host` est recommandé.
