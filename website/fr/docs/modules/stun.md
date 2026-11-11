# Traversée STUN

Lorsque votre FAI ne fournit pas de port public fixe, la traversée STUN utilise le percement de NAT pour obtenir une adresse et un port publics mappés, afin que les services de votre réseau local soient accessibles depuis internet.

## Fonctions

- Traversée TCP / UDP avec maintien de session ;
- UPnP / NAT-PMP crée automatiquement les mappages de ports sur le routeur ;
- La redirection intégrée envoie directement le port public obtenu par la traversée vers une cible du réseau local ;
- Des Webhooks et des scripts se déclenchent quand l'adresse ou le port public change, ce qui permet de mettre à jour le DNS, de notifier des clients, etc. ; le Webhook peut être envoyé via un proxy (le proxy global de mise à jour ou un proxy personnalisé).
- Contrôle d'accès par IP source (liste d'autorisation / de blocage) pour le relais intégré.

## Utilisation avec d'autres modules

Le port public obtenu via STUN change généralement. Associez-le à :

- [DNS dynamique](/fr/docs/modules/ddns) ou à un Webhook / script pour synchroniser la nouvelle adresse avec vos enregistrements DNS ; les valeurs d'enregistrement du DNS dynamique peuvent aussi utiliser `{STUN_nom de règle_PORT}` et des variables similaires, synchronisées dès que l'adresse de la règle change, voir [Variables de la valeur d'enregistrement](/fr/docs/modules/ddns#record-vars) ;
- [Services Web](/fr/docs/modules/web) pour fournir un point d'entrée HTTPS.
- Les sites de redirection des Services Web peuvent référencer l'adresse publique actuelle de la règle avec des variables comme `{STUN_NomRègle_PORT}` et suivent automatiquement le nouveau port quand il change ; voir [Variables de redirection](/fr/docs/modules/web#redirect-vars).

## Limites et NAT à plusieurs niveaux

- **NAT de niveau opérateur (CGNAT)** : quand le WAN du routeur reçoit une adresse interne comme `100.64.0.0/10`, la traversée dépend du type de NAT de l'opérateur. Un NAT à cône complet (toute adresse extérieure peut joindre le port mappé) fonctionne ; un NAT à port restreint ou symétrique ne fonctionne pas, utilisez alors [Tailscale, frp ou Cloudflare Tunnel](/fr/docs/modules/tunnels).
- **UPnP derrière plusieurs NAT** : avec UPnP / NAT-PMP, Gately mappe le port du tunnel vers cet hôte sur le routeur. Si l'adresse externe du routeur diffère de l'adresse publique obtenue par STUN, un autre NAT se trouve à l'extérieur et le port public lui appartient : Gately ne garde que le mappage du port du tunnel et n'ajoute plus de mappage pour le port public.
- **Fixer le port local et l'autoriser dans le pare-feu** : un port local vide est choisi au hasard à chaque démarrage. Si l'hôte utilise un pare-feu (par exemple le pare-feu système de fnOS), fixez le port local de la règle et autorisez-le dans le pare-feu (vous pouvez réserver une plage de ports aux règles STUN) ; sinon le pare-feu rejette les connexions entrantes et l'adresse de traversée reste injoignable.
- **TCP et NAT à cône du routeur** : sur certains routeurs, « NAT1 / cône complet » ne s'applique qu'à l'UDP et le port source TCP est réécrit en sortie. Les règles TCP nécessitent UPnP, ou un mappage du port du tunnel vers cet hôte sur le routeur.

## Ouvrir le port dans le pare-feu {#auto-firewall}

Lorsque « Ouvrir le port dans le pare-feu » est activé sur une règle, Gately autorise le port du canal dans le pare-feu de cet hôte (ainsi que le port interne UPnP / NAT-PMP s'il est renseigné) et retire l'autorisation quand la règle s'arrête ou est supprimée. À utiliser quand Gately tourne sur un routeur comme OpenWrt ou sur un hôte Linux avec pare-feu actif. Linux uniquement.

- nftables : une règle d'autorisation est insérée en tête de chaque chaîne de filtrage sur le hook input (par exemple OpenWrt fw4 et firewalld) ;
- iptables : une règle d'autorisation est insérée en tête de la chaîne INPUT de la table filter (par exemple OpenWrt fw3 et ufw) ;
- les règles portent le commentaire `gately-open:` ; un rechargement du pare-feu les efface, et Gately vérifie toutes les 30 secondes et les rajoute ;
- les noyaux sans nf_tables (comme Synology DSM) utilisent iptables (legacy) : les règles sont placées dans une chaîne filter propre à Gately (commençant par `GTLY_OPEN_`), vers laquelle saute la première règle d'INPUT ; si le pare-feu du système place ses règles devant, le saut est remis en tête dans les 30 secondes. L'image Docker inclut iptables (legacy) ;
- en Docker, il faut le réseau host et `cap_add: NET_ADMIN` ; la carte de la règle affiche le résultat ou la raison de l'échec ;
- les pare-feu qui ne passent pas par nftables / iptables, comme le pare-feu système de fnOS, demandent toujours d'ouvrir le port à la main dans leurs réglages.

## Contrôle d'accès par IP source

Avec le relais intégré, chaque règle peut utiliser une liste d'autorisation ou de blocage qui décide d'après l'IP source si le trafic est relayé vers la cible. Les listes et les listes d'IP abonnées fonctionnent comme pour la [Redirection de ports](/fr/docs/modules/portforward#controle-d-acces-par-ip-source), et les bannissements de la Prévention des intrusions passent aussi en premier. La liste ne s'applique qu'au relais ; les requêtes STUN et la vérification du canal ne sont pas affectées. Modifier la liste ne refait pas la traversée, le port public reste le même. UDP est vérifié par paquet. Sans le relais intégré, les connexions externes ne passent pas par Gately et la liste est sans effet. Vous pouvez aussi choisir « Exiger un knock » : seules les sources ayant frappé avec succès via fwknop sont relayées, voir [Port Knocking](/fr/docs/modules/knock#gately-entries).

## Conseil de déploiement

STUN doit écouter directement sur les ports de l'hôte. Pour un déploiement Docker, `--network host` est recommandé.
