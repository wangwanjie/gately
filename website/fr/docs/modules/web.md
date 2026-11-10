# Services Web

Servez un proxy inverse, des redirections, des fichiers statiques et du texte fixe par domaine. C'est le point d'entrée web unifié de Gately. Emplacement dans le menu : barre latérale « Web → Services Web ».

## Structure : écouteurs, sites et site par défaut

La configuration comporte trois niveaux :

1. Écouteur : un port d'écoute. Les champs comprennent le réseau (`tcp` double pile, `tcp4`, `tcp6`), l'adresse d'écoute (vide pour toutes les adresses), le port, l'interrupteur HTTPS, la version TLS minimale (1.0 / 1.1 / 1.2 / 1.3) et l'acceptation ou non du protocole PROXY.
2. Site : un site rattaché à un écouteur, sélectionné par domaine. Un domaine peut être un domaine exact, `*.example.com` (correspond aux sous-domaines à n'importe quelle profondeur), `api-*.example.com` (un `*` dans le premier label correspond à zéro caractère ou plus de ce label et reste à l'intérieur) ou `*` (correspond à tout). Priorité de correspondance : domaine exact, puis joker, puis `*`. Lorsque plusieurs jokers correspondent, la règle la plus longue l'emporte ; à longueur égale, le site placé en premier dans la liste l'emporte. Les jokers dans le premier label fonctionnent aussi avec l'import, la modification par lot et la variable de redirection `{1}`. Les sites HTTPS ont besoin d'un certificat couvrant les noms réellement visités, par exemple `*.example.com` ; les autorités de certification ne délivrent des jokers que sous la forme `*.example.com`.
3. Site par défaut : utilisé lorsqu'aucun site ne correspond. S'il n'est pas activé, une erreur 404 est renvoyée. Le site par défaut peut aussi être de type « Fermer la connexion », pour que les requêtes vers des domaines inconnus ne reçoivent aucune réponse. Une fois configuré, le site par défaut apparaît en dernier dans la liste des sites ; tant qu'il n'est pas configuré, cliquez sur « Configurer le site par défaut » sur la carte de l'écouteur pour le configurer.

Enregistrer une modification d'un site ou du site par défaut remplace la liste des sites sur place : l'écouteur continue de tourner, les connexions ouvertes (WebSocket compris) restent actives et chaque site conserve ses statistiques. Si la nouvelle configuration est invalide, les sites précédents continuent de servir. Modifier les paramètres de l'écouteur lui-même (réseau, adresse, port, HTTPS, TLS minimal, HTTP/3, protocole PROXY) redémarre cet écouteur.

Chaque domaine exact de la liste des sites est une URL complète cliquable qui s’ouvre dans un nouvel onglet, avec à côté un bouton qui copie l’URL ; les domaines génériques (`*.example.com`, `*`) restent affichés en texte. L’URL est déduite de l’écouteur : `https` si HTTPS est activé, `http` sinon, avec le port de l’écouteur, en omettant les ports par défaut comme 443 et 80, par exemple `https://example.com:8443`. Quand le port d’accès de l’écouteur est vide et qu’une règle activée du Routage par protocole ou de la Redirection de ports transfère vers cet écouteur, les liens des sites utilisent automatiquement le port d’entrée (d’écoute) de la règle, par exemple 443 pour un écouteur sur 8443 alimenté par le Routage par protocole depuis 443. L’adresse cible de la règle doit atteindre l’écouteur : une adresse de bouclage ou non spécifiée pour un écouteur sur toutes les adresses, la même adresse pour un écouteur sur une adresse précise. Le Routage par protocole est évalué par domaine dans le même ordre que le routage (domaine exact, puis le générique le plus précis, puis `*`, puis la cible par défaut), en ignorant les routes désactivées et celles qui n’acceptent que des ALPN hors navigateur comme `acme-tls/1` ; les règles de Redirection de ports qui ne transfèrent que de l’UDP ne comptent pas ; les plages de ports se correspondent une à une ; la première règle activée qui correspond l’emporte. L’en-tête de l’écouteur affiche le port détecté et la règle d’origine, et le champ **Adresse d’accès** l’utilise comme texte indicatif ; un port d’accès saisi manuellement reste prioritaire. Quand les visiteurs passent par une redirection de port du routeur, frp ou un CDN (ou qu’une règle transfère vers une adresse LAN de cet hôte), le schéma ou le port publics peuvent différer de l’écouteur ; indiquez-les dans l’**Adresse d’accès** de l’écouteur (par exemple https et 443 quand un CDN récupère le contenu en http sur 8080). L’adresse d’accès sert uniquement aux liens et la modifier ne redémarre pas l’écouteur. À l’enregistrement, les domaines sont stockés tels que les navigateurs les envoient : en minuscules, IPv6 sans crochets, domaines internationalisés en punycode (`中文.com` devient `xn--fiq228c.com`) ; les valeurs qui ne peuvent jamais correspondre, par exemple avec un port, un chemin ou `@`, sont refusées. Les imports depuis Lucky ou nginx ignorent seulement les domaines invalides et le signalent.

Un site peut recevoir un « Groupe » (64 caractères au maximum ; le site par défaut ne peut pas être groupé). Sous chaque écouteur, la liste affiche les groupes sous forme de sections repliables : les groupes apparaissent dans l'ordre de leur première occurrence, les sites sans groupe viennent ensuite et le site par défaut reste toujours en dernier. L'état replié de chaque groupe est conservé dans le navigateur. Cliquez sur « Tri et lot » pour passer en mode tri : vous pouvez monter et descendre les sites, les faire glisser au sein de leur groupe, déplacer des groupes entiers et réordonner les écouteurs. L'ordre concerne uniquement l'affichage de la liste : la correspondance des domaines suit la priorité ci-dessus quel que soit l'ordre des sites, et l'enregistrement du nouvel ordre remplace la liste des sites sur place sans couper les connexions. Le champ de recherche en haut de la page filtre les sites par domaine, nom, groupe, backend ou remarque ; pendant la recherche, les cartes d'écouteurs repliées se déplient automatiquement.

En mode tri, vous pouvez aussi cocher plusieurs sites (la case de l’en-tête d’un groupe sélectionne tout le groupe). Faire glisser l’un des sites cochés les déplace tous, et les déposer sur un site ou l’en-tête d’un autre groupe les place dans ce groupe. La barre d’outils place d’un coup les sites cochés dans un groupe, ou les retire de leur groupe si le nom est laissé vide ; les sites cochés sous différents listeners sont enregistrés par listener.

Survolez l’en-tête d’un groupe et cliquez sur le crayon pour le renommer : tous ses éléments passent sous le nouveau nom et gardent leur position et leur ordre. Un nom déjà utilisé fusionne les deux groupes après confirmation.

### Déplacer des sites {#move-sites}

Un site peut être déplacé vers un autre écouteur avec tous ses réglages :

- En modifiant un site, changez **Écouteur** ; à l'enregistrement, les modifications sont d'abord enregistrées puis le site est déplacé. À l'ajout d'un site, ce champ choisit l'écouteur dans lequel il est créé.
- En mode tri, cochez des sites (y compris sur plusieurs écouteurs), cliquez sur **Déplacer vers un écouteur** dans la barre d'outils, puis choisissez l'écouteur cible et le groupe (par défaut chaque site garde son groupe). Les sites sont placés à la fin de l'écouteur cible ou à la fin du groupe choisi ; ceux déjà présents dans l'écouteur cible ne bougent pas.
- En mode tri, faites glisser des sites au-dessus ou au-dessous d'un site d'un autre écouteur pour les insérer à cet endroit dans son groupe, ou sur un titre de groupe pour les placer à la fin de ce groupe. Pendant le glissement, les écouteurs sans site ordinaire affichent une zone de dépôt. Le dépôt demande une confirmation.

Chaque déplacement est une seule requête : l'écouteur source et l'écouteur cible sont rechargés une fois chacun et leur liste de sites est remplacée sur place, les connexions aux autres sites restent ouvertes. Si l'écouteur cible a déjà un site avec le même domaine, le déplacement est refusé et aucun écouteur n'est modifié ; le site par défaut appartient à son écouteur et ne peut pas être déplacé. Si la clé d'un site est déjà prise dans l'écouteur cible, il reçoit une nouvelle clé, et les règles d'alerte de la surveillance des performances qui le référencent sont mises à jour. Les sessions de connexion sont isolées par écouteur et par site : les visiteurs connectés devront se reconnecter après le déplacement ; les compteurs d'utilisation des liens de partage suivent le site, un lien limité ne redevient donc pas utilisable ; les statistiques et connexions en direct du site repartent de zéro. Déplacer un site protégé par connexion d'un écouteur HTTPS vers un écouteur HTTP non chiffré affiche un avertissement : mots de passe et cookies de connexion circuleront en clair.

### Opérations en lot

Cochez des sites en mode tri (y compris sous plusieurs écouteurs) et la barre d'outils propose les opérations en lot ci-dessous. Chaque opération est soumise en une fois : elle n'est enregistrée que si tous les sites passent la validation ; si l'un échoue, rien n'est modifié et le site en cause est indiqué. Chaque écouteur concerné est rechargé une seule fois et remplace sa liste de sites sur place, sans couper les connexions.

- **Activer / Désactiver** : les sites déjà dans l'état visé restent tels quels.
- **Supprimer** : supprime les sites cochés après confirmation ; le site par défaut ne peut pas être supprimé.
- **Domaines en lot** :
  - Ajouter des domaines : un par ligne (les virgules sont aussi acceptées) ; les domaines déjà présents sur un site sont ignorés, et les nouveaux sont ajoutés à la fin ;
  - Retirer des domaines : correspondance sur le domaine complet ; les domaines absents d'un site sont ignorés, et un site ne peut pas perdre tous ses domaines ;
  - Remplacer le domaine principal : remplace un suffixe de domaine par un nouveau. Le domaine d'origine et ses sous-domaines (y compris le joker `*.domaine-origine`) sont remplacés ; par exemple, avec `old.com` → `new.com`, `www.old.com` devient `www.new.com`. Les doublons issus du remplacement sont fusionnés, et les sites sans correspondance restent inchangés.

  Un domaine nouvellement ajouté est refusé s'il apparaît sur plusieurs sites du même écouteur, car seul le premier de ces sites pourrait lui correspondre ; des écouteurs différents peuvent utiliser le même domaine.
- **Options en lot** : active ou désactive en lot Compression, Journal d'accès, CORS, En-têtes de transfert et Fermer la connexion en cas de refus, et définit ou retire le WAF. Les éléments laissés sur « Ne pas modifier » conservent le réglage propre à chaque site. Les réglages qui touchent à l'accès, comme l'authentification, le contrôle d'accès et les adresses de backend, se modifient site par site.
- **Déplacer vers un écouteur** : déplace les sites cochés vers un autre écouteur, voir [Déplacer des sites](#move-sites).

Le point de terminaison correspondant est `POST /api/webservice/sites/batch`, avec le corps `{"sites": [{"listener": "clé de l'écouteur", "site": "clé du site"}], "action": "...", ...}`. `action` vaut `enable`, `disable`, `delete`, `addDomains`, `removeDomains` (avec `domains`), `replaceDomain` (avec `from` et `to`) ou `set` (avec `options`, qui n'accepte que les champs d'options ci-dessus). La réponse donne le nombre de sites modifiés et le nombre de sites déjà conformes. Une requête traite jusqu'à 500 sites et 100 domaines. Les écouteurs eux-mêmes prennent aussi en charge les points de terminaison génériques `PUT /api/webservice/listeners-enable` et `POST /api/webservice/listeners-delete`. Pour `move`, `move` désigne la cible, par exemple `{"listener": "clé de l'écouteur cible"}`, avec en option `group` (groupe où placer les sites ; une chaîne vide signifie sans groupe, l'omettre conserve le groupe de chaque site) ou `before` / `after` (clé d'un site de l'écouteur cible avant ou après lequel insérer, en rejoignant son groupe). `moved` dans la réponse liste la clé de chaque site avant et après le déplacement.

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
- Si la cible n'a plus d'hôte une fois les variables remplacées (par exemple `{1}` est vide parce qu'un domaine exact correspond, alors que la cible est seulement `https://{1}`), le site renvoie 503 ; si l'hôte du visiteur contient des caractères absents des noms de domaine, comme `&`, `=`, `/` ou `@`, il renvoie 400 sans l'insérer dans la cible ; quand la cible utilise `{host}`, `{hostAndPort}`, `{1}` ou `{domainPartN}`, les requêtes sans en-tête Host (sondes HTTP/1.0, etc.) reçoivent aussi 400 ;
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

### IP réelle du visiteur (derrière un CDN / WAF) {#real-ip}

Lorsqu'un site est derrière un proxy HTTP comme Cloudflare, Alibaba Cloud CDN ou Tencent Cloud CDN, les connexions proviennent des adresses de retour à l'origine du proxy. Renseignez les proxys de confiance (IP ou plages de retour à l'origine du proxy, plusieurs fournisseurs possibles) dans **Paramètres globaux → IP réelle du visiteur** et choisissez les en-têtes d'IP réelle. Gately lit alors l'adresse du visiteur dans les en-têtes et l'utilise pour les journaux d'accès, les listes d'IP autorisées/bloquées, l'accès LAN uniquement, le filtrage par pays, les bannissements après échecs d'authentification, les statistiques, la détection de sondes et les `X-Real-IP` / `X-Forwarded-For` envoyés aux backends.

- Les en-têtes ne sont lus que si le pair d'origine est un proxy de confiance. Le pair d'origine est l'adresse après traitement du protocole PROXY : celle de l'en-tête PROXY pour une connexion venant d'une source PROXY approuvée, sinon le pair TCP. Les en-têtes envoyés par un client connecté directement sont toujours ignorés ;
- Les en-têtes sont vérifiés dans l'ordre choisi et le premier qui donne une adresse valide l'emporte ; sans choix, `X-Forwarded-For` est utilisé. Les en-têtes à plusieurs valeurs comme `X-Forwarded-For` sont lus de droite à gauche en ignorant les proxys de confiance, et la première adresse non approuvée est retenue : les valeurs usurpées à gauche par le client ne sont jamais prises ;
- Ne choisissez que des en-têtes que votre proxy écrase. Cloudflare écrase `CF-Connecting-IP`, par exemple, alors que d'autres proxys transmettent souvent tel quel un en-tête du même nom fourni par le client ; `X-Forwarded-For` est alors plus sûr ;
- Avec Cloudflare, choisissez `CF-Connecting-IP` comme en-tête d'IP réelle : Cloudflare y écrit directement l'adresse du visiteur. `X-Forwarded-For` est lu de droite à gauche en ignorant les proxys de confiance ; si les proxys de confiance contiennent toutes les plages de Cloudflare, une requête envoyée par la sortie propre de Cloudflare (comme Workers) peut porter une adresse usurpée à gauche ;
- `0.0.0.0/0` et `::/0` sont refusés à l'enregistrement ;
- Si le Routage par protocole transmet les connexions aux Services Web via la boucle locale, activez l'envoi de l'en-tête PROXY dans le Routage par protocole et sa réception sur l'écouteur au lieu d'ajouter `127.0.0.1` aux proxys de confiance ; sinon, un visiteur se connectant directement au Routage par protocole pourrait usurper les en-têtes. Les routes de transfert direct du Routage par protocole restent un simple relais TCP et ne sont pas affectées ;
- Les requêtes d'un proxy de confiance portant `X-Forwarded-Proto: https` sont traitées comme HTTPS ;
- Sans proxy de confiance, le comportement est inchangé. Les bannissements au niveau connexion (règles de pare-feu écrites par la Prévention des intrusions) visent la source TCP et sont sans effet sur les visiteurs derrière un CDN ; les bannissements propres aux Services Web s'appliquent à l'adresse réelle.

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
| `loginFromLocation` | Si `loginUrl` est vide et que le point répond 401 avec une seule `Location` http(s) absolue, le visiteur y est redirigé en 302, avec `Set-Cookie` et `Cache-Control`. Prévu pour les services qui placent une adresse de connexion dynamique dans le 401, comme le `/api/forward-auth` natif de Casdoor ; toute autre `Location` est ignorée et le 401 est renvoyé tel quel. Désactivé par défaut |
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

Sur un site, cochez « Utiliser les utilisateurs partagés » à côté de l'authentification web ou Basic. Les utilisateurs propres au site continuent de fonctionner. Vous pouvez aussi cocher des sites en masse dans la liste des sites de la page Utilisateurs d'auth, regroupée par écoute et filtrable : cocher un site sans authentification active aussi l'auth Web ; décocher un site qui ne s'authentifie qu'avec les utilisateurs partagés l'indique « Authentification désactivée à l'enregistrement », et Enregistrer liste ces sites pour confirmation ; une fois confirmé, leur authentification est désactivée et ils deviennent accessibles sans connexion ; les sites en authentification déléguée ne peuvent pas utiliser les utilisateurs partagés. Les modifications des utilisateurs, d'OIDC et des sites sur la page s'appliquent ensemble quand vous cliquez sur Enregistrer en bas ; si l'une d'elles échoue à la validation, rien n'est enregistré, et quitter la page avec des modifications non enregistrées demande d'abord une confirmation.

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

Le site par défaut se configure de la même façon : type **Proxy inverse** avec un backend, liste d’autorisation ou de blocage dans Contrôle d’accès par IP (les listes d’IP abonnées décrites plus bas sont utilisables) et **En cas de refus** sur **Fermer la connexion**. Les requêtes présentes dans la liste d’autorisation, ou absentes de la liste de blocage, sont transmises au backend et les autres coupées ; une liste abonnée rafraîchie s’applique immédiatement. Le type de site Fermer la connexion n’a pas de backend et coupe toutes les requêtes.

Fermer la connexion, comme type de site ou comme action de refus, retient seulement la réponse HTTP et ne masque pas que le service fonctionne :

- la poignée de main TCP est terminée, le client sait donc que le port écoute ;
- HTTP/1.x ferme la connexion, alors que HTTP/2 et HTTP/3 ne réinitialisent que la requête et gardent la connexion ouverte, réutilisable ;
- en HTTPS, les domaines sans certificat échouent déjà pendant la poignée de main TLS : les requêtes qui atteignent le site par défaut ont reçu le certificat et les noms de domaine qu'il contient.

Pour que les scanners ne voient ensuite que des délais d'attente, bannissez leurs adresses dans [Prévention d'intrusion](/fr/docs/modules/guard) ; avec la synchronisation du pare-feu, leur trafic est rejeté dans le noyau.

### Listes d'IP abonnées

Ajoutez des listes dans « Paramètres globaux → Listes d'IP abonnées » : indiquez l'URL de téléchargement et l'intervalle d'actualisation (24 heures par défaut) ; si l'URL (par exemple GitHub) est inaccessible en direct, choisissez le proxy global de mise à jour ou saisissez une adresse de proxy dans « Proxy de téléchargement ». Gately télécharge la liste périodiquement et la met en cache dans le répertoire de configuration, elle reste donc utilisable après un redémarrage sans accès à Internet. Le contenu comporte une IP, un CIDR ou une plage `début-fin` par ligne ; tout ce qui suit `#`, `;` ou `//` est un commentaire. Pour les plages d'IP des provinces et villes chinoises, vous pouvez par exemple utiliser `https://metowolf.github.io/iplist/data/cncity/440000.txt` (province du Guangdong) de [metowolf/iplist](https://github.com/metowolf/iplist).

- Les listes d'autorisation et de blocage des sites et des règles de chemin peuvent référencer plusieurs listes, vérifiées avec les plages saisies à la main : correspondre à l'une d'elles suffit pour être dans la liste ;
- Si le contenu téléchargé est illisible (par exemple une page d'erreur), la liste précédente est conservée, la raison est affichée sur la liste et un nouvel essai a lieu une heure plus tard ;
- Tant qu'une liste n'a pas été téléchargée, elle est traitée comme vide : une liste d'autorisation refuse alors l'accès, et une liste de blocage ne bloque rien de plus ;
- Désactiver une liste encore référencée fait refuser toutes les sources aux listes d'autorisation qui l'utilisent, et les listes de blocage qui l'utilisent ne bloquent plus aucune source ;
- La suppression d'une liste référencée est refusée.

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

La liste de bannissement dans « Paramètres globaux » affiche les IP actuellement bannies, la raison (échecs d'authentification répétés ou bannissement manuel) et l'heure d'expiration, et chaque entrée peut être levée avec « Débannir ». Vous pouvez aussi bannir manuellement : saisissez une IP et un nombre d'heures (1 à 720, soit 30 jours au maximum) ; l'API utilise 24 heures par défaut. Les adresses de bouclage et celles des proxys de confiance ne peuvent pas être bannies manuellement, pour la raison indiquée plus bas sous **Fermer et bannir**. Cochez **Permanent** pour bannir sans expiration (seconds à -1 dans l'API).

La liste de bannissement est partagée avec la [prévention d'intrusion](/fr/docs/modules/guard) : quand ce module est activé, cette section n'affiche que le nombre de bannissements et renvoie à sa page pour les gérer. Les bannissements s'appliquent aussi à la redirection de ports, à STUN et aux autres services, et sont rejetés dans le pare-feu du système quand la synchronisation est activée ; avec « Ignorer les adresses du LAN », ces adresses ne sont jamais bannies automatiquement.

### Filtre de pays GeoIP

Le « Filtre de pays » d'un site propose trois modes : désactivé, autoriser uniquement et refuser. Saisissez des codes pays ISO à deux lettres séparés par des virgules, comme `CN, HK, US`. Au moins un pays est requis lorsque le filtre est activé.

- Le filtrage dépend d'une base GeoIP, que vous devez d'abord importer sous « Paramètres globaux → Base GeoIP » : téléversez un fichier `.mmdb`, ou saisissez une URL de téléchargement (« Proxy de téléchargement » utilise par défaut le proxy global de mise à jour ; sans proxy de mise à jour configuré, la connexion est directe). MaxMind GeoLite2, DB-IP et d'autres bases de pays ou de villes sont pris en charge ; la limite de téléversement est de 129 Mo ;
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
- Avec le cache de réponses en mémoire activé, la clé de cache inclut la règle utilisateur choisie et, si [Transmettre l’identité](#identity-headers) est configuré, l'identité transmise au backend, afin que les réponses de backends ou d'utilisateurs différents ne se mélangent pas.

L'enregistrement vérifie que l'authentification est activée sur le site, que chaque règle comporte au moins un utilisateur et des backends valides, et qu'aucun utilisateur ne figure dans deux règles. Dans la liste des sites, un site doté de règles par utilisateur affiche une étiquette avec le nombre de règles.

### Transmettre l’identité {#identity-headers}

« Transmettre l’identité » place l'identité de l'utilisateur connecté dans des en-têtes de requête pour le backend, pour les applications qui identifient les utilisateurs par en-tête ; Firefly III, par exemple, identifie les comptes par `Remote-Email`. Chaque ligne choisit un champ et un nom d'en-tête :

| Champ | En-tête courant | Source |
|---|---|---|
| Nom d’utilisateur | `Remote-User` | Le nom de l'utilisateur du site ou de l'utilisateur partagé ; `clé-fournisseur:id` pour la connexion tierce ; `Remote-User` pour l'authentification déléguée |
| E-mail | `Remote-Email` | L'e-mail de l'utilisateur partagé, ou le `Remote-Email` recopié par l'authentification déléguée |
| Nom affiché | `Remote-Name` | Le nom affiché de l'utilisateur partagé, ou le `Remote-Name` recopié par l'authentification déléguée |
| Groupes | `Remote-Groups` | Les groupes de l'utilisateur partagé (séparés par des virgules), ou le `Remote-Groups` recopié par l'authentification déléguée |

- « Ajouter les en-têtes Remote-* courants » ajoute en une fois les lignes du tableau pour les champs que fournit la méthode de connexion actuelle ;
- L'e-mail, le nom affiché et les groupes ne peuvent être choisis que sur les sites utilisant des [utilisateurs partagés](#shared-users) ou l'authentification déléguée ; les utilisateurs du site et la connexion tierce n'ont qu'un nom d'utilisateur. Quand les informations d'un utilisateur partagé changent, les sessions ouvertes transmettent aussitôt les nouvelles valeurs ; un utilisateur du site portant le même nom qu'un utilisateur partagé ne transmet que le nom d'utilisateur lorsqu'il se connecte avec le mot de passe de l'utilisateur du site ;
- L'authentification déléguée ne fait confiance qu'aux en-têtes qu'elle recopie : si `Remote-Email` et consorts ne figurent pas dans la liste de copie, les mêmes en-têtes envoyés par le visiteur ne sont jamais pris comme identité ; si le service d'authentification ne renvoie pas `Remote-User`, rien d'autre n'est transmis ;
- Les mêmes en-têtes envoyés par le visiteur sont toujours supprimés, y compris les écritures avec un tiret bas à la place du tiret (comme `Remote_Email`), ils ne peuvent donc pas être falsifiés ; les visiteurs non connectés ou arrivant par un lien de partage ne reçoivent aucune identité, et un en-tête n'est pas envoyé si sa valeur est vide ;
- L'enregistrement vérifie que les noms d'en-tête sont valides et uniques (sans tenir compte de la casse, `-` et `_` étant considérés comme identiques), qu'ils ne font pas partie des en-têtes qui influent sur la transmission ou l'authentification, comme `Host`, `Cookie`, `Authorization` ou `X-Forwarded-For`, et qu'ils ne reprennent pas un en-tête de requête du site ou d'une règle de chemin, qui écraserait le résultat de l'authentification ; 16 lignes au plus ;
- « Transmettre le nom d’utilisateur » des versions précédentes apparaît comme une ligne de nom d'utilisateur lors de la modification du site et rejoint cette liste à l'enregistrement.

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
- Quand un backend est injoignable (connexion refusée, hôte inaccessible ou délai de connexion dépassé), la même requête est relancée aussitôt sur le backend suivant, d'abord les autres backends principaux puis les secours, avec au plus 3 backends. La requête n'a pas encore été envoyée à ce stade, renvoyer un POST ou une requête similaire est donc sûr. Les échecs après l'établissement de la connexion ne sont pas relancés, et le backend en échec compte toujours pour la vérification passive.

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

### Délais du backend {#backend-timeouts}

Délais des connexions d'un site proxy inverse vers ses backends, en secondes ; 0 utilise la valeur par défaut. Les backends par chemin et par utilisateur utilisent les mêmes réglages.

| Champ | Défaut | Plage | Signification |
| --- | --- | --- | --- |
| Connexion `timeouts.connect` | 10 s | 1 à 300 | Établir la connexion TCP avec le backend |
| Négociation TLS `timeouts.tlsHandshake` | 10 s | 1 à 300 | Terminer la négociation TLS avec un backend HTTPS |
| En-têtes de réponse `timeouts.responseHeader` | Aucune limite | 1 à 86400 | Attendre les en-têtes de réponse du backend une fois la requête et son corps envoyés |

- Le délai des en-têtes ne couvre que l'attente avant leur arrivée ; le corps de la réponse, les téléchargements et les connexions longues comme WebSocket et SSE ne sont pas concernés ensuite. Pour les gros envois, le décompte commence une fois le corps de la requête entièrement envoyé ;
- Un dépassement renvoie 504 (Gateway Timeout) ; les autres erreurs, comme une connexion refusée, renvoient 502. Les dépassements comptent pour la vérification passive comme les échecs de connexion ;
- Augmentez le délai de connexion pour les backends lents à démarrer ou derrière des liens lents ; définissez un délai d'en-têtes quand un backend bloqué doit échouer vite.

### Délais des visiteurs {#client-timeouts}

Délais d'inactivité envers les visiteurs, en secondes ; 0 utilise la valeur par défaut de 60 secondes (comme les valeurs par défaut de `client_body_timeout` et `send_timeout` dans nginx). Tous les types de site sauf Fermer la connexion peuvent les définir.

| Champ | Défaut | Plage | Signification |
| --- | --- | --- | --- |
| Corps de requête `clientTimeouts.readIdle` | 60 s | 1 à 3600 | Attente maximale d'une seule lecture du corps de requête |
| Réponse `clientTimeouts.writeIdle` | 60 s | 1 à 3600 | Attente maximale d'une seule écriture de la réponse |

- Seule une lecture ou écriture bloquée est chronométrée : les gros envois et téléchargements qui progressent, l'attente du backend et les connexions longues comme WebSocket ne sont pas concernés ;
- Les visiteurs qui envoient le corps ou lisent la réponse extrêmement lentement (slowloris, slow read) sont déconnectés et ne peuvent pas occuper les connexions longtemps ;
- Un dépassement à la lecture du corps renvoie 408 et ferme la connexion sans compter pour la vérification passive du backend ; un dépassement à l'écriture ferme la connexion ;
- S'applique à HTTP/1, HTTP/2 et HTTP/3.

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
- La clé de cache inclut Host, l'URI complète de la requête et `Accept-Encoding` ; avec la [répartition par utilisateur](#user-routes), elle inclut aussi la règle utilisateur choisie, ainsi que l'identité transmise au backend lorsque [Transmettre l’identité](#identity-headers) est configuré.

### Réécriture et redirection de chemin

Les règles sont évaluées dans l'ordre, et l'évaluation s'arrête à la première correspondance.

| Champ | Description |
| --- | --- |
| `pattern` | Une expression régulière comparée au chemin de la requête, comme `^/old/(.*)$` |
| `to` | Le résultat de remplacement ; utilisez `$1` pour référencer les groupes |
| `redirect` | Désactivé pour une réécriture interne (le chemin réécrit est envoyé au backend, et la cible doit commencer par `/`) ; activé pour une redirection |
| `code` | Code de statut de redirection 301 / 302 / 307 / 308, 302 par défaut |

Les redirections conservent les paramètres de requête d'origine. La cible d'une redirection peut contenir `{host}`, le domaine utilisé par le visiteur. Les règles de redirection s'exécutent avant l'authentification, comme `location = / { return 302 ...; }` de nginx, de sorte qu'un chemin tel que `/` peut renvoyer les visiteurs ailleurs sans leur demander de se connecter.

## Pages d'erreur {#error-pages}

L'onglet « Pages d'erreur » de l'éditeur de site détermine les pages 404 et 5xx que voient les visiteurs. Le style intégré de Gately est utilisé par défaut ; vous pouvez le remplacer par l'un des 10 styles intégrés (Minimal, Terminal, Néon, Ciel étoilé, Journal, Brutaliste, Aurore, Pixel, Plan, Chaîne d'état) ou utiliser votre propre page. Tous les styles s'adaptent aux écrans de téléphone, le texte suit la langue du navigateur du visiteur, et le bouton en forme d'œil en bas à droite de la carte permet de prévisualiser différents codes d'état en taille bureau et téléphone.

- Portée : les pages 404 et 5xx que Gately génère pour un site, par exemple le 502 quand le backend est injoignable, le 404 quand un fichier statique n'existe pas, le 503 quand la cible d'une redirection est momentanément indisponible, et l'erreur quand le service d'authentification de transfert est indisponible. Lorsqu'un site statique a une « Page 404 », elle reste prioritaire pour les fichiers introuvables. Le 403 du contrôle d'accès et le 429 de la limitation de débit ne sont pas concernés.
- Site par défaut : son réglage s'applique aussi au 404 affiché lorsqu'aucun site ne correspond au domaine demandé, même si le site par défaut est désactivé.
- Les clients autres que les navigateurs (`curl`, appels d'API, etc.) reçoivent toujours une erreur en texte brut, comme avant.

### Intercepter les erreurs du backend {#intercept-errors}

Un site de proxy inverse peut activer « Intercepter les erreurs du backend » pour remplacer les codes d'état indiqués renvoyés par le backend par la page choisie ci-dessus, comme `proxy_intercept_errors on` avec `error_page` dans nginx. Séparez les codes par des virgules ; les plages et les notations abrégées sont prises en charge, par exemple `404, 500-599` ou `5xx`, et seules les valeurs de 400 à 599 sont acceptées.

- Seules les requêtes par lesquelles le navigateur ouvre une page (`Accept` contient `text/html`) sont remplacées. Les requêtes d'API, d'images, de scripts, etc. gardent la réponse du backend, si bien que le 404 JSON d'une application monopage n'est jamais remplacé par une page web.
- Le code d'état et les en-têtes comme `Set-Cookie` et `Retry-After` sont conservés ; les en-têtes qui décrivent le corps d'origine (encodage du contenu, `ETag`, etc.) sont retirés. Les requêtes `HEAD` ne renvoient que les en-têtes.
- « Options en lot » permet de modifier le style de la page d'erreur de plusieurs sites à la fois ; le réglage d'interception de chaque site reste inchangé.

### Pages personnalisées {#custom-error-pages}

Les pages personnalisées sont conservées dans une bibliothèque de pages, gérée dans « Paramètres globaux → Pages d'erreur personnalisées » ou via « Gérer les pages personnalisées » dans l'onglet « Pages d'erreur » d'un site ; plusieurs sites peuvent partager une même page. Une page provient de l'une de ces deux sources :

- Téléversement ou collage : choisissez un fichier `.html` / `.htm` (UTF-8, 256 Ko maximum) ou collez directement le code. Le contenu est stocké dans la configuration et reste modifiable.
- Fichier du serveur : saisissez le chemin absolu d'un fichier `.html` / `.htm` (un chemin interne au conteneur avec Docker). Le fichier est vérifié à chaque utilisation pour détecter les modifications. Un lien symbolique doit lui aussi pointer vers un fichier HTML, et les fichiers dont le contenu ne ressemble pas à du HTML sont refusés. Si le fichier est supprimé ou si son contenu devient invalide, les visiteurs voient le style par défaut et la page est marquée comme indisponible dans la bibliothèque.

Une page peut utiliser des espaces réservés, remplacés à la sortie par les informations de l'erreur en cours et échappés en HTML :

| Espace réservé | Contenu |
| --- | --- |
| `{code}` | Code d'état, par exemple 502 |
| `{status}` | Libellé d'état en anglais, par exemple Bad Gateway |
| `{title}`, `{detail}` | Titre et description de l'erreur, dans la langue du visiteur |
| `{host}`, `{path}` | Domaine et chemin demandés |
| `{time}` | Heure de l'erreur (UTC) |

La sécurité et l'utilisabilité sont vérifiées automatiquement avant l'enregistrement, et un aperçu est affiché :

- Une page d'erreur est de même origine que le site. Pour éviter qu'elle serve à voler des cookies ou à falsifier des actions, l'enregistrement retire les scripts, les attributs d'événement comme `onclick`, les liens `javascript:`, les iframes, les objets de plugin, les formulaires, `<base>`, `<meta http-equiv>` (redirections automatiques, etc.) et les constructions de style dangereuses ; le résultat de la vérification énumère ce qui a été retiré.
- La sortie porte `Content-Security-Policy: sandbox; default-src 'none'; ...` : la page s'exécute dans un bac à sable isolé, si bien que des scripts injectés ne peuvent pas s'exécuter, et seuls les styles en ligne et les images ou polices intégrées en `data:` sont chargés, de sorte que l'adresse du visiteur ne fuit pas vers des tiers. Les feuilles de style, images et polices externes ainsi que les adresses relatives du site ne sont pas chargées, et la vérification le signale ; placez les styles dans `<style>` et intégrez les images et polices en `data:`.
- L'enregistrement est refusé lorsque le contenu est vide, n'est pas en UTF-8, dépasse la taille maximale ou n'est pas du HTML. L'absence de `viewport`, une déclaration d'encodage autre que UTF-8 ou l'absence d'espace réservé donnent lieu à une suggestion.

## Statistiques d'accès et supervision

### Statistiques de site {#site-stats}

Pour un écouteur en cours d'exécution, cliquez sur « Statistiques » sur la ligne d'un site pour ouvrir la fenêtre de statistiques, où vous pouvez choisir la dernière heure, les 6 dernières heures ou les 24 dernières heures :

- Statistiques d'accès à la minute : un point par minute, conservées sur les dernières 24 heures, avec une courbe du nombre de requêtes et des courbes d'erreurs 4xx / 5xx ;
- Temps jusqu'au premier octet : P50 / P95 par minute et le P95 maximal de la plage choisie. La latence va de l'arrivée de la requête à l'envoi des en-têtes de réponse, sans le transfert du corps ; WebSocket et les autres changements de protocole ainsi que les requêtes coupées ne sont pas comptés, et les centiles sont estimés à partir du temps moyen de chaque tranche fine. Pour les alertes par seuil, voir [Performances](/fr/docs/modules/perf) ;
- Répartition des codes de statut (2xx / 3xx / 4xx / 5xx) et trafic cumulé des réponses ;
- Top 10 des IP sources et top 10 des pays (les statistiques par pays nécessitent une base GeoIP importée) ;
- Les sites en proxy inverse affichent l'état de santé, le poids, l'indicateur de secours, le nombre de connexions actuelles et le nombre de requêtes transmises de chaque backend, y compris les backends par chemin et par utilisateur (marqués de leur chemin ou de leurs utilisateurs) ;
- Si des requêtes ont été bloquées, les totaux par motif : bannissement, liste d'IP, filtre pays, limitation de débit et WAF (les échecs de connexion ne sont pas comptés).

Les statistiques sont conservées en mémoire et repartent de zéro après un redémarrage ou après la modification des paramètres de l'écouteur lui-même. Modifier un site conserve ses compteurs. Les classements suivent au plus 5000 sources distinctes.

### Connexions en direct {#connections}

Pour un listener en cours d’exécution, chaque ligne de site (y compris le site par défaut) affiche « N actives » après ses statistiques : le nombre de requêtes que le site traite en ce moment, mis à jour au rafraîchissement de la page. Cliquez sur **Connexions** pour ouvrir la fenêtre des connexions ; elle s’actualise toutes les 2 secondes et permet de rechercher par IP :

- **En direct** : adresse et port source, protocole (h1 / h2 / h3, WebSocket marqué WS), requête (méthode, hôte et chemin, sans la chaîne de requête), heure de début, durée écoulée et trafic (descendant ; montant aussi en cas de corps de requête ou de WebSocket). Triées de la plus longue à la plus courte, 500 lignes au maximum. Avec des [proxys de confiance](#real-ip) configurés, la source est l’adresse réelle du visiteur et le proxy traversé est indiqué.
- **Historique** : connexions récemment terminées avec heures de début et de fin, durée, trafic et résultat (terminée / fermée). Seules les connexions ayant duré au moins 1 seconde ou fermées manuellement sont conservées, les 200 plus récentes par site, vidées au redémarrage ; consultez le journal d’accès pour les requêtes courtes.

La liste compte les requêtes en cours : une connexion HTTP/1.1 persistante n’appartient à aucun site entre deux requêtes, et une connexion HTTP/2 ou HTTP/3 peut porter plusieurs requêtes à la fois ; la requête est donc l’unité qui correspond exactement à un site. Un WebSocket mis à niveau reste listé jusqu’à la fermeture de la connexion.

**Fermer** sur une ligne termine cette requête dès confirmation : en HTTP/1.x toute la connexion est fermée ; en HTTP/2 et HTTP/3 seule cette requête est réinitialisée et les autres requêtes de la même connexion continuent ; un WebSocket mis à niveau est fermé avec sa connexion au backend. Cela prend aussi effet immédiatement quand le visiteur ne lit plus et que l’envoi est bloqué. Une requête fermée est journalisée en 444 (ou garde son code si la réponse avait commencé) et ne compte ni comme erreur du backend ni pour la détection de chemins. Le visiteur peut se reconnecter aussitôt ; pour l’en empêcher, utilisez **Fermer et bannir**, qui ajoute l’adresse source (sa plage /64 en IPv6) à la [liste de bannissement](#liste-de-bannissement) et ferme ses requêtes en cours sur tous les listeners et tous les sites. Les adresses de bouclage (les requêtes arrivées par frp ou par une redirection locale sans protocole PROXY apparaissent toutes avec elles) et les adresses comprises dans les proxys de confiance (par /64 en IPv6) ne peuvent pas être bannies, car les bannir bloquerait un grand nombre de visiteurs sans rapport ; fermer sans bannir n’a pas cette restriction. Le bannissement dure de 1 à 720 heures, ou cochez **Permanent**.

Les comptes et jetons en lecture seule peuvent consulter la liste ; fermer et bannir exigent l’accès en écriture et sont consignés dans le journal d’audit.

### Topologie réseau {#topology}

**Web → Topologie** dans le menu latéral est une page à part (`#/webtopology`) ; le bouton **Topologie** de l'en-tête de la page Services web l'ouvre aussi. Elle déroule Internet → Gately → écouteurs → sites / actions de l'écouteur → backends / issues, s'actualise toutes les 2 secondes, se met en pause lorsque la page est en arrière-plan, et le canevas suit le mode clair ou sombre du thème actuel :

- **Nœuds** : les écouteurs affichent leur adresse et leur état d'exécution, en rouge s'ils n'ont pas pu démarrer ; les sites indiquent leur mode de correspondance (exact, joker, tout domaine, site par défaut), leurs QPS et leurs requêtes en cours ; les sites en proxy inverse se déploient en tous leurs backends, y compris les backends par chemin et par utilisateur (marqués de leur chemin ou de leurs utilisateurs), les backends indisponibles étant en rouge ; les sites de redirection, de fichiers statiques, de texte fixe et de fermeture de connexion mènent chacun à un nœud d'issue. Un site par défaut qui ferme les connexions, les requêtes HTTP en clair redirigées sur un port HTTPS (301 / 308) et le 404 renvoyé lorsqu'aucun site ne correspond et qu'aucun site par défaut n'est activé sont des comportements attendus et apparaissent en gris. Les écouteurs et sites désactivés sont aussi listés, affichés comme arrêtés.
- **Liens** : l'épaisseur augmente avec les QPS, et les liens qui portent du trafic affichent une animation de flux. Au-delà de 120 liens, seul le chemin mis en évidence est animé, et rien n'est animé lorsque le système demande de réduire les animations.
- **Mise en évidence** : survoler ou cliquer un nœud met en évidence tout son chemin, en remontant jusqu'à Internet et en descendant jusqu'aux backends, affiche les QPS et le trafic sur les liens de ce chemin et estompe le reste ; cliquez sur une zone vide pour réinitialiser.
- **Panneau de détails** : pour le nœud sélectionné, les QPS (moyenne des 10 dernières secondes), la moyenne sur 60 secondes, le trafic descendant, les requêtes en cours, le total des requêtes, les erreurs de backend, les requêtes bloquées par motif, la répartition des codes de statut, les courbes par seconde des requêtes et du trafic sur les 60 dernières secondes, et les IP actives qui y sont passées. Le ⓘ à côté de chaque métrique explique comment elle est mesurée. Pour un site, le panneau ouvre aussi ses fenêtres de statistiques, de connexions, de journal d'accès et de modification ; un backend ouvre le journal d'accès et l'éditeur de son site, et un écouteur se modifie directement.
- **Consulter le journal d'accès** : après avoir sélectionné un nœud de site ou de backend, **Journal d'accès** ouvre le journal d'accès de ce seul site (correspondance exacte du nom du site). Si vous avez déjà filtré sur une IP en haut de la page, le bouton devient **Journal d'accès de cette IP** et filtre aussi sur cette adresse. L'icône de journal à droite de chaque IP active du panneau affiche le journal d'accès de cette adresse sur le site sélectionné ; si le nœud sélectionné n'est pas de la famille des sites, elle l'affiche sur tous les sites. Les conditions de filtre de la fenêtre de journal restent modifiables, et la condition de site apparaît sous forme d'étiquette « Site : … » amovible. La fenêtre ne montre en outre que les visiteurs de l'écouteur du site : un site du même nom sur un autre écouteur (par exemple des écouteurs IPv4 et IPv6 séparés tous deux nommés HTTPS) ne s'y mélange pas, et cette condition apparaît sous forme d'étiquette « Écouteur : … » amovible. Seuls les sites dont le journal d'accès est activé disposent d'un historique complet ; s'il est désactivé, la fenêtre l'indique en haut. Un site sans nom est filtré sur son premier domaine. Le bouton **Journaux** de chaque ligne de site de la page Service Web ouvre la même fenêtre.
- **IP actives** : les adresses des visiteurs ayant envoyé des requêtes au cours des 60 dernières secondes, plus celles qui ont encore des requêtes en cours. Le canevas n'affiche que des nombres ; les adresses complètes sont listées dans le panneau de détails. Saisissez ou choisissez une adresse en haut (ou cliquez-en une dans le panneau) pour ne mettre en évidence que le chemin qu'elle a suivi, jusqu'au backend exact ; **Effacer le filtre** réinitialise. Chaque site, backend et action d'écouteur garde jusqu'à 1024 adresses récentes et l'API en renvoie au plus 500 ; avec trop de sources, le panneau signale que la liste est incomplète. Avec des [proxys de confiance](#real-ip) configurés, ce sont les adresses réelles des visiteurs.
- **Masquer** : interrupteur activé, les catégories choisies s’affichent avec `*` pour partager des captures ; cela ne touche que cette page et la recherche utilise toujours le texte complet. Les catégories sont les IP des visiteurs (liste des IP actives et filtre ; la dernière partie d’une IPv4, les 64 derniers bits d’une IPv6), les adresses des backends (IP et noms d’hôte des backends et des cibles de redirection), les adresses d’écoute (IP d’écoute, messages d’erreur de l’écouteur et sites utilisant une IP comme domaine) et les domaines (domaines des sites et domaines présents dans leurs noms ; le premier libellé et le suffixe restent visibles, par exemple `nas.***.com`). Les trois premières sont cochées par défaut et le choix est mémorisé.
- **Mode de masquage** : en haut du menu Masquer, choisissez **Astérisques** ou **Alias**. Les astérisques cachent une partie de la valeur et gardent le réseau et les extrémités ; les alias remplacent la valeur entière par un numéro constant sur cette page : les IP des visiteurs deviennent `Visitor-01`, les IP des backends et des cibles de redirection `Backend-01`, les adresses d’écoute `Listen-01`, et les domaines ne gardent que le suffixe, la partie enregistrable et la partie hôte étant numérotées séparément : `nas.home.example.com` devient `host-01.domain-01.com` et les sous-domaines d’un même domaine partagent son numéro `domain`. Une même valeur reçoit toujours le même alias et des valeurs différentes des alias différents, on voit donc toujours quels nœuds sont identiques. Les numéros sont attribués dans l’ordre d’apparition, conservés uniquement dans la page ouverte et réattribués après un rechargement.
- **Filtrer** : **Filtrer**, en haut à droite, limite l’affichage à certains écouteurs ; **Problèmes uniquement** affiche les écouteurs qui n’ont pas démarré (avec tous leurs sites), les sites activés mais arrêtés et les sites dont un backend est en échec, les écouteurs et sites désactivés étant considérés comme attendus ; **Avec blocages** affiche, par motif (banni, liste d’IP, filtre par pays, limitation de débit, WAF), seulement les sites qui ont bloqué des requêtes pour ce motif depuis le démarrage de Gately. Les conditions s’appliquent ensemble et se combinent avec la recherche de l’en-tête ; filtrer par problèmes ou blocages masque les actions au niveau de l’écouteur et les écouteurs sans site restant. Les filtres ne valent que tant que la page reste ouverte.
- **Actifs uniquement** : avec beaucoup de services, cliquez sur **Actifs uniquement** en haut à droite pour n’afficher que les sites et actions d’écouteur ayant reçu des requêtes au cours des 60 dernières secondes, ayant des requêtes en cours ou traversés par des IP actives ; les sites proxy ne gardent que leurs backends actifs (un site dont les backends n’ont reçu aucune requête, par exemple bloquée par le contrôle d’accès, s’affiche comme nœud terminal) et les écouteurs sans activité sont masqués. Dans ce mode, le repli des écouteurs et les nœuds de synthèse « N de plus » ne s’appliquent pas, et le canevas s’ajuste pour afficher ces nœuds en entier (au moins 50 % pour Arbre, Vertical et Sankey), en se mettant à jour quand des services deviennent actifs ou inactifs. Le nombre sur le bouton indique les sites actifs en ce moment, suit le filtre et la recherche, et reste affiché même mode désactivé. Le mode se combine avec le filtre, la recherche, la mise en évidence par IP et toutes les dispositions, et il est mémorisé par appareil ; si rien n’a reçu de requête depuis 60 secondes, le canevas l’indique et **Tout afficher** quitte le mode.
- **Relecture** : cliquez sur **Relecture** en haut à droite pour ouvrir une frise chronologique et revoir la topologie à un instant passé. L’enregistrement est désactivé par défaut ; un administrateur l’active avec **Démarrer l’enregistrement** sur la frise ou dans **Paramètres d’enregistrement**. Une fois activé, toute la topologie (débits, compteurs, santé des backends, adresses IP actives) est enregistrée sous forme d’image toutes les 5 secondes, compressée et conservée uniquement en mémoire dans la **Limite de mémoire** (8 Mio par défaut, de 1 à 256), les images les plus anciennes étant supprimées au-delà ; les images consécutives identiques (par exemple sans trafic) ne sont stockées qu’une fois, si bien qu’avec peu de trafic on remonte loin et avec beaucoup de trafic moins loin. La frise trace le trafic en graphique en aires, marque en rouge en bas les périodes où un écouteur n’a pas démarré ou un backend était défaillant, et grise les périodes sans enregistrement (module désactivé ou aucun écouteur actif). Cliquez ou faites glisser la frise pour vous positionner, survolez-la pour voir le QPS et les adresses IP actives à cet instant ; avancez image par image avec les boutons ou ← →, appuyez sur Espace pour lire ou mettre en pause, choisissez une vitesse de 1× à 60× et cochez **Sauter l’inactivité** pour passer les périodes sans trafic ni visiteurs ; la frise peut couvrir tout l’enregistrement ou les 15 dernières minutes jusqu’à 24 heures, mémorisé par appareil. Pendant la relecture, les sept dispositions, le filtre, Actifs seulement, le masquage et la mise en évidence par IP fonctionnent normalement, et la courbe du panneau de détails couvre les 60 secondes précédant l’instant choisi (un point toutes les 5 secondes) ; **Connexions** reflète l’état actuel et n’est donc pas affiché en relecture. Cliquez sur **Revenir au direct** pour quitter. Les données enregistrées contiennent les adresses IP des visiteurs et sont effacées au redémarrage de Gately ou à la désactivation ; enregistrer une image prend environ 0,4 ms (mesuré avec 40 sites et 200 adresses IP actives) pour environ 1 à 2 Ko après compression.
- **Disposition** : le menu de disposition en haut à droite en propose sept ; le choix est mémorisé, et le changement se fait en fondu avec un réajustement du canevas. **Arbre** dispose cinq colonnes de gauche à droite, le plus détaillé ; **Vertical** se déploie de haut en bas comme un organigramme, s’ouvre centré sur Gately et se parcourt en faisant glisser sur les côtés ; **Radial** place Gately au centre avec les écouteurs, les sites et les backends sur trois anneaux concentriques et les noms le long du rayon ; **Sankey** dessine chaque nœud comme une barre et chaque lien comme une bande, découpe la barre du parent selon la part de QPS de chaque enfant, la hauteur des barres et la largeur des bandes suivant le trafic tandis que les nœuds restent en place ; **Sunburst** place Gately au centre avec les écouteurs, les sites et les backends en trois anneaux de segments ; **Treemap** et **Bulles** imbriquent Gately, les écouteurs, les sites et les backends en blocs ou en cercles, chaque backend (ou issue) ayant la même surface. Sunburst, Treemap et Bulles n’ont pas de liens et sont plus sombres quand le trafic est élevé, pratiques pour un aperçu de nombreux sites. Sankey, Sunburst, Treemap et Bulles n’ont pas de bouton de repli sur le canevas ; repliez ou dépliez un écouteur depuis son panneau de détails.
- **Nœuds en trop repliés** : par défaut chaque écouteur affiche ses 8 premiers sites et chaque site ses 6 premiers backends ; les autres sont regroupés dans un nœud « N sites / backends de plus » qui affiche leur QPS total et les requêtes en cours. Cliquez dessus pour tout déplier, puis sur le nœud **Replier** ajouté à la fin pour les regrouper à nouveau. Les IP actives des nœuds repliés comptent pour ce nœud et la mise en évidence par IP s'arrête sur lui. Les sites et backends ayant reçu des requêtes au cours des 60 dernières secondes ou ayant des requêtes en cours ne sont jamais repliés : ils restent visibles dans leur ordre d’origine même au-delà de la limite, si bien que le nœud de synthèse ne regroupe que les inactifs et que son sous-titre indique « Aucune requête au cours des 60 dernières secondes ».
- **Déplacement, zoom et taille du canevas** : faites glisser le canevas pour le déplacer et utilisez la molette pour monter ou descendre (la page défile une fois le bord atteint) ; maintenez Ctrl/⌘ en faisant défiler, ou pincez sur le pavé tactile ou l’écran tactile, pour zoomer entre 15 % et 250 % autour du pointeur ou du centre du pincement. Les commandes en bas à droite zooment, dézooment, reviennent à 100 % et ajustent au canevas (tout afficher, icône de cible) ; le bouton le plus à droite étend le canevas à toute la fenêtre du navigateur, et Échap ou le même bouton le rétablit. Faites glisser la barre sous le canevas pour changer sa hauteur (fonctionne aussi au toucher) ; la hauteur est mémorisée par appareil, et un double-clic sur la barre ou **Hauteur automatique** revient au dimensionnement automatique.
- Le champ de recherche de l'en-tête filtre les sites dans la topologie (nom, domaine, groupe, adresse de backend) ; les écouteurs se replient un par un ou tous à la fois. **Modifier** et **Modifier l'écoute** dans le panneau de détails ouvrent sur cette page le même éditeur que la page Service Web ; après l'enregistrement, la topologie se met à jour immédiatement sans quitter la page. Les fenêtres de statistiques, de connexions et de journal d'accès s'ouvrent aussi ici.

Tous les chiffres proviennent des compteurs en mémoire de cette instance : ils repartent de zéro au redémarrage, modifier un site conserve ses compteurs, modifier les paramètres d'un écouteur comme le port remet à zéro les compteurs de cet écouteur, et les autres instances de Gately ne sont pas agrégées. La vue lit `GET /api/webservice/topology`, accessible aux comptes et jetons en lecture seule. La relecture lit `GET /api/webservice/topology/replay` (vue d’ensemble et frise) et `GET /api/webservice/topology/replay/frame` (l’instantané d’un instant), accessibles aussi aux comptes en lecture seule ; modifier les paramètres d’enregistrement passe par `PUT /api/webservice/topology/replay/settings` et nécessite le droit d’écriture.

### Audit des visiteurs {#visitor-audit}

L'entrée « Web → Audit des visiteurs » (`#/webvisitors`) regroupe les accès à tous les sites par adresse du visiteur : qui a visité quels sites et chemins, quand, sous quelle identité et combien de fois il a été bloqué. Tous les sites sont comptés, y compris le site par défaut et les requêtes qui ne correspondent à aucun site, que le journal d'accès du site soit activé ou non ; avec des [proxys de confiance](#real-ip), l'adresse réelle du visiteur est enregistrée.

- **Résumé et tendance** : choisissez la période dans l'en-tête (aujourd'hui, 7 jours, 30 jours, toute la durée de conservation). Les cartes affichent les visiteurs (dont les nouveaux), les requêtes, les blocages et leur part, les visiteurs authentifiés sur un site, les visiteurs bannis actuellement et les adresses de type scanner ; cliquez sur une carte pour filtrer, puis à nouveau pour annuler. Dans l'histogramme quotidien, la hauteur est le nombre de requêtes et la partie rouge les blocages ; cliquez ou survolez un jour pour voir ses visiteurs, requêtes et blocages.
- **Liste** : chaque visiteur indique le code pays (avec une base GeoIP importée ; les adresses privées et locales affichent « LAN »), les requêtes et blocages de la période, les sites les plus visités, les identités, la première et la dernière visite et la dernière requête (statut, méthode, chemin, domaine et User-Agent). Recherche par adresse, identité, site, domaine, chemin ou User-Agent ; filtres « Avec blocages », « Authentifiés », « Administrateurs », « Bannis », « Type scanner » et par site ; tri par dernière visite, plus de requêtes, plus de blocages ou première visite. Les sites de même nom sur deux écoutes portent le nom de l'écoute.
- **Identité** : après une authentification réussie sur le site (connexion web, authentification basique, utilisateurs partagés, connexion tierce, authentification déléguée), le nom d'utilisateur est enregistré ; les requêtes portant une session de la console d'administration de cette instance sont enregistrées comme « admin ». Les tentatives avec un mauvais mot de passe et les requêtes bloquées n'enregistrent aucune identité.
- **Type scanner** : adresses uniquement bloquées ou n'ayant visité que des sites non reconnus, jamais authentifiées.
- **Détail** : cliquez sur une ligne pour ouvrir le visiteur : première et dernière visite, total des requêtes, blocages, octets envoyés, connexions fermées (444), histogramme quotidien sur la durée de conservation, répartition des statuts et motifs de blocage (bannissement, liste d'IP, filtre pays, limitation de débit, WAF), sites visités, chemins les plus demandés (10 au maximum), identités et User-Agent. Vous pouvez bannir ou lever le bannissement (IPv6 par /64), voir le journal d'accès de cette adresse ou supprimer l'enregistrement.

**Ce qui est conservé et où** : seuls des compteurs agrégés par visiteur sont stockés, jamais les requêtes individuelles ; seule la partie chemin est enregistrée, et chaînes de requête, corps de requête, cookies et identifiants restent toujours exclus. Chaque visiteur conserve jusqu'à 12 sites, 10 chemins, 6 identités et 4 User-Agent ; au-delà, l'entrée ayant le moins de requêtes est remplacée. Pour les requêtes individuelles, activez le journal d'accès du site. Les données sont dans `visitors/visitors.jsonl.gz` sous le répertoire de configuration (droits 0600), écrites environ 10 minutes après un changement et immédiatement à l'arrêt, et ne font pas partie des sauvegardes de configuration.

**Paramètres** : « Paramètres » dans l'en-tête désactive l'enregistrement (les données existantes sont conservées), règle la conservation en jours (1-180, 30 par défaut) et le nombre maximal de visiteurs (100-100000, 10000 par défaut), et permet d'effacer toutes les données. Les visiteurs dont la dernière visite dépasse la conservation sont supprimés automatiquement ; réduire la conservation supprime immédiatement les données en trop, après confirmation. Au-delà de la limite, les adresses de type scanner partent en premier, puis celles absentes depuis le plus longtemps. Les comptes et jetons en lecture seule peuvent consulter ; bannir, supprimer et modifier les paramètres demandent l'accès en écriture. API : `GET /api/webservice/visitors`, `GET /api/webservice/visitors/detail`, `POST /api/webservice/visitors/delete` et `GET|PUT /api/webservice/visitors/settings`.

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
| `gately_site_blocked_total` | counter | Requêtes bloquées par motif (étiquette `reason` : `ban` bannissement, `ip` liste d'IP, `country` filtre pays, `rateLimit` limitation de débit, `waf`) |
| `gately_upstream_up` | gauge | Indique si un backend est sain : 1 sain, 0 indisponible (avec l'étiquette `upstream`) |
| `gately_upstream_requests_total` | counter | Requêtes transmises à un backend (avec l'étiquette `upstream`) |
| `gately_upstream_response_bytes_total` | counter | Octets du corps de réponse renvoyés par un backend, hors données après passage en WebSocket (avec l'étiquette `upstream`) |
| `gately_listener_requests_total` | counter | Requêtes traitées par l'écouteur lui-même sans site, étiquette `action` : `https_redirect` (HTTP en clair redirigé sur un port HTTPS) ou `not_found` (404 lorsqu'aucun site ne correspond) ; ne porte que l'étiquette `listener` |

À l'exception de `gately_listener_requests_total`, les métriques portent les étiquettes `listener` (nom de l'écouteur#clé de l'écouteur) et `site` (nom du site, ou premier domaine s'il n'a pas de nom). Les backends par chemin et par utilisateur portent en plus une étiquette `route` sur `gately_upstream_*` : le préfixe de chemin, ou `@` suivi de la liste des utilisateurs. L'étiquette `upstream` est l'adresse du backend, débarrassée de son nom d'utilisateur, de son mot de passe et de sa chaîne de requête.

## Importer une configuration nginx {#import-nginx}

Le bouton « Importer nginx » en haut de la page convertit des sites nginx existants en écouteurs et sites Gately. Le plus simple est d'exécuter `nginx -T` sur l'hôte nginx et d'en coller la sortie complète : elle contient tous les fichiers chargés par include, et Gately développe les include comme nginx.

- Aperçu d'abord : les sites sont regroupés par port d'écoute, avec pour chacun le type, le backend ou le répertoire, le fichier source et le numéro de ligne, ainsi que les points d'attention (directives utilisant des variables, location en expression régulière, domaines sans certificat, etc.) ; les sites non convertibles sont listés avec la raison ;
- Les sites d'un même port sont ajoutés à l'écouteur existant, sinon un nouvel écouteur est créé ; les domaines déjà présents sont ignorés, l'import peut donc être relancé sans risque ;
- Par défaut, les sites sont importés désactivés, pour être vérifiés un par un en parallèle de nginx avant activation ; cochez « Activer dès l'import » pour qu'ils s'appliquent immédiatement (si nginx occupe encore le même port, le nouvel écouteur ne démarre pas) ;
- Éléments convertis : `proxy_pass` (y compris poids, secours et politique de répartition de charge des upstream), répertoires statiques `root` / `alias` (`autoindex`, `try_files $uri.html`, `error_page 404`), redirections et contenu fixe `return`, redirections `rewrite` de tout le site, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, backends par chemin, contrôle d'accès et redirections au niveau des location, `listen ... proxy_protocol` et `quic` ;
- `proxy_connect_timeout` et `proxy_read_timeout` deviennent les [délais de connexion et d'en-têtes de réponse](#backend-timeouts) du site ; la valeur de location / l'emporte sur le niveau server et les valeurs au-delà de la limite sont importées à la limite. Les valeurs différentes dans d'autres locations ne sont pas importées, l'aperçu le signale ;
- `client_body_timeout` et `send_timeout` deviennent les [délais des visiteurs](#client-timeouts) du site, pour tous les types de site ;
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
| `error_page 502 503 504 /50x.html` et `proxy_intercept_errors on` | le réglage des [pages d'erreur](#error-pages) du site et l'interception des erreurs du backend |
| `add_header` | en-têtes de réponse |
| `return 444` | un site Fermer la connexion |
| `error_page 497 https://$host:$server_port$request_uri` dans un server HTTPS | « HTTP sur le même port » d'un nouvel écouteur réglé sur « Rediriger vers HTTPS » ; en cas de fusion avec un écouteur existant, seul un avertissement s'affiche et ses réglages restent inchangés |

## Ouvrir le port dans le pare-feu {#auto-firewall}

Quand Gately tourne sur un routeur comme OpenWrt ou sur un hôte Linux avec pare-feu, le pare-feu du système bloque par défaut les connexions venant de l’extérieur, et le port d’une écoute doit être autorisé pour être joignable. Avec « Ouvrir le port dans le pare-feu » activé sur une écoute, Gately autorise dans le pare-feu de cet hôte uniquement le port de cette écoute :

- Seuls les ports réellement en écoute sont autorisés. Si le port est pris par un autre programme et que l’écoute ne démarre pas, rien n’est autorisé et ce programme (la page d’administration du routeur, par exemple) reste fermé à l’extérieur.
- Le réseau de l’écoute fixe la famille d’adresses : « IPv6 uniquement » autorise IPv6, « IPv4 uniquement » autorise IPv4, la double pile autorise les deux. Avec une adresse d’écoute renseignée, seules les connexions vers cette adresse sont autorisées. Les écoutes sur 127.0.0.1 ou ::1 n’ont besoin de rien.
- Avec HTTP/3 activé, le port UDP de même numéro est aussi autorisé.
- La règle est retirée quand l’écoute est désactivée, supprimée, ne démarre pas ou que l’interrupteur est coupé, ainsi qu’à l’arrêt de Gately.
- nftables (OpenWrt fw4, firewalld, etc.) : la règle est insérée en tête des chaînes de filtrage input. iptables / ip6tables (OpenWrt fw3, ufw, etc.) : en tête de la chaîne INPUT. Les règles portent le commentaire `gately-open:` et sont rétablies dans les 30 secondes après un rechargement du pare-feu.
- Les noyaux sans nf_tables (comme Synology DSM) utilisent iptables (legacy) : les règles sont placées dans une chaîne filter propre à Gately (commençant par `GTLY_OPEN_`), vers laquelle saute la première règle d'INPUT ; si le pare-feu du système place ses règles devant, le saut est remis en tête dans les 30 secondes. L'image Docker inclut iptables (legacy).
- Linux uniquement. Sous Docker, il faut le réseau host et `cap_add: NET_ADMIN`. La carte de l’écoute affiche le résultat ; en cas d’échec, l’étiquette « Pare-feu » passe en orange.

Les règles de [Redirection de ports](/fr/docs/modules/portforward#auto-firewall) et de [Routage par protocole](/fr/docs/modules/protoroute#auto-firewall) ont le même interrupteur, et les règles [STUN](/fr/docs/modules/stun#auto-firewall) autorisent leur port de canal.

### Autoriser les ports à la main {#manual-firewall}

Pour autoriser des ports à la main, ajoutez une règle pour chaque port à exposer et laissez l’entrée de la zone wan rejetée. Passer toute l’entrée de la zone wan en acceptation expose aussi à Internet la page d’administration du routeur (80 / 443), SSH et d’autres services. Sur OpenWrt, ajoutez-la dans Réseau → Pare-feu → Règles de trafic, ou dans `/etc/config/firewall` :

```
config rule
	option name 'Allow-Gately-HTTPS'
	option src 'wan'
	option family 'ipv6'
	option proto 'tcp'
	option dest_port '50663'
	option target 'ACCEPT'
```

- Si Gately tourne avec le réseau bridge de Docker, le conteneur ne voit pas le pare-feu du routeur ; autorisez le port mappé sur l’hôte ou le routeur.
- Si Gately tourne sur un autre appareil du LAN (par exemple un conteneur avec sa propre adresse IPv6 via macvlan), le trafic est transféré par le routeur : ajoutez sur le routeur une règle de transfert avec lan comme zone de destination, l’adresse de l’appareil et les ports utilisés. Le préfixe IPv6 attribué par le FAI change ; faites correspondre l’adresse par son suffixe, par exemple `option dest_ip '::1234/::ffff:ffff:ffff:ffff'`.

## Conseil de déploiement

Les Services Web doivent écouter sur des ports de l'hôte tels que 80 / 443. Pour un déploiement Docker, `--network host` est recommandé.
