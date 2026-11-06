# Prévention d'intrusion

Prévention d'intrusion à la fail2ban : Gately repère les attaques par force brute dans les journaux, pose des pièges sur les ports inutilisés, compte les scans de ports, ajoute les IP malveillantes à une liste de bannissement et la synchronise avec le pare-feu du système (nftables), qui rejette tout leur trafic dans le noyau.

## Fonctionnalités

- **Règles de journaux** : lisent des fichiers journaux, le journal systemd, la sortie d'une commande (comme `logread -f` sur OpenWrt) ou les journaux d'un conteneur Docker, repèrent les échecs avec des expressions régulières et bannissent l'IP qui atteint le nombre d'échecs dans la fenêtre de temps. Des modèles sont fournis pour SSH, Dropbear, Nginx, Vaultwarden, Home Assistant, Proxmox VE et d'autres, et l'éditeur permet de coller des lignes d'exemple pour tester les filtres ;
- **Événements Gately** : les échecs de connexion à la console et les blocages du WAF peuvent aussi alimenter une règle, sans expression régulière ;
- **Sondage de chemins web** : compte directement les requêtes traitées par le service web par écouteur, chemin et code d’état, sans lire de journal ; les sources qui demandent des chemins leurres comme `/.env` ou `/.git`, ou qui scannent sans cesse des domaines inconnus, sont bannies ;
- **Ports pièges** : pièges sur des ports que cet hôte n'utilise pas (23, 135, 1433, 3389, 5900, 6379 et d'autres par défaut). Toute source qui s'y connecte est bannie ;
- **Détection de scan de ports** : une source qui se connecte à des ports fermés de cet hôte plus souvent que le seuil par minute est bannie ;
- **Une seule liste de bannissement** : les bannissements manuels (réseaux CIDR et bannissements permanents compris) et ceux déclenchés par les échecs d'authentification du service web sont gérés ici. Un bannissement manuel ne peut pas couvrir les adresses de bouclage, car toutes les connexions relayées par cette machine apparaissent avec cette adresse. Les bannissements sont enregistrés et restaurés après un redémarrage ;
- **Bannissements progressifs** : si une adresse est de nouveau bannie dans les 30 jours, la durée double jusqu'à un maximum ;
- **Liste blanche** : les adresses du LAN, de bouclage, de 100.64.0.0/10 (Tailscale) et les réseaux IPv6 /64 des interfaces de cet hôte sont ignorés par défaut (les bannissements portent sur le /64, ce qui évite de bannir avec le fautif les appareils du LAN en IPv6 publique), et vous pouvez ajouter des IP ou des réseaux. Une seule adresse IPv6 en liste blanche protège tout son /64.

## Portée des bannissements

| Environnement | Effet d'un bannissement | Ports pièges | Détection de scan |
| --- | --- | --- | --- |
| Linux (root ou `CAP_NET_ADMIN`, nftables installé) | Tout le trafic est rejeté dans le noyau, y compris SSH, les autres programmes et le trafic redirigé vers des conteneurs Docker et des appareils du LAN | Gérés par le noyau, repèrent les scans qui n'envoient que des SYN | Disponible |
| Docker (réseau host + `cap_add: NET_ADMIN`) | Comme ci-dessus, les règles sont écrites sur l'hôte | Idem | Disponible |
| Paquet fnOS, OpenWrt 22.03 et versions ultérieures | Comme ci-dessus (exécution en root, nftables inclus) | Idem | Disponible |
| Autres cas (Windows, macOS, FreeBSD, Linux sans nftables ou sans autorisation) | Uniquement les services de Gately : redirection de ports, service web, STUN, WebDAV, FTP, SMB, connexions frps et console d'administration | Gately écoute lui-même, ne repère que les connexions qui terminent la poignée de main | Indisponible |

Le haut de la page affiche l'état de la synchronisation du pare-feu et, si elle est indisponible, la raison et la marche à suivre.

## Règles du pare-feu

Avec « Synchroniser avec le pare-feu » activé, Gately crée sa propre table `inet gately` dans nftables et ne touche pas aux règles du système (fw4, Docker, ufw, etc.) :

- La chaîne `prerouting` (priorité -190, après le suivi de connexion et avant le NAT de destination) rejette les paquets venant des adresses bannies, qu'ils soient destinés à cet hôte ou redirigés vers un conteneur ou un appareil du LAN. Les réponses aux connexions que cet hôte ouvre lui-même (DNS, miroirs de paquets, etc.) passent toujours, même si l'autre extrémité est bannie. En IPv6, le bannissement porte sur le /64 ;
- La chaîne `input` ne regarde que les nouvelles connexions vers cet hôte, pour les ports pièges et la détection de scan. Les adresses en liste blanche sont ignorées ;
- Toute la table est supprimée quand la synchronisation est désactivée ou que Gately s'arrête. Si un autre programme la supprime, Gately la réécrit en moins de 10 secondes.

« Voir les règles » sur la page affiche l'ensemble des règles en place.

::: tip Docker
Utilisez `network_mode: host` et accordez `NET_ADMIN` pour que les règles soient écrites sur l'hôte :

```yaml
    network_mode: host
    cap_add:
      - NET_ADMIN
```

Sans cela, les bannissements s'appliquent toujours aux services de Gately. Pour lire les journaux de l'hôte, montez le répertoire des journaux dans le conteneur, par exemple `- /var/log:/var/log/host:ro`, et indiquez `/var/log/host/auth.log` dans la règle. La source « journal systemd » n'est pas disponible dans un conteneur.
:::

## Règles de journaux

| Source | À indiquer | Remarques |
| --- | --- | --- |
| Fichier journal | Chemins de fichiers, un par ligne | Lit les nouvelles lignes depuis la fin du fichier et suit la rotation (renommage ou troncature) |
| Journal systemd | Unités de service comme `ssh.service` | Lu via `journalctl -f` |
| Sortie de commande | Une commande comme `logread -f` | Relancée si elle s'arrête |
| Conteneur Docker | Nom du conteneur | Lu via la connexion Docker du module Docker |
| Événement Gately | Échec de connexion / blocage WAF | Les événements contiennent l'IP source, aucun filtre n'est nécessaire |
| Sondage de chemins web | Écouteurs (facultatif) et règles de chemin | Compte les requêtes du service web, voir plus bas |

Les filtres sont des expressions régulières Go (syntaxe RE2), une par ligne, où `<HOST>` représente l'adresse source, par exemple :

```
Failed password for (?:invalid user )?\S* ?from <HOST>
```

Les lignes correspondant à une « Exclusion » ne sont pas comptées. La fenêtre de temps, le nombre d'échecs et la durée du bannissement se règlent par règle ; la durée peut être celle par défaut, personnalisée ou permanente.

## Sondage de chemins web

Avec la source « Sondage de chemins web », la règle compte directement les requêtes traitées par le service web, sans lire de journal :

- **Écouteurs** : un ou plusieurs ; vide pour tous ;
- **Chemins** : un par ligne ; vide pour tous. `*` correspond à n’importe quels caractères (y compris `/`), le chemin entier doit correspondre, sans tenir compte de la casse. Par exemple `*/.env` correspond à `/.env` et `/api/.env`. Les lignes commençant par `~` sont des expressions régulières Go. Des variantes comme `//.env` ou `/a/../.env` correspondent aussi via le chemin normalisé ;
- **Chemins exclus** : même syntaxe ; les requêtes correspondantes ne sont pas comptées ;
- **Ce qui compte** : par défaut seuls 404 et 444 (contenu inexistant, ou connexion fermée par un site « Fermer la connexion ») ; les requêtes vers des pages existantes ne comptent pas. Avec « Tout code d’état », chaque réponse compte, ce qui convient aux chemins leurres, car les applications monopage renvoient 200 pour n’importe quel chemin. « Tout code d’état » exige des règles de chemin ou « Domaines inconnus uniquement », sinon l’enregistrement est refusé ;
- **Refus du contrôle d’accès** : quand En cas de refus d’un site vaut Fermer la connexion, les requêtes rejetées par un bannissement, le contrôle d’accès IP ou le filtre par pays comptent comme 403 sur les sites ordinaires, pour ne pas bannir les visiteurs légitimes hors liste, et comme 444 sur le site par défaut, comme un site par défaut « Fermer la connexion » ; un site par défaut en proxy inverse avec liste d’autorisation pour vos IP continue donc de compter les scans de domaines inconnus ;
- **Domaines inconnus uniquement** : ne compte que les requêtes dont le domaine ne correspond à aucun site, c’est-à-dire traitées par le site par défaut ou, à défaut, répondues par 404 par l’écouteur.

Trois modèles sont fournis :

| Modèle | Condition | Remarques |
| --- | --- | --- |
| Chemins leurres web | `/.env`, `/.git`, `wp-login.php`, `phpinfo.php`, `phpmyadmin` et d’autres, tout code d’état, 1 fois | Bannit 1 jour. Si un site exécute réellement WordPress, phpMyAdmin ou autre, supprimez d’abord les lignes correspondantes |
| Scans de domaines inconnus | Domaines inconnus uniquement, 5 fois en 1 minute | Bannit 1 jour. À associer à un site par défaut « Fermer la connexion » : le scanner est banni après quelques 444 et, avec la synchronisation du pare-feu, les connexions suivantes sont rejetées |
| Sondage web 404 | Tous les chemins, 10 fois en 1 minute | Les chemins que les navigateurs demandent d’eux-mêmes, comme favicon, robots.txt et les fichiers `.map`, sont exclus. Les 404 renvoyés par les backends proxifiés comptent aussi : des miniatures de galerie manquantes ou une application qui interroge un point d’accès absent peuvent la déclencher ; consultez le journal d’accès avant de l’activer et augmentez le nombre ou ajoutez des chemins exclus si besoin |

« Bannir les sondages » sur la carte d’un écouteur du service web ouvre cette page avec cet écouteur sélectionné et le modèle de leurres appliqué ; la règle est stockée et appliquée par la protection contre les intrusions. En modifiant une règle, vous pouvez coller des chemins de requête pour tester les règles de chemin.

::: warning Portée du bannissement
Quel que soit l’écouteur déclencheur, le bannissement entre dans la même liste et s’applique à tous les écouteurs, sites et autres services de Gately ; avec la synchronisation du pare-feu, le pare-feu du système rejette aussi tout le trafic de cette adresse.
:::

L’adresse source est la même que dans le journal d’accès et n’est restaurée que par le protocole PROXY. Pour les sites derrière un CDN qui transmet l’IP réelle dans un en-tête, c’est le nœud du CDN qui serait banni : n’utilisez pas le sondage de chemins sur ces écouteurs. Si Gately est derrière un autre proxy inverse sur la même machine, la source est une adresse de bouclage : avec « Ignorer les adresses du LAN », elle n’est jamais bannie, et la règle reste sans effet.

## Compromis de la détection de scan

La détection compte les nouvelles connexions vers des ports **sur lesquels cet hôte n'écoute pas**. Les ports TCP en écoute sont actualisés toutes les 10 secondes et comptent comme ouverts ; les ports redirigés ailleurs par d'autres règles de pare-feu ne passent pas par la chaîne `input` et ne sont pas comptés. Le seuil par défaut est de 12 par minute. Un client normal réessaie un port fermé environ 5 fois par minute, donc ne descendez pas sous 10.

Les ports pièges en cours d'utilisation sont exclus automatiquement pour ne pas bannir de visiteurs légitimes.

## Mode strict IPv6

IPv6 n'a pas de NAT : chaque appareil possède une adresse publique, et dès qu'elle apparaît dans le DNS ou dans les journaux de transparence des certificats, les scanners peuvent s'y connecter directement. L'option « Mode strict IPv6 » de la détection de scan de ports passe IPv6 en liste d'autorisation : les sources IPv6 ne peuvent se connecter qu'aux ports ouverts (ports en écoute plus « Ports ouverts supplémentaires ») et la première connexion à un autre port entraîne un bannissement, sans seuil. IPv4 utilise toujours le seuil.

- Le bannissement s'applique à tout le /64 : les autres appareils du même /64 sont bannis aussi. Les adresses de la liste blanche et celles couvertes par « Ignorer les adresses privées » sont épargnées.
- Un port qui commence à écouter est considéré ouvert après environ 10 secondes, et le mode prend effet environ 15 secondes après l'activation ou un redémarrage ; d'ici là, le seuil s'applique. Le seuil s'applique aussi quand les ports en écoute ne peuvent pas être lus.
- Les ports utilisés par d'autres programmes via des règles de pare-feu doivent d'abord être ajoutés aux « Ports ouverts supplémentaires », sinon les sources qui s'y connectent seront bannies.
- Seules les nouvelles connexions TCP sont comptées. La synchronisation avec le pare-feu doit être active et la détection de scan de ports activée.

## Lien avec le service web

Les bannissements de « Bannir après des échecs d'authentification » du service web vont dans la même liste ; avec la synchronisation activée, ils s'appliquent donc à tout l'hôte et plus seulement aux sites web. Avec « Ignorer les adresses du LAN » activé, les adresses du LAN ne sont jamais bannies automatiquement.

Le service web et le WAF ne connaissent l'adresse réelle du client que par le protocole PROXY. Pour les sites derrière un CDN qui transmet l'IP réelle dans un en-tête, les bannissements après échec d'authentification et la règle « Blocages du WAF » banniraient les nœuds du CDN : ne les activez pas pour ces sites.

## Notifications

Les bannissements déclenchent l'événement `Prévention d'intrusion : IP bannie`, auquel vous pouvez vous abonner dans le [Centre de notifications](/fr/docs/modules/notify). Les adresses bannies par les ports pièges et la détection de scan lors d'une même vérification sont regroupées en une seule notification.

## Vérification

Vérifié dans des espaces de noms réseau isolés sur fnOS (Debian 12, nftables 1.0.6) et iStoreOS 24.10 (nftables 1.1.1) : les ports pièges et la détection de scan (IPv4 et IPv6) bannissent dans le noyau et sont importés dans la liste ; les fichiers journaux (rotation comprise), le journal systemd, les journaux de conteneurs Docker et les échecs de connexion à la console déclenchent des bannissements ; les bannissements manuels de réseaux et permanents, le débannissement, les durées progressives, la restauration après redémarrage, la réécriture après suppression externe de la table et le nettoyage à l'arrêt fonctionnent comme prévu ; un conteneur Docker sans `NET_ADMIN` est signalé comme manquant d'autorisation et passe aux ports pièges en espace utilisateur.
