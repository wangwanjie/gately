# Routage par protocole

Plusieurs services partagent un même port public (par exemple un site web, un VPN, SSH et le Bureau à distance, tous sur le 443). Gately lit les premières données envoyées par le client pour identifier le protocole, examine ensuite le domaine (SNI) et l'ALPN de la poignée de main TLS ou le Host HTTP, puis transmet toute la connexion telle quelle au backend correspondant. Rien n'est déchiffré ni terminé ici, les certificats TLS restent donc sur les backends. Cela remplit le rôle des règles `req_ssl_sni` de haproxy et de sslh réunis.

Le routage par domaine (routage SNI) en est un cas particulier : ajoutez une route TLS et renseignez ses domaines.

Chaque règle comporte :

- L'écoute : le protocole (TCP uniquement ; tcp est double pile, tcp4 / tcp6 se limitent à une seule famille d'adresses), l'adresse d'écoute et les ports (`443`, `443,8443` ou une plage) ;
- Des routes : chaque route a un protocole, un nom, des adresses cibles (plusieurs adresses sont essayées tour à tour, avec basculement), un port cible facultatif (vide signifie le port d'écoute) et un réglage du protocole PROXY (désactivé, v1 ou v2). Le réglage PROXY appartient à la route, car certains backends l'exigent et d'autres le rejettent ;
- Une cible par défaut : les adresses cibles, les ports cibles et le réglage du protocole PROXY de la règle elle-même, utilisée quand aucune route ne correspond ou que le protocole n'est pas reconnu ;
- Un délai de détection : le nombre de secondes d'attente pour recevoir du client des données reconnaissables ; 0 correspond à la valeur par défaut de 5 secondes.

## Protocoles reconnus

| Protocole de la route | Reconnu par | Correspondance supplémentaire |
| --- | --- | --- |
| TLS | L'enregistrement de poignée de main TLS (ClientHello) | Domaine (SNI), ALPN |
| HTTP | La méthode et la version `HTTP/` de la ligne de requête, y compris CONNECT, les méthodes WebDAV et HTTP/2 en connaissance préalable (h2c) | Domaine (en-tête Host) |
| SSH | La chaîne de version du client `SSH-` | — |
| RDP (Bureau à distance) | L'en-tête TPKT et la demande de connexion X.224 | — |
| SOCKS4 / SOCKS5 | Les requêtes SOCKS4 et SOCKS4a et la négociation de méthode SOCKS5 | — |
| OpenVPN (TCP) | Le paquet de réinitialisation forcée (hard reset) du client, y compris tls-auth, tls-crypt et tls-crypt-v2 | — |
| XMPP | Une déclaration XML ou `<stream:stream` | — |
| MQTT | Le paquet CONNECT (3.1, 3.1.1, 5.0) | — |
| PostgreSQL | Le message de démarrage, SSLRequest et GSSENCRequest | — |
| Minecraft (édition Java) | Le paquet de handshake ; le ping de la liste des serveurs de la 1.6 et antérieures | Adresse du serveur |
| RTSP | La version `RTSP/1.0` ou `RTSP/2.0` dans la ligne de requête (caméras IP, NVR) | — |
| SIP | La version `SIP/2.0` dans la ligne de requête (TCP) | — |
| Redis | Les commandes envoyées sous forme de tableaux RESP | — |
| AMQP | L'en-tête de protocole `AMQP` (0-9-1 et 1.0, RabbitMQ et autres) | — |
| SMB | Les requêtes de négociation SMB1, SMB2 et SMB3 dans un message de session NetBIOS | — |
| Git | La première pkt-line du protocole `git://` (git daemon) | — |
| IRC | La commande CAP, PASS, NICK ou USER que le client envoie en premier | — |
| ADB | Le message CNXN de l'Android Debug Bridge (adb connect) | — |
| BitTorrent | Une poignée de main entre pairs non chiffrée | — |
| Délai d'attente | Le client n'envoie rien pendant le délai de détection | — |

## Correspondance

Les routes TLS, HTTP et Minecraft correspondent par domaine (Minecraft utilise l'adresse du serveur saisie par le client, lue dans le handshake ; les données ajoutées par Forge, BungeeCord ou TCPShield sont ignorées) : d'abord les domaines exacts (`vpn.example.com`), puis le joker le plus long (`*.example.com`, à n'importe quelle profondeur), puis `*` (toute connexion qui transporte un domaine), enfin les routes sans domaine (toutes les connexions de ce protocole, y compris celles sans SNI ni Host). Les domaines sont insensibles à la casse. À l’enregistrement, les domaines sont mis en minuscules et les domaines internationalisés convertis en punycode ; les valeurs avec un port, un chemin ou `@` sont refusées.

Les routes TLS peuvent aussi lister des valeurs ALPN : la route correspond seulement si le client propose l'une d'elles. Pour un même domaine, les routes avec ALPN sont prioritaires, et quand l'ALPN ne correspond pas, la recherche se poursuit vers des domaines plus larges. Par exemple, envoyez `acme-tls/1` au programme qui répond à la validation de certificat TLS-ALPN-01, ou `postgresql` aux connexions TLS directes de PostgreSQL 17. Au sein d'un protocole, un domaine (avec un ALPN donné) ne peut appartenir qu'à une seule route ; les autres protocoles acceptent une route chacun.

« Délai d'attente » sert aux protocoles où le serveur parle en premier, comme VNC, MySQL, FTP, SMTP, POP3 et IMAP : leurs clients se connectent et attendent le serveur, si bien que Gately ne reçoit rien et ne peut pas savoir duquel il s'agit. Chaque port ne peut donc envoyer qu'un seul d'entre eux vers la route Délai d'attente, et ces connexions sont transférées à l'expiration du délai de détection. Le délai de détection accepte une décimale : les clients TLS, SSH, HTTP et similaires envoient leur premier paquet quelques millisecondes après la poignée de main, donc avec une route Délai d'attente, 0,3 à 1 seconde suffit, et c'est tout ce qu'attendent VNC et les connexions similaires.

Ces connexions vont vers la cible par défaut : aucune route ne correspond ; les données n'appartiennent à aucun des protocoles ci-dessus (par exemple des protocoles sans signature comme Shadowsocks, VMess ou BitTorrent chiffré, et des protocoles dont le premier paquet se résume à un champ de longueur et se confond facilement avec d'autres, comme MongoDB ou DNS over TCP) ; le ClientHello ne peut pas être analysé ; ou le client n'envoie rien et il n'existe aucune route Délai d'attente. Sans cible par défaut, ces connexions sont fermées et comptées comme « coupées ». Un client qui se déconnecte avant la fin de la détection est abandonné.

## Désactiver temporairement une route

Chaque route a un interrupteur à gauche, et les lignes de route de la carte de la règle peuvent aussi être basculées directement. Une route désactivée conserve toute sa configuration mais ne correspond plus ; les connexions qui lui revenaient passent aux routes plus larges ou à la cible par défaut selon les règles ci-dessus. Les routes désactivées sont exclues de la vérification des conflits de domaine, d'ALPN et de protocole : vous pouvez donc garder une route active et plusieurs routes alternatives désactivées pour le même domaine ou protocole, et changer de backend en basculant les interrupteurs. Une route désactivée reste validée. Le basculement est une mise à jour à chaud : l'écoute n'est pas interrompue et les compteurs sont conservés et reprennent après réactivation. Dans l'API, il s'agit du champ `disabled` de la route, activée par défaut.

## Remarques

- TCP uniquement. Le trafic UDP tel que HTTP/3 (QUIC) ou WireGuard passe par une règle distincte ;
- Les variantes chiffrées de SIP, Redis, AMQP, IRC et des protocoles similaires passent par TLS ; routez-les avec des routes TLS par domaine ou ALPN ;
- Le ClientHello peut être réparti sur plusieurs enregistrements TLS et atteindre 64 Ko ; les lignes de requête HTTP, RTSP et SIP peuvent atteindre 8 Ko, et en HTTP, jusqu'à 32 Ko d'en-têtes de requête sont lus pour trouver le Host ;
- Chaque connexion est analysée une seule fois : les requêtes suivantes sur la même connexion HTTP keep-alive vont au backend choisi la première fois ;
- La carte de la règle indique combien de connexions chaque route et la cible par défaut ont reçues ; les entrées de débogage du journal d'exécution consignent le protocole, le SNI, l'ALPN ou le Host détecté pour chaque connexion ;
- Lors d'une mise à jour à chaud d'une règle (note, routes, cible par défaut, liste d'IP source, etc.), les routes dont le protocole, les domaines et l'ALPN ne changent pas gardent leur nombre de correspondances, y compris si elles sont déplacées, renommées ou changent de cible ; les nouvelles routes et celles dont le protocole, les domaines ou l'ALPN changent repartent de 0 ; désactiver puis réactiver la règle ou la redémarrer fait repartir tous les compteurs de 0 ;
- Les modifications des routes, de la cible par défaut et du délai de détection s'appliquent immédiatement sans arrêter l'écoute, et les connexions ouvertes sont conservées.
- Chaque règle peut utiliser une liste d'autorisation ou de blocage par IP source (pouvant utiliser les listes d'IP abonnées des Services Web), vérifiée avant la détection du protocole : une connexion qui échoue est fermée sans lire de données et ne compte pas comme une correspondance de route ; les bannissements de la Prévention des intrusions passent en premier. Modifier la liste n'interrompt pas l'écoute et la carte de la règle affiche le nombre de refus. Le fonctionnement est le même que pour la [Redirection de ports](/fr/docs/modules/portforward#controle-d-acces-par-ip-source). Chaque règle peut aussi avoir une **Liste de refus prioritaire** (`ipDenyList` / `ipDenySets`) qui fonctionne indépendamment du mode liste d'autorisation ou de blocage : une source qui correspond est toujours refusée, avant les deux listes, et une adresse également présente dans la liste d'autorisation est refusée aussi. Elle accepte des IP ou sous-réseaux saisis à la main et des listes abonnées, pour un refus prioritaire, par exemple abonné à un flux d'IP malveillantes tout en n'admettant que vos propres sous-réseaux.
- Une règle entière ou une route individuelle peut choisir « Exiger un knock » : seules les sources ayant frappé avec succès via fwknop peuvent l'utiliser ; après avoir ajouté une route « Port Knocking » (protocole `spa`), le client peut envoyer le paquet de knock au même port avec `fwknop -P tcp`, et un seul port est exposé. Voir [Port Knocking](/fr/docs/modules/knock#gately-entries).

## Exemple : remplacer haproxy

Une configuration haproxy comme celle-ci :

```text
frontend https
    bind 192.0.2.10:443
    mode tcp
    tcp-request inspect-delay 5s
    tcp-request content accept if { req_ssl_hello_type 1 }
    use_backend ocserv if { req_ssl_sni -i tz.example.com }
    default_backend web
backend ocserv
    server ocserv 127.0.0.1:443 send-proxy-v2
backend web
    server web 127.0.0.1:8443 send-proxy-v2
```

devient une seule règle Gately :

| Réglage | Valeur |
| --- | --- |
| Protocole | TCP |
| Adresse d'écoute / port | `192.0.2.10` / `443` |
| Route `ocserv` | protocole TLS, domaine `tz.example.com`, cible `127.0.0.1`, port `443`, PROXY v2 |
| Cible par défaut | `127.0.0.1`, port cible `8443`, PROXY v2 |

Réglez l'adresse d'écoute sur l'adresse publique du serveur afin d'éviter tout conflit avec les backends qui écoutent sur `127.0.0.1:443`. Pour servir des sites avec Gately lui-même, ajoutez un écouteur de [Services Web](/fr/docs/modules/web) sur `127.0.0.1:8443`, avec « Accepter le protocole PROXY » activé et `127.0.0.0/8` dans les sources PROXY de confiance. Arrêtez haproxy avant d'activer la règle, car un seul programme peut écouter sur le port.

## Exemple : remplacer sslh

Ces arguments sslh :

```text
sslh --listen 0.0.0.0:443 --timeout 2 \
     --ssh 127.0.0.1:22 --openvpn 127.0.0.1:1194 --tls 127.0.0.1:8443
```

correspondent à une seule règle Gately :

| Réglage | Valeur |
| --- | --- |
| Protocole | TCP |
| Adresse d'écoute / port | vide / `443` |
| Délai de détection | `2` |
| Route `ssh` | protocole SSH, cible `127.0.0.1`, port `22` |
| Route `vpn` | protocole OpenVPN, cible `127.0.0.1`, port `1194` |
| Route `web` | protocole TLS, sans domaine, cible `127.0.0.1`, port `8443` |
| Route `vnc` (facultative) | protocole Délai d'attente, cible `127.0.0.1`, port `5900` |

Par défaut, sslh envoie les connexions vers SSH à l'expiration du délai. Gately reconnaît SSH immédiatement grâce à sa chaîne de version `SSH-`, si bien que le délai peut mener à un service où le serveur parle en premier, comme VNC. Vous pouvez ajouter d'autres routes RDP, SOCKS et HTTP, ou répartir le TLS entre plusieurs backends selon le domaine.

## Mise à niveau depuis la redirection de ports

Le routage par protocole était auparavant le mode « Routage SNI » des règles de redirection de ports. Au premier démarrage après la mise à niveau, les règles de routage SNI de la redirection de ports passent automatiquement dans le module Routage par protocole : chaque règle garde sa clé, son nom, son état d'activation, son groupe et toutes ses routes, et les anciennes routes deviennent des routes TLS. Si le module de redirection de ports était désactivé, le Routage par protocole l'est aussi, et les jetons d'API ayant accès à la redirection de ports reçoivent le même niveau d'accès au Routage par protocole.

Après la migration, gérez ces règles via `/api/protoroute/rules` : le champ des routes passe de `sniRoutes` à `routes` et le champ `mode` n'est plus utilisé ; les règles de routage soumises à `/api/portforward/rules` sont refusées. Les métriques deviennent `gately_protoroute_*` ; voir [Métriques de supervision](/fr/docs/reference/monitoring).

## Ouvrir le port dans le pare-feu {#auto-firewall}

Avec « Ouvrir le port dans le pare-feu » activé, Gately autorise dans le pare-feu de cet hôte uniquement les ports sur lesquels la règle écoute : les ports consécutifs sont regroupés en une seule règle, IPv4 / IPv6 suivent les protocoles de la règle et, avec une adresse d’écoute renseignée, seules les connexions vers cette adresse sont autorisées. La règle est retirée quand elle est désactivée ou supprimée. Voir [Services Web · Ouvrir le port dans le pare-feu](/fr/docs/modules/web#auto-firewall) pour les cas d’usage, les pare-feu pris en charge et l’autorisation à la main.

## Conseil de déploiement

Le routage par protocole écoute directement sur les ports de l'hôte. Pour un déploiement Docker, `--network host` est recommandé.

## Groupes et tri

Les règles peuvent avoir un « Groupe ». Les groupes, le tri et la recherche fonctionnent comme dans la [Redirection de ports](/fr/docs/modules/portforward#groupes-et-tri) ; la zone de recherche filtre par nom, groupe, port d'écoute, cible, domaine ou protocole de route.
