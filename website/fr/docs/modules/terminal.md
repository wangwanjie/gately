# Terminal Web

Connectez-vous à des serveurs SSH et Telnet ou au shell local depuis votre navigateur. Les sessions sont conservées sur le serveur et peuvent être rattachées à nouveau après une déconnexion. Le point d'entrée est « Terminal Web » dans la barre latérale.

## Types de connexion

| Type | Description |
| --- | --- |
| SSH | Prend en charge l'authentification par mot de passe et par clé privée (collez une clé privée ou indiquez un chemin `keyFile` sur le serveur ; les phrases secrètes de clé sont prises en charge). Vous pouvez définir le `workDir` à rejoindre après la connexion, `keepAlive`, le délai de connexion (1 à 120 secondes, 15 par défaut) et un proxy |
| Telnet | Port 23 par défaut. Lorsque des invites telles que `login:`, `username:` et `password:` sont détectées, le champ correspondant est rempli automatiquement, une fois chacun. Un proxy peut être défini |
| Shell local | Ouvre directement un shell sur l'appareil qui exécute Gately. Vous pouvez préciser le chemin du shell, le répertoire de travail et des variables d'environnement supplémentaires |

Chaque connexion peut avoir une remarque. La liste des connexions permet le tri par glisser-déposer et le « Test de connexion » : pour SSH, il renvoie la version du serveur et la latence ; pour Telnet, il vérifie uniquement l'accessibilité TCP (y compris via le proxy) ; les connexions de shell local ne peuvent pas être testées. Lorsque vous modifiez une connexion, le mot de passe, la clé privée, la phrase secrète de la clé et le mot de passe du proxy sont affichés masqués, et les valeurs d'origine sont conservées si vous ne les changez pas.

Dans « Paramètres », vous pouvez gérer des « préréglages d’identifiants SSH » (mot de passe ou clé privée, avec un nom d’utilisateur par défaut facultatif). Choisissez un préréglage lors de l’ajout ou de la modification d’une connexion SSH pour le réutiliser : plusieurs connexions peuvent partager le même identifiant, et les modifications du préréglage s’appliquent à la prochaine connexion. Un préréglage encore référencé par une connexion ne peut pas être supprimé.

### Proxy

Les connexions SSH et Telnet peuvent chacune utiliser leur propre proxy sortant de type `socks5` ou `http`, avec un nom d'utilisateur et un mot de passe de proxy facultatifs.

### Confiance à la première utilisation pour les clés d'hôte

Lors de la première connexion SSH, l'empreinte de l'hôte du serveur est enregistrée et considérée comme fiable, puis vérifiée à chaque connexion ultérieure. Après la réinstallation d'un serveur ou un changement de ses clés, exécutez « Réinitialiser l'empreinte de l'hôte » sur la connexion pour lui refaire confiance.

## Sessions

- Les sessions sont conservées sur le serveur. Après un rafraîchissement du navigateur ou une déconnexion, vous pouvez vous rattacher depuis la liste des sessions ; au rattachement, le tampon de sortie est d'abord rejoué, puis la sortie est diffusée en direct ;
- Plusieurs clients peuvent se rattacher simultanément à la même session, avec une saisie partagée et une sortie diffusée à tous ;
- Les sessions peuvent avoir une remarque. La liste des sessions affiche l'heure de création, l'heure de la dernière activité, la taille de la fenêtre et les octets envoyés et reçus ;
- Après un rafraîchissement de la page, les sessions toujours actives sont restaurées sous forme d'onglets, ainsi que la disposition de la vue scindée ;
- Le terminal intègre la recherche (saisissez un mot-clé, Entrée pour l'occurrence suivante et Maj+Entrée pour la précédente), la copie, le collage, l'effacement de l'écran, le réglage de la taille de police, le plein écran et plusieurs thèmes de couleurs (qui suivent par défaut le thème de l'interface, ou un thème fixe).

### Paramètres

Ajustez-les dans « Paramètres » en haut à droite de la page :

| Champ | Par défaut | Description |
| --- | --- | --- |
| `allowLocalShell` | Désactivé | Autorise ou non l'ouverture du shell local |
| `maxSessions` | 20 | Nombre maximal de sessions simultanées, jusqu'à 1000. Créer une session au-delà de la limite renvoie HTTP 429 |
| `idleTimeout` | 0 | Une session se termine automatiquement après ce nombre de minutes sans entrée ni sortie ; 0 signifie aucune limite |
| `bufferSize` | 256 | Tampon de rejeu conservé par session (Ko), jusqu'à 16384 |
| `heartbeatInterval` | 30 | Intervalle de heartbeat WebSocket (secondes), jusqu'à 600 |
| `sessionKeepAlive` | 0 | Minutes pendant lesquelles une session est conservée après la déconnexion du navigateur ; 0 la termine immédiatement à la déconnexion, jusqu'à 10080 |

::: tip Astuce
Pour pouvoir vous reconnecter après une déconnexion, définissez `sessionKeepAlive` à une valeur supérieure à 0. Avec 0, la session se termine dès que le navigateur se déconnecte.
:::

## Gestionnaire de fichiers SFTP

L'icône de dossier de la barre d'outils du terminal ouvre le panneau du gestionnaire de fichiers. Les connexions SSH opèrent sur les fichiers distants via SFTP, et les connexions de shell local opèrent directement sur le système de fichiers de l'appareil qui exécute Gately. Les connexions Telnet n'ont pas de gestionnaire de fichiers. Le gestionnaire de fichiers local exige que « Autoriser le shell local » soit activé.

| Fonction | Description |
| --- | --- |
| Parcourir | Fil d'Ariane et navigation vers le dossier parent, dossiers en premier et triés par nom, avec ordre de tri modifiable ; affiche les permissions, la taille, la date de modification et le propriétaire |
| Créer | Nouveau dossier, nouveau fichier vide |
| Téléverser | Téléversement de plusieurs fichiers ; sur ordinateur, des dossiers entiers peuvent être téléversés (l'arborescence est conservée). Les fichiers du même nom sont écrasés, et le panneau affiche la progression |
| Télécharger | Téléchargement en flux avec reprise (`Range`) |
| Édition en ligne | Aperçu et enregistrement des fichiers texte ; les fichiers de plus de 1 Mo ou binaires ne peuvent pas être prévisualisés |
| Renommer, copier, coller | Lors d'une copie dans le même dossier, le fichier est renommé automatiquement en `name copy.ext`, `name copy 2.ext` |
| Supprimer | Suppression par lot avec sélection multiple ; les dossiers sont supprimés récursivement ; le répertoire racine est protégé |
| Compresser et extraire | Compression en `zip` ou `tar.gz` ; extraction de `zip`, `tar`, `tar.gz` et `tgz` ; le contenu de l'archive (jusqu'à 10000 entrées) peut être prévisualisé avant l'extraction. La compression et l'extraction sont réalisées par Gately lui-même, aucune commande n'a donc besoin d'être installée sur l'hôte distant |
| Modifier les permissions | Permissions octales (0 à 7777), avec une option récursive pour les dossiers |
| Accès rapide | Enregistrez les dossiers fréquemment utilisés pour chaque connexion et accédez-y en un clic |

Lors de l'extraction, une archive contenant une entrée avec `..` est rejetée dans son ensemble pour éviter la traversée de répertoires, les entrées de liens symboliques et de liens physiques sont ignorées, et seuls les bits rwx des permissions de fichier sont conservés. Sur mobile, le panneau propose un mode de sélection multiple et une feuille d'actions.

## Raccourcis

La liste déroulante « Raccourcis » affiche les raccourcis globaux ; un clic en envoie un au terminal courant. Chaque raccourci possède un `name`, une `command` et `sendEnter` (appuyer ou non automatiquement sur Entrée après l'envoi). Ajoutez, modifiez et supprimez des raccourcis via « Gérer les raccourcis » en bas de la liste déroulante.

## Vue scindée

Le menu « Scinder » de la barre d'outils propose les dispositions côte à côte, empilée et en grille, avec jusqu'à 4 panneaux à la fois. Une fois la vue scindée activée, toute connexion que vous ouvrez rejoint automatiquement la vue, et vous pouvez aussi cliquer sur un autre onglet pour l'ajouter. La barre de titre d'un panneau permet de retirer une session de la vue scindée. La disposition est stockée sur le serveur et restaurée après un rafraîchissement de la page. Sur les écrans étroits, la vue scindée est désactivée automatiquement, et la disposition enregistrée n'est pas supprimée.

## Prise en charge du mobile

- Sur les écrans étroits, la liste des connexions et le terminal sont des vues distinctes, et le bouton retour en haut à gauche de la vue du terminal ramène à la liste ;
- La zone du terminal suit le viewport visuel et se réduit lorsque le clavier virtuel apparaît ;
- La barre de touches virtuelles tient sur une seule ligne que l'on fait défiler latéralement. Elle fournit `Esc`, `Tab`, `Ctrl`, `Alt` (persistantes), les flèches (maintenir pour répéter), les symboles courants et un accès aux raccourcis, avec un bouton clavier fixé à droite qui affiche ou masque le clavier. Par défaut, la barre apparaît et disparaît avec le clavier virtuel ; clavier fermé, un bouton flottant en bas à droite le rouvre. Pour utiliser les flèches, la pagination et d'autres touches sans clavier, activez **Toujours afficher la barre de touches** dans le menu **Plus**.

## Sécurité

Un terminal équivaut à exécuter des commandes arbitraires sur l'hôte cible ; Gately applique donc ces restrictions par défaut :

- **Le shell local est désactivé par défaut** et doit être activé explicitement dans les paramètres ;
- **Confirmation du contrat** : la première fois que vous activez le shell local, créez une session, vous rattachez à une session ou utilisez le gestionnaire de fichiers, un contrat d'utilisation s'affiche et vous devez le cocher pour continuer ;
- **Authentification à deux facteurs (2FA)** : après avoir lié une application d'authentification TOTP en scannant le QR code dans les paramètres de sécurité, les actions ci-dessus exigent un code à 6 chiffres. Une fois vérifié, le jeton de connexion courant n'a pas besoin d'être vérifié à nouveau pendant 30 minutes ; après 5 codes erronés consécutifs, ce jeton de connexion est verrouillé pendant 1 minute ;
- Les mots de passe de connexion, clés privées et mots de passe de proxy sont masqués dans les sorties de l'API.

::: warning Attention
Avant d'activer le shell local, activez l'authentification à deux facteurs pour l'interface d'administration, et associez-la à un « accès limité au réseau local » ou à un VPN.
:::

## API

Consultez l'[API du Terminal Web](/fr/docs/reference/webterminal-api) pour la référence complète de l'API.
