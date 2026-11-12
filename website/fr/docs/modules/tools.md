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

**Limitation des connexions** : WebDAV, FTP et SMB limitent les mots de passe erronés par adresse source. Après 10 échecs en 15 minutes, toute connexion de cette source est refusée jusqu'à la fin de la fenêtre (WebDAV renvoie `429`, FTP et SMB renvoient un échec de connexion, et SMB écrit aussi « trop d'échecs de connexion, refusé » dans le journal), même avec le bon mot de passe. Une connexion réussie ne remet pas le compteur à zéro et les sources IPv6 sont comptées par /64. Lorsque WebDAV se trouve derrière un reverse proxy local comme le service web de Gately, l'adresse du visiteur transmise par le proxy est utilisée.

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
| `shutdownMode` | Vide | Méthode d’arrêt à distance : `agent` (agent sur l’appareil), `ssh` (commande SSH) ou `http` (requête HTTP) ; vide pour désactiver l’arrêt |
| `shutdownConn` | Vide | Connexion SSH du [terminal web](/fr/docs/modules/terminal) utilisée par la méthode `ssh` |
| `shutdownCommand` | Aucune | Commande d’arrêt de la méthode `ssh`, sur une seule ligne, comme `shutdown -h now` ou `shutdown /s /t 0` |
| `agentToken` | Généré automatiquement | Jeton de l’agent de la méthode `agent`, masqué dans l’API |
| `shutdownHttp` | Vide | Requête de la méthode `http` : `method`, `url`, `headers`, `body`, `insecure` |
| `bemfaTopic` | Vide | Sujet Bemfa ; vide pour ne pas utiliser la commande vocale ; unique par appareil |
| `remark` | Vide | Remarque |

Cliquez sur Réveiller dans la liste des appareils pour envoyer le paquet magique ; la page indique si l'appareil est en ligne d'après `checkAddr`.

### Arrêt à distance

Choisissez une méthode dans *Extinction à distance* de l’appareil : la carte de l’appareil affiche alors un bouton d’arrêt ; l’API est `POST /api/wol/devices/{key}/shutdown`. Le corps de la requête peut être omis, ou utiliser `{"action": "reboot"}` ou `{"action": "sleep"}` pour redémarrer ou mettre en veille (méthode de l’agent uniquement). Une seule action à la fois par appareil, 30 secondes au plus ; après un arrêt ou une mise en veille réussis, l’état de l’interrupteur Bemfa est écrit sur éteint.

| Méthode | Cas d’usage | Actions prises en charge |
| --- | --- | --- |
| Agent | Windows, Linux, macOS et OpenWrt ; l’appareil n’a pas besoin d’ouvrir de port ni d’activer SSH | Arrêter, redémarrer, mettre en veille |
| Commande SSH | Hôtes dont le service SSH est activé et déjà enregistrés dans le terminal web | Arrêter |
| Requête HTTP | Services offrant une interface HTTP, comme Home Assistant, les prises connectées ou un outil d’arrêt déjà présent sur l’appareil | Arrêter |

#### Agent {#agent}

Un script d’agent s’exécute sur l’appareil contrôlé. Il se connecte lui-même à Gately, interroge les instructions par long polling et, à la réception d’une instruction, exécute localement l’arrêt, le redémarrage ou la mise en veille. L’agent n’accepte que ces trois actions fixes : Gately ne peut pas lui faire exécuter d’autre commande ; l’appareil n’a besoin d’ouvrir aucun port, et Gately ne conserve pas les identifiants de connexion de l’appareil.

1. Réglez la méthode d’arrêt de l’appareil sur *Agent* et enregistrez ; Gately génère un jeton pour l’appareil et ouvre les instructions d’installation (vous pouvez les rouvrir ensuite depuis *Agent* sur la carte de l’appareil) ;
2. Vérifiez que *Adresse de Gately* est une adresse d’administration accessible depuis l’appareil (par défaut l’adresse actuelle du navigateur ; en passant par la passerelle unifiée fnOS, remplacez-la par l’adresse directe), et activez *Ne pas vérifier le certificat* si Gately utilise un certificat auto-signé ;
3. Sélectionnez le système de l’appareil, copiez la commande d’installation et exécutez-la sur l’appareil :

| Système | Où l’exécuter | Installé comme | Journal |
| --- | --- | --- | --- |
| Windows | PowerShell exécuté en administrateur | Tâche planifiée au démarrage « Gately Agent » du compte SYSTEM | `C:\ProgramData\GatelyAgent\agent.log` |
| Linux | Terminal (nécessite sudo, ainsi que curl ou wget) | Service systemd ou OpenRC `gately-agent` | `journalctl -u gately-agent` ou `/var/log/gately-agent.log` |
| macOS | Terminal (nécessite le mot de passe administrateur) | Démon launchd `cn.gately.agent` | `/var/log/gately-agent.log` |
| OpenWrt | SSH (root), avec curl ou le wget du système | Service procd `gately-agent`, configuration conservée lors des mises à niveau du système | `logread -e gately-agent` |

La commande d’installation télécharge le script depuis Gately (il peut aussi être téléchargé directement depuis les instructions d’installation) et vérifie d’abord son SHA-256 (`sha256sum` sous Linux et OpenWrt, `shasum -a 256` sous macOS, `Get-FileHash` sous Windows) ; s’il ne correspond pas à la somme fournie par Gately, elle affiche `gately-agent: checksum mismatch` et s’arrête, sinon elle installe ce script vérifié lui-même. Le jeton est transmis dans le corps de la requête et par une variable d’environnement, jamais dans l’URL ni dans les arguments de ligne de commande d’un processus : curl et wget le lisent dans un fichier temporaire créé par `mktemp`, et sudo transmet les variables d’environnement avec `--preserve-env`. Même téléchargé en HTTP non chiffré ou avec *Ne pas vérifier le certificat*, un script modifié n’est donc jamais exécuté ; le jeton, lui, circule en clair, d’où la recommandation d’une adresse HTTPS. Une fois l’agent connecté, la carte de l’appareil le montre en ligne, avec le nom d’hôte, le système et l’adresse source que l’agent signale ; un appareil utilisant la méthode de l’agent n’a pas besoin d’adresse de vérification de présence.

- Le jeton équivaut à l’interrupteur d’alimentation de l’appareil. Le fichier de configuration n’est lisible que par root (sous Windows, SYSTEM et les administrateurs). Un même jeton ne sert que pour un seul appareil : si une autre machine se connecte avec le même jeton, elle est refusée et un message apparaît dans le journal ;
- Une instruction n’est valable que pendant les 10 secondes suivant son émission : l’agent confirme d’abord, puis l’exécute une fois que Gately a répondu. Lorsque l’agent est hors ligne, une erreur est renvoyée aussitôt et l’instruction n’est pas mise en file d’attente jusqu’à sa prochaine connexion ;
- Après avoir *Régénéré* le jeton ou choisi une autre méthode d’arrêt, l’ancien agent est déconnecté immédiatement et doit être réinstallé avec la nouvelle commande ;
- L’agent interroge toutes les 25 secondes et, après une déconnexion, réessaie à des intervalles de 5 secondes jusqu’à 60 secondes ;
- Emplacement d’installation : `/usr/local/bin/gately-agent` sous Linux, `/Library/Gately/gately-agent` sous macOS (Homebrew confie souvent `/usr/local/bin` à un utilisateur ordinaire) et `/usr/bin/gately-agent` sous OpenWrt ; le répertoire d’installation doit appartenir à root et ne pas être modifiable par les autres utilisateurs, sinon l’installation est refusée. Sous Windows, c’est `C:\ProgramData\GatelyAgent`, dont les permissions sont entièrement remplacées pour que seuls SYSTEM et les administrateurs y aient accès ; si ce répertoire existe déjà mais est un lien (jonction ou lien symbolique) ou n’appartient pas à SYSTEM ou aux administrateurs, l’installation est refusée et invite à vérifier puis supprimer le répertoire ;
- Désinstallation : copiez la commande de désinstallation depuis les instructions d’installation. Sous Windows, c’est `powershell -ExecutionPolicy Bypass -File "$env:ProgramData\GatelyAgent\gately-agent.ps1" uninstall`, sous Linux `sudo sh /usr/local/bin/gately-agent uninstall`, sous macOS `sudo sh /Library/Gately/gately-agent uninstall`, et sous OpenWrt `sh /usr/bin/gately-agent uninstall` ;
- Veille : Windows suspend en mémoire (une machine dont la veille est désactivée échoue et le journal l’indique), Linux utilise `systemctl suspend`, macOS utilise `pmset sleepnow` ; pour réveiller par Wake-on-LAN un appareil en veille, autorisez le Wake-on-LAN dans la carte réseau ou le système.

Les interfaces de l’agent se trouvent sous `/api/wol/public/agent/`. Elles ne demandent pas de connexion et n’acceptent que le jeton de l’appareil ; lorsqu’un chemin d’accès secret est défini, les requêtes portant un jeton valide peuvent aussi y accéder, et « Accès réseau local uniquement » ainsi que la liste de blocage s’appliquent toujours. L’interface d’administration `GET /api/wol/agent-scripts` renvoie le SHA-256 de chaque script de l’agent, indexé par nom de fichier.

#### Commande SSH

Choisissez une connexion SSH du terminal web et saisissez une commande d’arrêt :

- La connexion doit avoir été ouverte une fois dans le terminal web pour enregistrer la clé d’hôte ; Gately ne fait jamais confiance à un hôte inconnu sans supervision ;
- Chaque arrêt ouvre une nouvelle connexion SSH et la ferme ensuite : une connexion périmée d’avant le redémarrage de l’appareil n’est jamais réutilisée ;
- L’arrêt coupe souvent la connexion SSH avant le retour du code de sortie ; on considère alors que l’arrêt a commencé ;
- Le formulaire propose les commandes courantes pour Linux (`shutdown -h now`), Windows (`shutdown /s /t 0`) et macOS (`sudo -n shutdown -h now`). Windows doit avoir le serveur OpenSSH activé ; sous Linux, un utilisateur non root peut utiliser `sudo -n shutdown -h now` avec sudo sans mot de passe pour cette commande.

#### Requête HTTP

Saisissez l’URL de la requête (`http://` ou `https://`), la méthode, les en-têtes et le corps ; un code d’état 2xx dans la réponse compte comme un succès, et le contenu de la réponse s’affiche dans le résultat. Avec la méthode réglée sur *Auto*, une requête avec corps utilise `POST`, sinon `GET` ; un corps commençant par `{` ou `[` sans `Content-Type` précisé est envoyé en JSON. Placez les identifiants (comme `Authorization: Bearer …`) dans les en-têtes ; les comptes en lecture seule voient les valeurs masquées, et l’adresse figurant dans les journaux ne contient pas les paramètres de requête. Si la cible utilise un certificat auto-signé, activez *Ne pas vérifier le certificat*.

Les jetons d’API ont besoin de `webterminal:write` en plus de `wol:write` pour créer, modifier ou copier des appareils ou déclencher un arrêt (la méthode SSH peut définir ou exécuter des commandes via une connexion SSH) ; lister les connexions SSH disponibles nécessite `wol:read` et `webterminal:read` ; consulter le jeton de l’agent (`GET /api/wol/devices/{key}/secrets`) et le régénérer (`POST /api/wol/devices/{key}/agent-token`) nécessitent `wol:write`.

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
