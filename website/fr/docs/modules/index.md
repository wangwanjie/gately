# Vue d'ensemble des modules

Chaque fonction de Gately est un module indépendant. Chacun peut être activé, désactivé et configuré séparément, et enregistre sa configuration dans un simple fichier JSON en texte brut.

Vous pouvez désactiver un module entier dans « Paramètres → Modules ». Un module désactivé charge toujours sa configuration, mais ne démarre aucun service, n'expose aucune API d'administration (ses points d'accès renvoient 403) et est masqué dans l'interface ; sa configuration est conservée et revient dès que vous le réactivez. Les modules dont d'autres dépendent ne peuvent pas être désactivés, et les changements prennent effet après un redémarrage.

| Module | Fonctions | Documentation |
| --- | --- | --- |
| DNS dynamique | 11 fournisseurs DNS, IP via URL / interface réseau / commande, IPv4 / IPv6, Webhook | [Consulter](/fr/docs/modules/ddns) |
| Traversée STUN | TCP / UDP, maintien de session, UPnP / NAT-PMP, redirection intégrée, Webhook et scripts déclenchés au changement | [Consulter](/fr/docs/modules/stun) |
| Redirection de ports | TCP / UDP, IPv4 / IPv6, plages de ports, basculement multi-cibles, limites de connexions, statistiques de trafic | [Consulter](/fr/docs/modules/portforward) |
| Routage par protocole | Plusieurs services partagent un port : TLS par SNI et ALPN, HTTP par Host, Minecraft par adresse du serveur, plus SSH, RDP, SOCKS, OpenVPN, RTSP, SIP, XMPP, MQTT, AMQP, PostgreSQL, Redis, SMB, Git, IRC, ADB, BitTorrent et une route Délai d'attente pour VNC et similaires | [Consulter](/fr/docs/modules/protoroute) |
| Certificats SSL | Émission et renouvellement automatiques ACME DNS-01, import, mappage vers un répertoire avec scripts | [Consulter](/fr/docs/modules/ssl) |
| Services Web | Proxy inverse, redirections, fichiers statiques, HTTPS, authentification, contrôle d'accès, répartition de charge et vérifications d'état, cache et limitation de débit, statistiques d'accès | [Consulter](/fr/docs/modules/web) |
| Authentification unique OIDC | Propose les utilisateurs partagés à Gitea, OpenList, Grafana et d'autres applications via OpenID Connect ; remplace l'OIDC d'Authelia | [Consulter](/fr/docs/modules/oidc) |
| Pare-feu WAF | Coraza + OWASP CRS v4 | [Consulter](/fr/docs/modules/waf) |
| Prévention d'intrusion | Règles de journaux (à la fail2ban), ports pièges et détection de scan, bannissements synchronisés dans nftables | [Consulter](/fr/docs/modules/guard) |
| Tailscale / frp / cloudflared | Réseau et traversée NAT avec des binaires officiels gérés | [Consulter](/fr/docs/modules/tunnels) |
| Docker | Création et édition de conteneurs, journaux et terminal, Compose, mises à jour et montées de version d'images, réseaux et volumes | [Consulter](/fr/docs/modules/docker) |
| Terminal Web | SSH, Telnet, shell local, gestionnaire de fichiers SFTP, sessions persistantes | [Consulter](/fr/docs/modules/terminal) |
| Diagnostic réseau | Ping, traceroute, test de port, résolution DNS, test de débit, scan du réseau local | [Consulter](/fr/docs/modules/nettools) |
| Centre de notifications | Envoi d'événements multicanal et règles | [Consulter](/fr/docs/modules/notify) |
| Sauvegarde de la configuration | Sauvegardes chiffrées, politique de rétention, sauvegarde planifiée et restauration | [Consulter](/fr/docs/modules/backup) |
| Outils fichiers et exploitation | WebDAV, FTP, SMB, File Browser, montages de stockage, Wake-on-LAN, tâches planifiées | [Consulter](/fr/docs/modules/tools) |
