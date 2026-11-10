# Outils de fichiers et d'exploitation

## WebDAV

Configurez-le dans « Service WebDAV » du groupe « Services de fichiers » de la barre latérale.

| Champ | Par défaut | Description |
| --- | --- | --- |
| `enable` | Désactivé | Active le service ; au moins un utilisateur est requis lorsqu'il est activé |
| `listenIp` | Vide | Adresse d'écoute ; laissez vide pour écouter sur toutes les adresses |
| `port` | 18800 | Port d'écoute |
| `tls` | Désactivé | Active HTTPS ; le certificat est fourni par SNI par le module [Certificats SSL](/fr/docs/modules/ssl) |
| `users` | Aucun | Plusieurs utilisateurs. Chacun possède `name`, `password`, `root` (un chemin de répertoire absolu) et `readOnly`, ou utilise `mounts` pour monter plusieurs répertoires |
| `sharedUsers` | Désactivé | Autorise les [utilisateurs partagés](#shared-users) à se connecter à ce service |

Chaque utilisateur peut être associé à un seul répertoire, ou monter plusieurs répertoires :

- **Répertoire unique** : définissez `root` et `readOnly`. Après la connexion, la racine est ce répertoire.
- **Plusieurs répertoires** : cliquez sur « Monter plusieurs répertoires » dans l'interface, ou renseignez plusieurs entrées dans `mounts`. Chaque entrée comporte `name` (nom affiché, ne peut pas contenir `/`, unique pour un même utilisateur), `path` (un chemin absolu) et `readOnly`. Après la connexion, la racine liste les répertoires par nom. La racine elle-même n'est pas modifiable, et les fichiers ne peuvent pas être déplacés d'un répertoire à un autre.

## FTP

Configurez-le dans « Service FTP » du groupe « Services de fichiers ». FTP et FTPS sont pris en charge.

| Champ | Par défaut | Description |
| --- | --- | --- |
| `enable` | Désactivé | Active le service ; au moins un utilisateur est requis lorsqu'il est activé |
| `listenIp` | Vide | Adresse d'écoute ; laissez vide pour écouter sur toutes les adresses |
| `port` | 2121 | Port d'écoute |
| `passiveStart` / `passiveEnd` | 0 | Plage de ports de données du mode passif. Si les deux valent 0, le système attribue les ports aléatoirement ; lorsqu'ils sont définis, 1 ≤ start ≤ end ≤ 65535 doit être respecté, et la plage doit être autorisée dans le pare-feu et les redirections de ports |
| `publicHost` | Vide | IP annoncée à l'extérieur en mode passif. Derrière un NAT, saisissez l'IP publique ou redirigée. Seules les adresses IP sont prises en charge ; laissez vide pour utiliser automatiquement l'adresse de connexion |
| `tlsMode` | 0 | 0 autorise à la fois le texte clair et FTPS, 1 exige FTPS explicite (`AUTH TLS`), 2 est FTPS implicite (chiffré dès la connexion). S'il est différent de 0, le certificat est fourni par SNI par le module [Certificats SSL](/fr/docs/modules/ssl) |
| `idleTimeout` | 900 | Secondes d'inactivité avant la fermeture d'une connexion |
| `connectionTimeout` | 30 | Délai en secondes pour établir une connexion de données |
| `disableActiveMode` | Désactivé | Utilise uniquement le mode passif, mieux adapté aux environnements NAT |
| `users` | Aucun | Plusieurs utilisateurs, avec les mêmes champs que WebDAV : `name`, `password`, `root`, `readOnly` ou `mounts` ; les noms d'utilisateur doivent être uniques |
| `sharedUsers` | Désactivé | Autorise les [utilisateurs partagés](#shared-users) à se connecter à ce service |

## SMB

Configurez-le dans « Service SMB » du groupe « Services de fichiers » pour partager des répertoires locaux avec l’Explorateur Windows, le Finder de macOS, Linux et les clients SMB des téléphones et téléviseurs. L’implémentation du protocole est intégrée à Gately (basée sur [gosamba](https://github.com/ahmetozer/gosamba)) et prend en charge SMB 2.0.2 à 3.1.1, l’authentification NTLMv2 et la signature, avec un chiffrement SMB 3 obligatoire en option ; SMB 1 n’est pas pris en charge. Disponible uniquement sous Linux et macOS.

| Champ | Par défaut | Description |
| --- | --- | --- |
| `enable` | Désactivé | Active le service ; au moins un utilisateur est requis lorsqu'il est activé |
| `listenIp` | Vide | Adresse d'écoute ; laissez vide pour écouter sur toutes les adresses |
| `port` | 445 | Port d'écoute. Windows se connecte uniquement au 445 ; si le Samba fourni avec un NAS comme fnOS ou Synology utilise déjà le 445, choisissez un autre port, que macOS, Linux et la plupart des lecteurs peuvent indiquer dans l’adresse |
| `requireEncryption` | Désactivé | Exige le chiffrement SMB 3 pour chaque connexion ; les anciens appareils et certains téléviseurs limités à SMB 2 ne pourront plus se connecter |
| `mdns` | Désactivé | Annonce le service via Bonjour pour qu’il apparaisse dans Réseau du Finder de macOS ; sous Docker, cela ne fonctionne qu'en réseau host |
| `users` | Aucun | Plusieurs utilisateurs avec les mêmes champs que WebDAV : `name`, `password`, `root`, `readOnly` ou `mounts` ; les noms d’utilisateur sont insensibles à la casse et uniques |
| `sharedUsers` | Désactivé | Autorise les [utilisateurs partagés](#shared-users) à se connecter à ce service |

Les partages appartiennent aux utilisateurs, et un utilisateur connecté ne voit et n’ouvre que les siens :

- **Répertoire unique** : le partage porte le nom de l’utilisateur, par exemple `\\192.168.1.2\alice` pour l’utilisateur `alice`.
- **Plusieurs répertoires** : chaque répertoire est un partage dont `name` est le nom (insensible à la casse et unique par utilisateur), et chacun peut être en lecture seule. Des utilisateurs différents peuvent avoir des partages du même nom, chacun pointant vers son propre répertoire.

Les noms d’utilisateur et de partage ne peuvent pas contenir `" / \ [ ] : ; | = , + * ? < >` ni commencer ou se terminer par `.`. L’authentification NTLM a besoin du mot de passe en clair : les utilisateurs dont le mot de passe est stocké sous forme de hachage bcrypt doivent le redéfinir. Les fichiers sont lus et écrits sous l’identité du processus Gately.

« Adresses de connexion », en bas de la page, liste l’adresse de chaque partage d’après les paramètres enregistrés :

- Windows : saisissez `\\hôte\partage` dans la barre d’adresse de l’Explorateur, ou exécutez `net use Z: \\hôte\partage /user:utilisateur`
- macOS : dans le Finder, choisissez Aller → Se connecter au serveur et saisissez `smb://hôte:port/partage` (le port peut être omis pour 445)
- Linux : `mount -t cifs //hôte/partage /mnt -o username=utilisateur,port=port`

Pour diagnostiquer un client qui n'arrive pas à se connecter ou à s'authentifier, définissez la variable d'environnement `GATELY_SMB_DEBUG=1` et redémarrez Gately : les journaux de débogage du protocole sont écrits dans le journal d'exécution. Retirez la variable une fois terminé.

## Utilisateurs partagés {#shared-users}

WebDAV, FTP et SMB partagent une liste d’« utilisateurs partagés » avec les mêmes champs que les utilisateurs locaux de chaque service (nom, mot de passe, `root` et `readOnly`, ou `mounts`), enregistrée dans `fileusers.json` du répertoire de configuration. Ils se gèrent sur la page « Utilisateurs partagés de fichiers » du groupe « Services de fichiers » de la barre latérale, et l’enregistrement s’applique aux trois services ; les utilisateurs locaux de chaque service ne changent pas. Le haut de la page indique si chaque service autorise les utilisateurs partagés et liste ceux qui portent le même nom qu’un utilisateur local ou que SMB ne peut pas utiliser.

- Chaque service décide seul d’accepter ou non les utilisateurs partagés : activez « Autoriser les utilisateurs partagés » dans la carte « Utilisateurs partagés » de la page du service (champ `sharedUsers`) et enregistrez, et les utilisateurs partagés peuvent se connecter à ce service ; la carte liste aussi les utilisateurs partagés enregistrés, et « Gérer les utilisateurs partagés » ouvre la page « Utilisateurs partagés de fichiers » ;
- Si un utilisateur local porte le même nom, seul l'utilisateur local est vérifié (sans distinction de casse pour SMB) ;
- Les noms d'utilisateurs partagés doivent être uniques sans distinction de casse et ne peuvent pas contenir de deux-points ;
- Les modifications s'appliquent immédiatement : WebDAV utilise les nouveaux paramètres à la requête suivante, FTP et SMB redémarrent et les clients connectés doivent se reconnecter ;
- Interrupteur activé et avec au moins un utilisateur partagé, le service peut être activé sans utilisateur local ;
- Un service activé uniquement avec des utilisateurs partagés doit garder après une modification un utilisateur partagé capable de se connecter (pour SMB, un utilisateur utilisable par SMB), sinon la modification est refusée ;
- Si `fileusers.json` est cassé à la main (mot de passe vide, répertoire qui n'est pas un chemin absolu…), aucun utilisateur partagé ne peut se connecter et la page l'indique ; corrigez puis enregistrez pour les rétablir ;
- SMB n’accepte que des mots de passe en clair, et les noms d’utilisateur et de partage doivent respecter les règles de caractères de SMB ; les utilisateurs partagés qui ne les respectent pas sont signalés sur la page « Utilisateurs partagés de fichiers », sur la page SMB et dans le journal d’exécution, puis ignorés, tandis que WebDAV et FTP peuvent toujours les utiliser.

L’API est `GET` / `PUT /api/fileusers`, qui demande la permission `fileusers:read` / `fileusers:write` pour les jetons d’API ; `services` dans la réponse indique comment chaque service les utilise. Les anciens `GET` / `PUT /api/<service>/shared-users` de chaque service sont obsolètes mais fonctionnent toujours et lisent et écrivent la même liste : la lecture utilise la permission de lecture du service, et une modification exige à la fois `webdav:write`, `ftp:write` et `smb:write`.

**Changement de mot de passe en libre-service** : activez Changement de mot de passe en libre-service sur la page Utilisateurs partagés de fichiers et enregistrez ; les utilisateurs partagés peuvent alors changer leur mot de passe sur `https://domaine-du-site/__gately_auth/password` de n'importe quel site du service web avec authentification web, en saisissant le nom d'utilisateur, le mot de passe actuel et le nouveau (8 à 128 caractères). WebDAV, FTP et SMB utilisent aussitôt le nouveau mot de passe (FTP et SMB redémarrent, les clients connectés doivent se reconnecter). Les mots de passe en clair restent en clair et les hachages bcrypt restent hachés ; un mot de passe actuel erroné compte dans la même limitation et le même bannissement automatique que la connexion web. L'interrupteur est stocké dans `selfService` de `fileusers.json`, avec le même nom de champ dans l'API. Pour les utilisateurs partagés de connexion web, voir [le changement de mot de passe du service web](/fr/docs/modules/web#self-service-password).

## File Browser {#filebrowser}

Se configure dans « File Browser », dans le groupe « Services de fichiers ». Gately héberge [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser), un gestionnaire de fichiers web qui prévisualise images, vidéos et documents, envoie et télécharge des fichiers, crée des liens de partage et dispose de ses propres utilisateurs et permissions. Le filebrowser/filebrowser d’origine a été archivé en 2026 et ne reçoit plus de correctifs de sécurité ; Gately héberge donc le fork Quantum, toujours maintenu.

Le programme n’est pas fourni avec Gately. La carte « Installation en ligne » en bas de la page télécharge FileBrowser Quantum depuis GitHub dans `bin/filebrowser` sous le répertoire de configuration et permet de mettre à jour, d’installer une version précise, d’utiliser un proxy de téléchargement propre, de revenir en arrière et de désinstaller ; après une mise à jour ou un retour arrière, les instances activées redémarrent. Voir [Installation, mise à jour et retour arrière en ligne](/fr/docs/modules/tunnels#install). Le bouton « Rétablir les valeurs par défaut » de l’en-tête supprime toutes les instances et leurs données ; voir [Rétablir les valeurs par défaut](/fr/docs/modules/tunnels#reset). Vous pouvez aussi placer le programme vous-même et indiquer son chemin. Le programme est recherché dans cet ordre : le chemin indiqué, `bin/` sous le répertoire de configuration, puis `PATH` ; la page avertit si le programme trouvé n’est pas Quantum (par exemple l’original dans `PATH`).

Chaque instance est un processus File Browser distinct :

| Champ | Par défaut | Description |
| --- | --- | --- |
| `listenIp` | vide | Adresse d’écoute ; vide écoute sur toutes les adresses IPv4. Utilisez `127.0.0.1` si l’accès passe uniquement par un proxy inverse du Service Web |
| `port` | 8080 | Port d’écoute |
| `baseUrl` | `/` | Chemin de base ; en proxy inverse par chemin dans le Service Web (par exemple `/files`), saisissez le même chemin |
| `accessUrls` | Vide | URL d’accès, plusieurs possibles, par exemple `https://files.example.com/` en proxy inverse via le service Web ; affichées sur la carte de l’instance. Si vide, l’URL est déduite de l’adresse et du port d’écoute. Le schéma, l’hôte et le port de la première URL sont écrits dans `http.externalUrl` de File Browser, utilisé par les liens de partage |
| `sources` | aucun | Répertoires partagés, chacun avec `path` (absolu), `name` (nom affiché, par défaut celui du répertoire) et `readOnly` ; les nouveaux utilisateurs ont accès à tous par défaut |
| `authMethod` | `password` | Connexion : `password` par mot de passe, `proxy` fait confiance à l’en-tête de nom d’utilisateur du proxy inverse, `noauth` sans connexion |
| `proxyHeader` | `Remote-User` | En-tête portant le nom d’utilisateur pour `proxy` ; les utilisateurs absents sont créés automatiquement |
| `adminUser` | `admin` | Nom de l’administrateur |
| `adminPassword` | aucun | Mot de passe administrateur, au moins 8 caractères, obligatoire avec `password` |
| `trustProxyHeaders` | désactivé | Fait confiance à `X-Forwarded-*` et `X-Real-IP` ; à activer derrière un proxy inverse |
| `disableWebDav` | désactivé | Désactive l’interface WebDAV intégrée à File Browser |
| `locale` | vide | Langue d’interface des nouveaux utilisateurs, par exemple `fr` ; vide correspond à l’anglais |
| `tokenHours` | 2 | Durée de session en heures |

- Le mot de passe administrateur est synchronisé avec File Browser à chaque démarrage, la valeur de Gately fait donc foi ; les autres utilisateurs, liens de partage et préférences se gèrent dans File Browser et sont stockés dans la base de données de l’instance.
- Le fichier de configuration, la base de données et le cache se trouvent dans `filebrowser/<clé de l’instance>/` sous le répertoire de configuration, le fichier de configuration en 0600 ; le mot de passe administrateur est transmis au processus par une variable d’environnement et n’est jamais écrit dans un fichier. Supprimer une instance supprime ce répertoire, et les fichiers des répertoires partagés restent intacts.
- Une instance dont un répertoire partagé n’existe pas ne démarre pas, et sa carte en indique la raison.
- Pour un accès depuis l’extérieur, ajoutez dans le [Service Web](/fr/docs/modules/web) un site de proxy inverse vers `http://127.0.0.1:port`, protégez-le par HTTPS et contrôle d’accès, et activez « Faire confiance aux en-têtes du proxy ». La méthode `proxy` nécessite l’authentification déléguée du Service Web pour placer le nom d’utilisateur dans l’en-tête, et le port de File Browser doit rester fermé vers l’extérieur, sinon l’en-tête peut être falsifié pour se connecter.
- Les fichiers sont lus et écrits avec l’identité du processus Gately.

## Montages de stockage

Ajoutez des montages dans « Montages de stockage » du groupe « Services de fichiers », et gérez les fichiers en ligne depuis la page.

| Champ | Description |
| --- | --- |
| `name`, `remark`, `enable` | Nom, remarque et interrupteur d'activation |
| `type` | `local` (répertoire local), `webdav` (WebDAV distant) ou `smb` (partage SMB) |
| `path` | Répertoire local, doit être un chemin absolu existant (utilisé par `local`) |
| `url`, `username`, `password` | Adresse distante et compte ; le mot de passe est masqué dans l'interface (utilisés par `webdav` et `smb`). Une adresse WebDAV commence par `http://` ou `https://` ; une adresse SMB a la forme `smb://hôte[:port]/partage[/sous-dossier]`, SMB2/3 est pris en charge et un utilisateur vide se connecte en invité |
| `skipVerify` | Ignore la vérification du certificat HTTPS (utilisé par `webdav`) |
| `domain` | Domaine ou groupe de travail pour l'authentification SMB, généralement vide (utilisé par `smb`) |
| `readOnly` | Monte en lecture seule ; le gestionnaire de fichiers rejette toute opération d'écriture |

Le gestionnaire de fichiers en ligne permet de parcourir les répertoires, télécharger des fichiers, téléverser en flux (le fichier entier n'est pas chargé en mémoire), créer des répertoires, renommer et supprimer. Un montage peut servir d'emplacement cible pour une tâche de sauvegarde : les fichiers de sauvegarde sont écrits dans le répertoire indiqué du montage, et les anciens fichiers sont supprimés selon la politique de rétention.

## Wake-on-LAN

Gérez les appareils dans « Wake-on-LAN » et envoyez-leur des paquets magiques (6 octets `0xFF` suivis de l'adresse MAC répétée 16 fois).

| Champ | Par défaut | Description |
| --- | --- | --- |
| `name` | Aucun | Nom de l'appareil, obligatoire |
| `macs` | Aucun | Adresses MAC, une par ligne, au moins une ; vous pouvez en saisir plusieurs pour un même appareil |
| `broadcastIps` | Vide | Adresses de diffusion IPv4. Si vide, les paquets sont envoyés à `255.255.255.255` et à l'adresse de diffusion de chaque interface réseau activée |
| `port` | 9 | Port UDP cible |
| `repeat` | 3 | Nombre d'envois pour chaque MAC, jusqu'à 20 |
| `checkAddr` | Vide | Adresse de vérification de présence au format `host:port`, par exemple `192.168.1.10:3389` ; laissez vide pour ignorer la vérification |
| `shutdownConn` | Vide | Connexion SSH du [terminal web](/fr/docs/modules/terminal) utilisée pour l’arrêt à distance ; vide pour désactiver l’arrêt |
| `shutdownCommand` | Aucune | Commande d’arrêt, obligatoire si une connexion est choisie, sur une seule ligne, comme `shutdown -h now` ou `shutdown /s /t 0` |
| `bemfaTopic` | Vide | Sujet Bemfa ; vide pour ne pas utiliser la commande vocale ; unique par appareil |
| `remark` | Vide | Remarque |

Cliquez sur Réveiller dans la liste des appareils pour envoyer le paquet magique ; la page indique si l'appareil est en ligne d'après `checkAddr`.

### Arrêt à distance

Choisissez une connexion SSH du terminal web et saisissez une commande d’arrêt : la carte de l’appareil affiche alors un bouton *Éteindre* ; l’API est `POST /api/wol/devices/{key}/shutdown`.

- La connexion doit avoir été ouverte une fois dans le terminal web pour enregistrer la clé d’hôte ; Gately ne fait jamais confiance à un hôte inconnu sans supervision ;
- Chaque arrêt ouvre une nouvelle connexion SSH et la ferme ensuite : une connexion périmée d’avant le redémarrage de l’appareil n’est jamais réutilisée ; la commande dure au plus 30 secondes, une seule à la fois par appareil ;
- L’arrêt coupe souvent la connexion SSH avant le retour du code de sortie ; on considère alors que l’arrêt a commencé ;
- Le formulaire propose les commandes courantes pour Linux (`shutdown -h now`), Windows (`shutdown /s /t 0`) et macOS (`sudo -n shutdown -h now`). Windows doit avoir le serveur OpenSSH activé ; sous Linux, un utilisateur non root peut utiliser `sudo -n shutdown -h now` avec sudo sans mot de passe pour cette commande.
- Les jetons d’API ont besoin de `webterminal:write` en plus de `wol:write` pour créer, modifier ou copier des appareils ou déclencher un arrêt (ces opérations définissent ou exécutent des commandes via la connexion SSH) ; lister les connexions SSH disponibles nécessite `wol:read` et `webterminal:read`.

### Commande vocale Bemfa {#bemfa}

Une fois [Bemfa](https://cloud.bemfa.com) connecté, des assistants vocaux comme Xiao AI, Xiaodu ou Google Assistant peuvent allumer et éteindre les appareils :

1. Dans la console Bemfa, créez un sujet dans le cloud d’appareils MQTT dont le nom se termine par `001` (prise) ou `006` (interrupteur), par exemple `pc001` ;
2. Saisissez le même sujet sur l’appareil puis, dans *Wake-on-LAN → Bemfa*, la clé privée utilisateur de la console, et activez ;
3. Associez votre compte Bemfa dans Mi Home (Xiao AI), Xiaodu ou Google Home et synchronisez les appareils.

Ensuite, « allumer » réveille l’appareil et « éteindre » l’arrête (arrêt à distance configuré requis) ; après une action réussie, l’état est réécrit dans Bemfa. Gately se connecte à `bemfa.com:9503` en MQTT 3.1.1 sur TLS et vérifie le certificat ; la clé privée sert d’ID client, n’est utilisée que sur cette connexion chiffrée et reste masquée dans l’API. Après une coupure, il réessaie à partir de 5 secondes jusqu’à toutes les 5 minutes ; l’état précédent renvoyé par le serveur à l’abonnement (messages retenus et tout message reçu dans les 3 secondes suivant l’abonnement) n’est pas exécuté, et la même commande répétée en moins de 10 secondes n’est exécutée qu’une fois ; un réveil ou un arrêt réussi depuis l’interface ou l’API réécrit aussi l’état. Un seul client peut être en ligne par clé privée : si un autre programme ou un autre Gately se connecte avec la même clé, les deux se déconnectent mutuellement.

## Tâches planifiées

Exécutez des commandes ou envoyez des requêtes HTTP selon un calendrier cron.

- `spec` accepte les expressions standard à 5 champs, un champ de secondes facultatif (6 champs, par exemple `*/30 * * * * *`) et des descripteurs comme `@every 10m` et `@daily` ;
- Lorsque `type` vaut `command`, `command` s'exécute via `sh -c` (un `.bat` temporaire sous Windows), et l'environnement reçoit `taskName` ainsi qu'une variable en majuscules de même nom avec le préfixe `GATELY_` ;
- Lorsque `type` vaut `http`, une requête est envoyée selon la configuration `http`, avec les champs `url`, `method` (GET, POST, PUT, PATCH, DELETE), `headers` (chacun sous la forme `Key: Value`), `body`, `successContent` (le corps de la réponse doit contenir l'une de ses entrées pour compter comme un succès ; si vide, seul le statut 2xx est vérifié), `timeout`, `insecureSkipVerify` et `proxy` (vide : aucun proxy ; `update` : proxy global de mise à jour ; ou une adresse de proxy `http://`, `https://` ou `socks5://`) ;
- Les tâches peuvent être activées ou désactivées, la page affiche l'heure de la prochaine exécution, et une tâche peut être lancée immédiatement à la main ;
- Si l'exécution précédente de la même tâche n'est pas terminée, le nouveau déclenchement est ignoré ;
- Chaque tâche conserve ses 20 derniers enregistrements d'exécution, incluant l'heure de début, la durée, le succès ou l'échec, si elle a été déclenchée manuellement, et la sortie (un résumé de 4000 caractères maximum) ;
- Le formulaire de création valide et analyse l'expression.

## Vue d'ensemble et journaux système

- La page « Vue d'ensemble » affiche les informations de l'hôte : modèle de CPU, utilisation et charge, mémoire, disque, adresses des interfaces réseau et débits en direct, IPv4 et IPv6 publiques (cliquez pour copier), l'état du service de chaque module et, pour le processus Gately, sa version, son commit, sa date de build, son heure de démarrage, son PID, son CPU, sa mémoire résidente, son tas Go et son nombre de goroutines ;
- Lorsque Gately gère des programmes externes (frp, Tailscale, NetBird, tunnel Cloudflare, File Browser), la page « Vue d'ensemble » comporte aussi une carte « Processus gérés » qui liste chaque processus avec son module et son instance, le nom du programme, le PID, la durée d'activité, le nombre de redémarrages, le CPU et la mémoire résidente, les totaux figurant dans l'en-tête ; cliquez sur une ligne pour ouvrir son module. Seul le processus lui-même est compté, pas les enfants qu'il lance, et les chemins et arguments de lancement ne sont pas affichés ;
- La page « Journaux système » rassemble les journaux de tous les modules, y compris les enregistrements d'exécution de modules tels que Docker, FTP et Tâches planifiées.

## Thèmes

L'interface propose 15 palettes en deux groupes, les originales Gately (Émeraude, Océan, Fraîcheur, Sakura, Coucher de soleil, Minuit, Punk) et les classiques (GitHub, Nord, Dracula, Catppuccin, Tokyo Night, Gruvbox, Rosé Pine, Solarized), chacune avec une variante claire et une variante sombre. Le mode peut être clair, sombre ou suivre le système, qui utilise la variante claire ou sombre de la palette en cours. Toute palette accepte l'une des 10 couleurs d'accent prédéfinies ou une couleur choisie avec le sélecteur du système ; le logo et le dégradé de la page de connexion la suivent, et choisir à nouveau une palette efface l'accent personnalisé. Les coins peuvent être droits, ceux de la palette ou arrondis. Les réglages du thème sont stockés localement dans le navigateur.
