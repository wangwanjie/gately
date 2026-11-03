# Conception technique et feuille de route de Gately

## Objectif

Construire un outil de passerelle réseau open source, auditable et porté par la communauté : il couvre le DDNS, la traversée NAT STUN, la redirection de ports, le reverse proxy et la gestion des certificats, les fonctions les plus utilisées sur les réseaux domestiques, les NAS et les serveurs, et prend en charge l'import des configurations Lucky existantes.

Principes de conception :

1. **Transparence** : aucune connexion aux serveurs du projet par défaut ; toutes les adresses externes sont visibles, modifiables et désactivables.
2. **Migration** : import des configurations Lucky `.lkcf`.
3. **Configuration auditable** : la configuration est du JSON en clair (permissions de fichier 0600), que l'on peut placer directement dans git pour le diff et le retour en arrière.
4. **Déploiement en un seul fichier** : un binaire statique avec le frontend intégré, compatible Docker, systemd et OpenWrt ; les modules optionnels et les langues de l'interface peuvent être retirés à la compilation pour produire une version personnalisée plus légère.

## Pile technique

| Couche | Choix | Raison |
| --- | --- | --- |
| Backend | Go 1.26+ | Réutilisation directe de bibliothèques éprouvées comme lego, pion/stun, frp, cloudflared, coraza et rclone ; compilation statique et compilation croisée pour les routeurs et NAS arm/arm64/mips/riscv |
| HTTP | Bibliothèque standard `net/http` (syntaxe de routage de Go 1.22) | Dépendances minimales, audit facilité |
| Stockage | Un fichier JSON en clair par module, écriture atomique | Import, sauvegarde et git diff faciles |
| Authentification | Mot de passe bcrypt + jeton de session signé HMAC, limitation des échecs de connexion, TOTP prévu ensuite | Aucune dépendance externe |
| Journaux | Tampon circulaire en mémoire par module + fichiers JSON Lines écrits par lots (rotation par taille, restaurés après redémarrage) + API de lecture incrémentale | API d'interrogation des journaux simple, rafraîchissement incrémental côté frontend |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, empaqueté avec `go:embed` | Composants nombreux, développement rapide |
| Publication | Binaires multi-architectures avec GoReleaser + images Docker multi-architectures | — |

## Arborescence

```
cmd/gately/            Point d'entrée du programme
internal/app/          Assemblage : chargement de la configuration, démarrage des modules, service HTTP
internal/config/       Stockage modulaire de la configuration JSON
internal/auth/         Authentification et sessions de l'administrateur
internal/logbuf/       Tampon circulaire des journaux de module
internal/httpx/        Utilitaires et middlewares HTTP
internal/module/       Interface et registre des modules
internal/modules/*/    Modules fonctionnels
internal/modules/registry/  Enregistrement des modules : un mod_<module>.go par module optionnel (build tag no_<module>), capacités inter-modules fournies par injection
tools/custombuild/     Version personnalisée : convertit modules.json en build tags, langues et arguments de build Docker, puis empaquette
modules.json           Modules optionnels, dépendances, taille estimée, niveaux de langue, cibles de build et préréglages (lu par build.gately.cn)
internal/luckyimport/  Déchiffrement des .lkcf de Lucky et correspondance des champs
web/                   Projet frontend, le résultat de la compilation est intégré par web/embed.go
```

## Jalons

Classés selon l'usage réel des utilisateurs (STUN 18, redirection de ports 25, reverse proxy 27, certificats 5, DDNS 75 enregistrements).

| Phase | Contenu | Validation |
| --- | --- | --- |
| ✅ **M0 Squelette** | Stockage de la configuration, connexion administrateur, framework de modules, tampon de journaux, coque de l'interface d'administration, Dockerfile | Après connexion, l'aperçu et le menu des modules s'affichent |
| ✅ **M1 Redirection de ports** | TCP/UDP, v4/v6, ports multiples et plages de ports, cibles multiples, limite de connexions, démarrage/arrêt et journaux ; import des règles de redirection de ports Lucky | Tests unitaires de la redirection TCP/UDP ; import de 25/25 règles de l'échantillon réel |
| ✅ **M2 DDNS** | Récupération de l'IP (URL/interface réseau/commande), IPv4/IPv6, Alibaba Cloud/Cloudflare/DNSPod (Tencent Cloud)/Huawei Cloud/rappel personnalisé, Webhook ; import | 11 fournisseurs (signature Alibaba Cloud vérifiée avec les exemples officiels) ; import de 2/2 tâches et 75 enregistrements de l'échantillon réel |
| ✅ **M3 Traversée STUN** | Obtention du mappage public via STUN, keep-alive TCP, UPnP/NAT-PMP, redirection intégrée, Webhook et variables de script ; import | Test de bout en bout local + traversée réelle sur un réseau domestique ; import de 18/18 règles de l'échantillon réel |
| ✅ **M4 Certificats** | ACME (lego, DNS-01 réutilise les Provider de M2), renouvellement automatique, export vers un chemin, script lors d'un changement | Test de bout en bout avec Pebble (wildcard + renouvellement) ; import de 5/5 certificats de l'échantillon réel sans réémission |
| ✅ **M5 Service web** | Reverse proxy, redirection, fichiers statiques, routage par domaine, TLS automatique, BasicAuth, filtrage IP ; import | Couvre le routage, les 4 types de site, l'authentification, WebSocket, HTTPS, l'envoi et la réception du protocole PROXY ; import des 27 sites de l'échantillon réel |
| ✅ M6 Exploitation | Wake-on-LAN, terminal web (SSH/shell local), tâches planifiées, authentification à deux facteurs de l'administrateur | Import de WOL 3/3 et de connexions de terminal 14/14 ; TOTP valide les vecteurs de test de la RFC 6238 |
| ✅ M7 Écosystème (partiel) | Coraza WAF, connexion via GitHub/Google/QQ/OIDC (authentification des sites et connexion à l'administration, avec migration de la table d'identités Lucky), gestion Docker, WebDAV, service FTP, gestion du stockage (montages locaux/WebDAV distants et gestion de fichiers en ligne) | WAF testé avec de vraies charges d'attaque ; Docker vérifié en lecture seule dans un environnement réel |
| ✅ M7.5 Tunnels réseau | Tailscale, NetBird, frp (plusieurs instances frpc et frps) et cloudflared, intégrés en supervisant les processus des binaires officiels | Tests unitaires pour la génération de configuration, la supervision des processus et la collecte des journaux ; les programmes amont ne sont pas inclus |
| ✅ M8 Confiance et exploitation | Journal d'audit des opérations, historique de configuration et restauration, répertoire de configuration déclarative ; connexion par passkey et comptes en lecture seule ; ACME HTTP-01 ; HTTP/3 ; métriques Prometheus globales et tableau de bord Grafana ; autodiagnostic réseau et assistant de premier démarrage ; le DDNS ajoute Route 53, OVH, Gandi, NS1, Bunny, Namecheap et DynDNS2 ; les notifications ajoutent ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus et des modèles de règles ; signature cosign, attestation de provenance de build et SBOM des artefacts publiés ; modèles de service systemd et OpenWrt | WebAuthn testé de bout en bout avec un authentificateur logiciel ; signature Route 53 vérifiée avec les vecteurs de test SigV4 officiels d'AWS ; HTTP/3 testé avec un client quic-go |
| ✅ M8.5 Installation et mise à niveau | Application native fnOS (.fpk, intégrée à la passerelle unifiée et à la session fnOS) ; mise à niveau en ligne des binaires Linux ; paquets d'installation OpenWrt (.ipk pour 24.10 et versions antérieures, .apk à partir de 25.12) | fpk testé sur une machine virtuelle fnOS ; paquets OpenWrt testés dans les conteneurs rootfs officiels 24.10 / 25.12 et sur iStoreOS, avec des tests de fumée en CI couvrant l'installation et la désinstallation |
| ✅ M8.6 Migration et exploitation | Import de configuration nginx (`nginx -T`, avec développement des include, upstream et `auth_request` d'Authelia) ; déploiement automatique des certificats (envoi par SSH avec exécution d'une commande, redémarrage de conteneurs Docker) ; limite d'inspection du corps de requête du WAF, inspection du seul début au-delà de la limite et exemption par chemin ; listes d'IP abonnées (actualisation périodique, référencées par les sites et les règles de chemin) ; l'autodiagnostic vérifie la couverture des sites HTTPS par un certificat et les conflits de proxys frp ; mise à niveau en ligne compatible avec les numéros de version à quatre composants | Import nginx validé sur une vraie configuration de production (77 sites, Authelia, upstream) ; envoi SSH testé de bout en bout contre un serveur SFTP de test ; WAF validé avec un corps de requête de 2 Mo pour le rejet, l'inspection du seul début et les chemins exemptés |
| ✅ M8.7 Prévention d'intrusion | Règles de journaux à la fail2ban (fichiers journaux, journal systemd, sortie de commande, journaux de conteneurs Docker, échecs de connexion à la console, blocages du WAF) avec des modèles pour les services courants ; ports pièges et détection de scan ; une liste de bannissement (intègre les bannissements du service web après échec d'authentification, enregistrée, durées progressives, bannissements manuels de réseaux et permanents, liste blanche) ; la redirection de ports, le service web, STUN, WebDAV, FTP et la console refusent les adresses bannies ; bannissements synchronisés avec nftables (table dédiée `inet gately`) et rejetés dans le noyau | Ports pièges, détection de scan (IPv4/IPv6), les quatre sources de journaux, la persistance et l'auto-réparation des règles vérifiées dans des espaces de noms réseau isolés sur fnOS (nftables 1.0.6) et iStoreOS 24.10 (nftables 1.1.1) ; détection des autorisations et repli sur les ports pièges en espace utilisateur vérifiés dans des conteneurs Docker |
| M9 À définir | Plusieurs administrateurs et rôles fins | Planification selon les besoins de la communauté |

## Choix de conception

- La mise à niveau en ligne est proposée uniquement pour les déploiements par binaire Linux et se déclenche seulement à la main par un administrateur : après le téléchargement, le SHA-256 est comparé à checksums.txt et le nouveau programme est lancé à l'essai ; l'ancienne version est conservée lors du remplacement. Docker, fnOS et les paquets OpenWrt restent mis à niveau par l'image, le centre d'applications et le gestionnaire de paquets. Les versions personnalisées ne proposent pas la mise à niveau en ligne (elle réinstallerait l'édition complète) : régénérez-les sur build.gately.cn avec les mêmes options, puis téléversez le paquet de mise à niveau.
- Aucune configuration n'est envoyée à distance et aucune information sur l'appareil n'est remontée.
- La configuration est stockée en clair ; les champs sensibles sont masqués dans les réponses de l'API.
