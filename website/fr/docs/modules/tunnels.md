# Tailscale / frp / cloudflared

Ces trois modules intègrent les programmes officiels en tant que processus gérés : redémarrage automatique, journaux collectés dans l'interface et configuration directement depuis l'interface. Gately n'embarque pas ces programmes ; ils s'installent, se mettent à jour et se restaurent en ligne depuis la page.

## Installation, mise à jour et retour arrière en ligne {#install}

Les pages frp, tunnel Cloudflare, Tailscale et [File Browser](/fr/docs/modules/tools#filebrowser) comportent en bas une carte « Installation en ligne » :

- **Télécharger et installer / Mettre à jour** : télécharge la dernière version stable depuis l'amont, la vérifie avec le SHA-256 publié par l'amont, contrôle qu'elle s'exécute sur cette machine, l'installe dans `bin/` sous le répertoire de configuration, puis redémarre les processus qui utilisent le programme. Une version déjà à jour n'est pas retéléchargée ;
- **Installer une version** : saisissez un numéro de version (par exemple `v0.65.0` pour frp, `2025.8.0` pour cloudflared, `1.90.2` pour Tailscale), utile aussi pour revenir à une version antérieure ;
- **Proxy de téléchargement** : réglable par programme, prend en charge http, https et socks5 et est enregistré au lancement de l'installation ; vide, le proxy de mise à jour des « Paramètres » est utilisé ;
- **Versions précédentes et retour arrière** : les versions remplacées par une mise à jour ou un retour arrière sont conservées dans `bin/.versions/`, 3 au plus. Si la nouvelle version pose problème, cliquez sur « Revenir » dans la liste ; la version actuelle est conservée elle aussi, vous pouvez donc y revenir ensuite ;
- **Désinstaller** : supprime le programme installé dans `bin/` et toutes les versions précédentes ; les paramètres du module sont conservés. Les processus qui utilisent le programme redémarrent et se rabattent sur un programme du même nom dans `PATH` ; sans lui, ils ne peuvent pas s'exécuter.

| Programme | Source | Fichiers installés |
| --- | --- | --- |
| frp | GitHub fatedier/frp | `frpc`, `frps` |
| cloudflared | GitHub cloudflare/cloudflared | `cloudflared` |
| Tailscale | Paquets statiques pkgs.tailscale.com, Linux uniquement | `tailscale`, `tailscaled` |
| FileBrowser Quantum | GitHub gtsteffaniak/filebrowser | `filebrowser` |

Pour macOS et Windows, Tailscale ne fournit pas de paquet directement exécutable : utilisez l'installateur officiel, puis indiquez le chemin dans les paramètres. La carte signale quand la plateforme actuelle n'est pas prise en charge.

## Rétablir les valeurs par défaut {#reset}

Le bouton « Rétablir les valeurs par défaut » de l'en-tête est distinct de la désinstallation : il arrête tous les processus du module, remet les paramètres (chemin du programme et proxy de téléchargement compris) à leurs valeurs par défaut et supprime les fichiers que Gately a générés pour lui. Les programmes installés sont conservés.

| Module | Contenu supprimé |
| --- | --- |
| frp | Toutes les instances frpc / frps, les proxys et les réglages des programmes, fichiers de configuration générés dans `frp/` |
| Tunnel Cloudflare | Tous les tunnels et les paramètres du programme |
| Tailscale | Tous les paramètres et l'état de connexion du répertoire d'état par défaut ; si le tailscaled géré par Gately tourne, il se déconnecte d'abord. Les répertoires d'état personnalisés et le tailscaled déjà présent sur le système restent intacts |
| File Browser | Toutes les instances et les paramètres du programme, avec les utilisateurs, liens de partage et caches de chaque instance ; les fichiers des répertoires partagés restent intacts |

L'ancienne configuration est conservée dans l'[historique de configuration](/fr/docs/guide/audit) et peut être restaurée en cas d'erreur. Pour tout retirer, désinstallez d'abord le programme, puis rétablissez les valeurs par défaut. Les API sont `DELETE /api/{module}/install` (désinstallation) et `POST /api/{module}/reset` (rétablissement des valeurs par défaut).

## Emplacement des programmes

Les exécutables sont recherchés dans cet ordre :

1. Le chemin défini dans les paramètres du module ;
2. Le sous-répertoire `bin/` du répertoire de configuration (emplacement de l'installation en ligne) ;
3. `PATH`.

Quand un chemin est indiqué, la version installée en ligne n'est pas utilisée et la carte le signale. Vous pouvez aussi placer le binaire vous-même dans le répertoire de configuration, avec Docker par exemple `./gately-data/bin/frpc`. Tailscale nécessite à la fois `tailscale` et `tailscaled` ; avec le tailscaled déjà présent sur le système, la commande `tailscale` installée devrait avoir la même version que le démon.

Les jetons et les clés sont stockés avec les permissions 0600. Le jeton cloudflared est transmis via une variable d'environnement, il n'apparaît donc pas dans la liste des processus.

## Tailscale

- Gère `tailscaled` (réseau en espace utilisateur, adapté aux conteneurs) ou se rattache à un démon système existant ;
- Clé d'authentification ou lien de connexion, Headscale pris en charge ;
- Routes de sous-réseau et nœuds de sortie ;
- Liste des nœuds et proxy SOCKS5 ;
- Port UDP WireGuard fixe, pratique pour autoriser les connexions directes à travers un pare-feu ;
- Arguments supplémentaires de tailscaled et variables d'environnement. Si iptables de l'hôte utilise le backend nf_tables, ajoutez `TS_DEBUG_FIREWALL_MODE=nftables` aux variables d'environnement, faute de quoi le transfert des sous-réseaux ne fonctionnera pas.

## frp

- **Client frpc** : ajoutez autant d'instances frpc que nécessaire ; chacune se connecte à un frps et s'exécute, s'active, s'arrête et redémarre indépendamment. Les proxys appartiennent à une instance et prennent en charge tcp / udp / http / https / stcp / xtcp ; modifier un proxy ne redémarre que son instance ;
- **Adresse d’accès des proxys** : frpc ne connaît pas les ports HTTP / HTTPS ni le domaine racine des sous-domaines de frps ; renseignez-les dans l’**Adresse d’accès** de l’instance frpc (un clic les reprend d’une instance frps locale ; si un proxy inverse ou un CDN se trouve devant frps, indiquez le schéma et le port réellement utilisés par les visiteurs). Les domaines des proxys http / https, y compris le nom complet formé à partir d’un sous-domaine, s’ouvrent alors d’un clic et leur URL complète peut être copiée ; sans cela ils restent en texte. L’adresse d’accès sert uniquement aux liens et la modifier ne redémarre pas frpc. Les domaines personnalisés sont enregistrés en minuscules et en punycode comme dans les Services web, et les valeurs avec un port ou un chemin sont refusées ; un sous-domaine doit tenir sur un seul niveau, comme `nas`, comme l’exige frps ;
- **Serveur frps** : ajoutez plusieurs instances frps sur des ports différents, chacune avec sa liste d'autorisation de ports, ses sous-domaines et son tableau de bord ;
- **Mise à niveau depuis une ancienne version** : les réglages uniques frpc / frps de la 0.5.9 et antérieures sont migrés automatiquement vers des instances nommées `frpc` / `frps` ; les proxys existants appartiennent à cette instance frpc et les chemins des programmes passent dans la carte « Programmes » en bas de la page ;
- **Adresses d'écoute de frps** : « Adresse d'écoute » définit où écoute le port de connexion de frpc, et « Adresse d'écoute des proxys » où écoutent les ports HTTP / HTTPS et les ports distants des proxys tcp / udp ; vides, elles écoutent sur toutes les adresses. Pour placer frps derrière un proxy inverse comme Gately, mettez `127.0.0.1` comme adresse d'écoute, créez dans le Service web un site en proxy inverse vers `127.0.0.1:<port d'écoute>` et choisissez websocket ou wss comme protocole de transport de frpc vers ce site ; laissez l'adresse d'écoute des proxys vide pour que les ports distants des proxys tcp restent joignables de l'extérieur. À l'import d'une configuration frps sans `proxyBindAddr`, `bindAddr` est repris, comme le fait frps ;
- **Contrôle d'accès par IP source de frps** : chaque instance frps peut utiliser une liste d'autorisation ou de blocage pour décider, selon l'IP source, si frpc peut se connecter (au port d'écoute). La liste réunit des IP / sous-réseaux saisis à la main et des listes d'IP abonnées du Service web ; correspondre à l'une d'elles suffit. Le module Service web télécharge les listes abonnées périodiquement ; elles s'appliquent dès leur mise à jour, sans redémarrer frps, et sont vides jusqu'au premier téléchargement réussi. Le contrôle a lieu avant que frps vérifie le jeton : Gately ouvre un point de contrôle sur l'adresse de bouclage et ajoute `[[httpPlugins]]` (`ops = ["Login"]`) à la fin de la configuration frps générée ; seule la connexion de frpc est concernée, les ports distants des proxys tcp / udp et les ports HTTP / HTTPS restent ouverts comme avant. Les bannissements de la Prévention d'intrusion priment : une adresse bannie ne peut pas se connecter même dans un sous-réseau autorisé, et les instances sans contrôle d'accès respectent aussi la liste des bannis, que la synchronisation du pare-feu soit activée ou non. Un frpc refusé reçoit une erreur comme `gately: client address ... is not in the allowlist`, et la carte de l'instance affiche le nombre de connexions refusées depuis le démarrage de Gately. La liste est vérifiée à la connexion de frpc et porte sur les connexions de frpc au port d'écoute de frps : après un resserrement de la liste ou le bannissement d'une adresse, un frpc déjà connecté le reste et n'est refusé qu'à sa prochaine connexion ; redémarrez cette instance frps pour un effet immédiat. Les connexions visitor stcp / xtcp ne passent pas par la vérification de connexion ; pour les limiter, utilisez la synchronisation du pare-feu de la [Prévention d'intrusion](/fr/docs/modules/guard). Derrière un proxy inverse (adresse d'écoute `127.0.0.1`), frps voit l'adresse du proxy comme source ; faites alors le contrôle d'accès par IP sur le site du Service web  ; derrière le Routage par protocole ou la Redirection de ports, frps voit une adresse locale et la liste ne peut pas distinguer les visiteurs : définissez alors le contrôle d'accès par IP source sur la règle de routage ou de redirection;
- **Importer une configuration existante** : « Importer la configuration » dans l'en-tête de la page prend en charge les anciens `frpc.ini` / `frps.ini` (avec la section `[common]`) et le `toml` de frp 0.52+. Choisissez un fichier ou collez le contenu, puis importez dans une instance existante ou une nouvelle instance. Le résultat de l'analyse et les champs ignorés sont d'abord prévisualisés, puis vous confirmez l'import. Dans une instance frpc existante, vous pouvez ajouter (les proxys de même nom restent inchangés) ou remplacer tous les proxys de cette instance. Une instance existante conserve son état et une nouvelle instance est créée désactivée. L'import est aussi disponible via l'API `POST /api/frp/import`. Lors d'une migration depuis Lucky, chaque instance frpc / frps de Lucky est importée comme instance Gately.

## Tunnel Cloudflare

- Tunnels à jeton et tunnels temporaires trycloudflare ;
- Plusieurs tunnels avec redémarrage automatique ;
- Affichage de l'adresse publique.

### Routes {#cloudflared-ingress}

Les noms d'hôte publics et les services d'origine (routes) d'un tunnel à jeton sont stockés chez Cloudflare, et cloudflared en cours d'exécution les récupère automatiquement. Renseignez le **jeton d'API** facultatif dans les paramètres du tunnel : le bouton Routes de la carte permet alors de les modifier directement.

- Le jeton nécessite l'autorisation Cloudflare Tunnel : Edit, ainsi que DNS : Edit sur les zones pour créer automatiquement les enregistrements DNS. Le compte et l'ID du tunnel sont lus depuis le jeton du tunnel.
- Chaque règle comporte un nom d'hôte, une expression régulière de chemin facultative et un service (par exemple `http://localhost:8080`, `ssh://localhost:22` ou `http_status:404`). Les règles sont comparées dans l'ordre et la première correspondance l'emporte. Vous pouvez activer Ignorer la vérification TLS de l'origine et définir l'en-tête Host vers l'origine ; les autres paramètres d'origine sont conservés. Le service par défaut traite les requêtes qui ne correspondent à aucune règle ; il vaut `http_status:404` par défaut.
- Les modifications s'appliquent dès l'enregistrement, sans redémarrer le tunnel. Si les routes ont été modifiées entre-temps dans le tableau de bord Cloudflare ou ailleurs, l'enregistrement le signale et affiche Recharger afin de conserver ces modifications. Les services HTTP ne peuvent pas avoir de chemin : un `/` final isolé (comme `http://nas:5000/`) est retiré à l'enregistrement, car cloudflared rejetterait toute la configuration.
- Quand le tunnel est géré par un fichier de configuration local (la boîte de dialogue indique que les routes distantes ne s'appliquent pas), l'enregistrement demande une confirmation supplémentaire : une fois la configuration distante écrite, le tunnel passe en gestion distante et les routes du fichier de configuration local cessent de s'appliquer.
- Avec Synchroniser le DNS activé, chaque nom d'hôte reçoit un enregistrement CNAME proxifié pointant vers `<ID du tunnel>.cfargotunnel.com` ; les noms d'hôte qui ont déjà d'autres enregistrements, ou un CNAME vers le tunnel sans proxy (le trafic n'entre alors pas dans le tunnel), les conservent et sont listés dans le résultat. Les zones sont recherchées uniquement dans le compte du tunnel. Supprimer une règle conserve son enregistrement DNS ; supprimez-le dans Cloudflare si nécessaire.

## Comment choisir

| Scénario | Recommandation |
| --- | --- |
| Construire un réseau privé entre plusieurs appareils | Tailscale |
| Vous disposez d'un serveur public et souhaitez exposer des services internes à travers lui | frp |
| Pas d'IP publique ni de serveur, et vous voulez exposer rapidement un service web | Tunnel Cloudflare |
| Votre FAI fournit un NAT qui autorise le hole punching | [Traversée STUN](/fr/docs/modules/stun) |
