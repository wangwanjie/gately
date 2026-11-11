# Port Knocking

Autorisation par paquet unique (SPA, Single Packet Authorization) compatible avec [fwknop](https://www.cipherdyne.org/fwknop/) : les ports protégés sont normalement inaccessibles de l'extérieur. Le client envoie d'abord un paquet de knock chiffré, authentifié par HMAC et protégé contre le rejeu ; une fois l'authentification réussie, Gately ouvre l'accès à cette seule adresse source pour une durée limitée. Usage courant : protéger l'accès SSH direct. Les cas où le terminal Web ne suffit pas (scp, développement distant en IDE, redirection de ports) peuvent aussi se connecter en sécurité depuis Internet, et les scanners comme les attaques par force brute ne voient pas le port.

## Fonctionnalités

- **Compatible avec les clients fwknop** : utilisez directement le client officiel fwknop (Linux, macOS, WSL). Le format des clés est celui de `KEY_BASE64` et `HMAC_KEY_BASE64` générés par `fwknop -k`, et la page des règles génère en un clic la commande client et la section `~/.fwknoprc` ;
- **Sécurité** : les paquets sont chiffrés en AES-256-CBC ; le HMAC (SHA-256 / 384 / 512) est vérifié avant le déchiffrement, de sorte qu'un paquet avec une mauvaise clé n'atteint jamais le déchiffrement ; chaque paquet n'est utilisable qu'une fois (anti-rejeu) et un horodatage hors de la durée de validité (120 secondes par défaut) est rejeté ; vous pouvez limiter le nom d'utilisateur et exiger que le paquet indique l'adresse à autoriser ;
- **Blocage par le pare-feu** : les ports protégés sont inscrits dans le pare-feu du système (nftables, ou iptables + ipset quand le noyau n'a pas nf_tables) et seules les adresses ayant frappé avec succès sont autorisées, ce qui protège le sshd local et les autres services qui ne passent pas par Gately ;
- **Entrées de Gately autorisées par knock** : les règles de la redirection de ports, du routage par protocole et du relais intégré de STUN, ainsi que les routes individuelles du routage par protocole, peuvent choisir « Exiger un knock ». Gately décide lui-même, sans privilèges de pare-feu, ce qui fonctionne aussi sous Windows, macOS et dans les conteneurs sans `NET_ADMIN` ;
- **Un seul port ouvert** : le routage par protocole reconnaît les paquets de knock envoyés en TCP (`fwknop -P tcp`) ; le même port reçoit le knock et ouvre ensuite des routes comme SSH selon le résultat ;
- **Lié à la prévention d'intrusion** : un knock refusé publie l'événement « Knock refusé » ; le modèle intégré « Échec de knock » de la prévention d'intrusion peut bannir la source après un certain nombre d'échecs, et les knocks provenant d'adresses déjà bannies sont abandonnés directement ;
- Liste des autorisations en cours (temps restant, révocables) ; les knocks réussis et refusés sont consignés dans le journal d'exécution et peuvent être suivis dans le centre de notifications.

## Démarrage rapide : protéger le SSH local

1. Ouvrez « Port Knocking » et ajoutez une règle : saisissez `tcp/22` comme port, activez « Blocage par le pare-feu » ; laissez les clés vides pour qu'elles soient générées automatiquement ;
2. Sur la carte de la règle, cliquez sur « Client », saisissez le nom de domaine ou l'IP publique du serveur, puis copiez la ligne de commande ou la section `~/.fwknoprc` ;
3. Installez fwknop sur le client : macOS `brew install fwknop`, Debian / Ubuntu `apt install fwknop-client`, sous Windows dans WSL ;
4. Connectez-vous en SSH dans les 30 secondes suivant le knock :

```bash
fwknop -n gately-ssh && ssh user@example.com
```

Le paquet de knock est envoyé par défaut au port UDP 62201. Quand Gately se trouve derrière un routeur, il faut rediriger l'UDP 62201 avec le port protégé vers l'hôte de Gately ; avec « Ouvrir le port dans le pare-feu » activé, le pare-feu local (OpenWrt fw4, firewalld, ufw, etc.) autorise automatiquement le port de knock, et autorise aussi le port protégé une fois le blocage actif : seule la réussite du knock décide de la connexion.

::: warning Éviter de se bloquer soi-même
- « Réseau local sans knock » est activé par défaut : les adresses privées, les adresses de lien local et les plages IPv6 publiques des interfaces de la machine (le préfixe de l'interface s'il fait /64 ou moins ; seulement ce préfixe lui-même pour les /124 ou /128 courants chez les hébergeurs cloud) n'ont pas besoin de knock, et vous pouvez ajouter une liste blanche supplémentaire dans les paramètres. Les adresses du NAT opérateur (100.64.0.0/10) n'en font pas partie, car d'autres clients du même opérateur les utilisent aussi ; avec Tailscale, ajoutez 100.64.0.0/10 à la liste blanche ;
- Quand le blocage par le pare-feu est activé, les connexions déjà établies ne sont pas affectées, et elles restent aussi en place à l'expiration de l'autorisation ;
- Un arrêt normal de Gately supprime les règles de blocage ; en cas d'arrêt forcé (`kill -9`, `docker rm -f`), les règles restent jusqu'au prochain démarrage de Gately. Assurez-vous de pouvoir vous connecter à cette machine depuis le réseau local ou par une autre voie.
:::

## Règles

| Champ | Description |
| --- | --- |
| Ports | Ports que le client peut demander d'ouvrir, comme `tcp/22` ou `udp/51820`, 16 au maximum. Tous les ports du paramètre `-A` du client doivent figurer dans cette liste, sinon le knock entier est refusé |
| Blocage par le pare-feu | Une fois activé, ces ports sont bloqués dans le pare-feu du système et seules les adresses ayant frappé avec succès sont autorisées ; désactivé, le pare-feu n'est pas modifié et la règle sert uniquement aux entrées configurées sur « Exiger un knock » |
| Clé de chiffrement / clé HMAC | Encodées en base64, générées automatiquement si laissées vides (32 / 64 octets, comme `fwknop -k`) ; vous pouvez aussi saisir les clés d'un serveur fwknop existant. Dupliquer une règle génère de nouvelles clés ; modifier, désactiver ou supprimer une règle révoque toutes les autorisations qu'elle a produites |
| Algorithme HMAC | SHA-256 (valeur par défaut de fwknop), SHA-384, SHA-512, à faire correspondre à `--hmac-digest-type` du client |
| Durée d'accès | Durée pendant laquelle de nouvelles connexions sont possibles après un knock réussi, 30 secondes par défaut |
| Durée maximale côté client | Plafond appliqué quand le client demande une durée plus longue avec `-f` (300 secondes par défaut) ; 0 ignore la durée demandée par le client ; une demande plus courte que la durée d'accès est toujours retenue |
| Nom d'utilisateur imposé | N'accepte que les knocks dont le nom d'utilisateur du paquet a cette valeur (fwknop envoie par défaut le nom de connexion local, modifiable avec `-U`) |
| Adresse obligatoire | Refuse les paquets sans adresse (`fwknop -s`) ; le client doit indiquer l'adresse à autoriser avec `-R` ou `-a` |

Une règle référencée par la redirection de ports, le routage par protocole ou STUN ne peut pas être supprimée ; la carte liste les références.

### Quelle adresse est autorisée

- `-R` (recommandé) : le client interroge d'abord son IPv4 publique et l'écrit dans le paquet ; même intercepté en chemin, le paquet ne peut pas servir pour une autre adresse ;
- `-s` : le paquet ne contient pas d'adresse, c'est l'adresse source du paquet qui est autorisée. Quelqu'un qui intercepte le paquet peut le rejouer le premier depuis sa propre adresse ; activez « Adresse obligatoire » sur la règle pour refuser ces paquets ;
- `-a adresse` : autorise l'adresse indiquée, par exemple pour frapper à la place d'un autre appareil.

Le client fwknop 2.6 ne peut envoyer les paquets de knock qu'en IPv4 et n'autorise que des adresses IPv4 : le serveur doit donc être joignable en IPv4. Gately accepte lui-même aussi les paquets de knock reçus en IPv6 et autorise leur adresse source.

## Paramètres globaux

- **Port UDP de knock** : 62201 par défaut (port par défaut du client fwknop) ; 0 n'écoute pas en UDP et ne reçoit que les knocks acheminés par le routage par protocole. Sans règle activée, aucune écoute ;
- **Ouvrir le port dans le pare-feu** : voir plus haut ;
- **Réseau local sans knock / liste blanche sans knock** : n'affecte que le blocage par le pare-feu ; les entrées « Exiger un knock » sont contrôlées par leurs propres listes d'IP ;
- **Durée de validité des paquets** : écart maximal entre l'horodatage du paquet et l'heure locale, 120 secondes par défaut ; les horloges du client et du serveur doivent être exactes.

## Blocage par le pare-feu {#firewall}

| Environnement | Blocage par le pare-feu | Entrées de Gately autorisées par knock |
| --- | --- | --- |
| Linux (root ou `CAP_NET_ADMIN`, avec nftables) | Disponible | Disponible |
| Docker (réseau host + `cap_add: NET_ADMIN`) | Disponible, règles inscrites sur l'hôte | Disponible |
| Linux sans nf_tables, comme Synology DSM | Utilise iptables (legacy) + ipset, déjà inclus dans l'image Docker officielle | Disponible |
| Windows, macOS, FreeBSD, Linux sans privilèges | Indisponible, la raison est affichée en haut de la page | Disponible |

Gately crée dans nftables une table dédiée `inet gately_knock` sans toucher aux règles existantes du système : la chaîne `prerouting` (priorité -160, après le suivi de connexion et avant la traduction d'adresse de destination) confie les paquets destinés aux ports protégés à la chaîne `gate`, qui laisse passer successivement le trafic dont la destination n'est pas cette machine (les connexions relayées vers d'autres appareils ne sont pas affectées), l'interface de bouclage, les connexions établies, les paquets TCP qui n'initient pas de connexion, et les triplets adresse + protocole + port des réseaux sans knock et de l'ensemble des autorisations par knock ; tout le reste est abandonné. Le contrôle se fait dans prerouting : un port mappé vers un conteneur Docker est donc bloqué selon le port avant mappage. Chaque autorisation est un élément d'ensemble avec délai d'expiration, supprimé par le noyau à l'échéance.

Sans nf_tables, la table mangle d'iptables est utilisée : `PREROUTING` saute vers `GATELY_KNOCK`, avec un ensemble ipset par port protégé (`gkn4_tcp_22`, etc., avec délai d'expiration). Si la table ou le saut est supprimé (rechargement du pare-feu, `nft flush ruleset`, etc.), ils sont réécrits en 10 secondes avec les autorisations encore valides. « Afficher les règles du pare-feu » sur la page montre les règles actuellement inscrites.

::: tip SSH arrivant par redirection de ports
Une connexion redirigée par Gately vers `127.0.0.1:22` de la machine passe par l'interface de bouclage et n'est pas soumise au blocage du pare-feu. Dans ce cas, choisissez « Exiger un knock » sur la règle de redirection de ports, ou protégez le port de redirection exposé avec le blocage par le pare-feu.
:::

## Entrées de Gately autorisées par knock {#gately-entries}

Dans le « Contrôle d'accès par IP » de la redirection de ports, du routage par protocole et de STUN (relais intégré), vous pouvez choisir une règle de knock comme « Exiger un knock » : l'adresse source doit détenir une autorisation non expirée de cette règle pour établir une nouvelle connexion (UDP est jugé par paquet) ; les autres connexions sont coupées. Combiné à une liste d'autorisation / de blocage, les deux conditions doivent être remplies. La règle de knock peut désactiver le « Blocage par le pare-feu » et servir uniquement à ces entrées.

Le routage par protocole permet aussi de choisir « Exiger un knock » sur une route individuelle, par exemple : sur le port 443, la route HTTPS reste ouverte à tous et la route SSH n'est ouverte qu'aux adresses ayant frappé avec succès.

### Ouvrir un seul port

Ajoutez à une règle de routage par protocole une route « Port Knocking » (protocole `spa`) ; le client envoie le paquet de knock en TCP au même port :

```bash
fwknop -A tcp/443 -P tcp -p 443 -R -D example.com --use-hmac \
  --key-base64-rijndael <encryption key> --key-base64-hmac <HMAC key>
ssh -p 443 user@example.com
```

- La route de knock remet le paquet au module Port Knocking ; elle ne relaie ni ne répond ; le port de `-A` doit figurer dans la liste des ports de la règle de knock (par exemple `tcp/443`) ;
- Quand toute la règle exige un knock, une source n'ayant pas frappé ne peut utiliser que la route de knock, et n'accède aux autres routes et à la cible par défaut qu'après le knock ;
- Les paquets de knock ne sont reconnus que si la règle contient une route de knock, sans effet sur la détection de protocole des autres règles ;
- Le port UDP de la règle de knock peut être mis à 0 : seul le port du routage par protocole est alors exposé;
- Les ports du routage par protocole qui portent une route de knock et le port UDP de knock ne peuvent pas figurer dans une règle avec blocage par pare-feu (vérifié à l'enregistrement et à l'activation, car les bloquer empêcherait les paquets de knock eux-mêmes d'arriver), et ils sont toujours exclus lors de la génération des règles du pare-feu.

## Lien avec la prévention d'intrusion

Les knocks refusés publient l'événement « Knock refusé » (son champ `via` vaut `udp` ou `tcp`), au plus une fois toutes les 5 secondes par source. L'adresse source d'un paquet UDP peut être usurpée : pour qu'un attaquant ne puisse pas se servir de la prévention d'intrusion afin de bannir n'importe quelle IP, les paquets UDP dont le HMAC échoue sont seulement comptés, sans événement ; l'événement n'est publié que si le paquet a passé le HMAC (il vient d'un client qui détient la clé) mais a été refusé par la règle (échec de déchiffrement, rejeu, expiration, demande de port non autorisé, nom d'utilisateur différent, etc.). Les paquets reçus sur un port TCP du routage par protocole viennent d'une poignée de main complète et ne peuvent pas être usurpés : un HMAC non concordant y publie aussi l'événement. Créez dans la prévention d'intrusion une règle avec le modèle « Échec de knock » (par défaut 5 échecs en 10 minutes entraînent un bannissement d'un jour) : une source qui continue d'envoyer des paquets forgés est bannie, et les knocks d'une adresse bannie sont abandonnés directement. Les données qui n'ont pas le format d'un knock (scans de ports, autres protocoles) sont seulement comptées, sans événement.

Avant l'authentification, le débit est limité par plage source (au plus 10 paquets par seconde par /24 IPv4 et par /64 IPv6), puis par une limite globale de 200 par seconde ; le surplus est abandonné, de sorte qu'un déluge de paquets forgés ne sature pas le processeur et que le déluge d'une source n'évince pas les knocks normaux des autres. Les paquets dont le HMAC échoue ne sont pas journalisés un par un : ils sont résumés chaque minute en une ligne « M paquets de knock invalides de N sources ».

## Limites

- Seul le mode Rijndael (AES) CBC de fwknop avec HMAC est pris en charge ; GnuPG, l'ancien mode IV et les paquets sans HMAC ne le sont pas ;
- Aucun message de commande n'est exécuté (`fwknop -C`) et les demandes d'accès NAT (`-N`) ne sont pas prises en charge ;
- Les autorisations de knock sont conservées en mémoire : après un redémarrage de Gately, il faut frapper à nouveau (les connexions déjà établies ne sont pas affectées). Les traces anti-rejeu ne sont elles aussi qu'en mémoire : après un redémarrage, tout paquet dont l'horodatage précède le démarrage est refusé ; si l'horloge du client retarde sur celle du serveur, un knock dans les premières secondes après le redémarrage peut être refusé, il suffit de recommencer.
