---
title: Journal des modifications
description: Annonces et notes de version de chaque version de Gately.
---

# Journal des modifications

Les annonces des versions majeures sont listées ci-dessous. Consultez [GitHub Releases](https://github.com/wangwanjie/gately/releases) pour la liste complète des changements et les téléchargements de chaque version.

## [v0.6.5.3](/fr/news/v0.6.5.3)

10 novembre 2026 · Correctif de sécurité (cache de réponses) ; panneaux de navigation ; audit des visiteurs ; relecture de la topologie réseau ; règles de cache par chemin ; le terminal Web vérifie les empreintes d’hôte selon le type de clé ; hauteur réglable du panneau des journaux.

## [v0.6.5.2](/fr/news/v0.6.5.2)

8 novembre 2026 · Prévention d’intrusion et ouverture automatique du pare-feu sur Synology DSM et les autres noyaux sans nf_tables ; joker dans le premier libellé des sites ; l’authentification déléguée suit les redirections 401 ; corrige un arrêt sur les requêtes HTTP/2.

## [v0.6.5.1](/fr/news/v0.6.5.1)

7 novembre 2026 · Démarrage automatique avec gately service ; longues listes paginées ; ajout en lot d’enregistrements DDNS ; crash.log ; binaires macOS signés et notariés.

## [v0.6.5](/fr/news/v0.6.5)

6 novembre 2026 · Délais du backend et des visiteurs avec nouvelle tentative automatique pour les sites en reverse proxy ; transmission de l’identité aux backends ; ouverture automatique du pare-feu pour les écouteurs et les règles de redirection ; archives quotidiennes et export des journaux d’accès ; mode strict IPv6 de la Prévention d'intrusion ; ressources des processus gérés.

## [v0.6.4.1](/fr/news/v0.6.4.1)

4 novembre 2026 · Plusieurs instances Tailscale et NetBird ; les notifications indiquent le nom du serveur ; les routes du routage par protocole peuvent être désactivées ; corrige la synchronisation et la validation des certificats DNSPod et l'IP publique depuis l'arrêt d'ipw.cn.

## [v0.6.4](/fr/news/v0.6.4)

3 novembre 2026 · Builds personnalisés : sur build.gately.cn, cochez seulement les modules et les langues d’interface utiles pour générer en ligne des paquets et des images Docker plus légers ; la console d’administration peut masquer les signes qui trahissent un accès non autorisé ; les sites Web peuvent choisir parmi 10 styles de page d’erreur ou une page personnalisée, et remplacer les pages d’erreur du backend.

## [v0.6.3.5](/fr/news/v0.6.3.5)

3 novembre 2026 · Nouveau réseau NetBird ; vérification automatique des nouvelles versions et des avis de sécurité, avec alerte ; chaque élément de liste peut être copié ; les sites Web peuvent changer d’écouteur ; les en-têtes de page et barres d’outils des listes restent visibles au défilement ; notifications, Webhooks et listes d’IP peuvent passer par un proxy ; les tâches Docker en arrière-plan peuvent être interrompues ; contient des correctifs de sécurité, mise à niveau recommandée.

## [v0.6.3](/fr/news/v0.6.3)

2 novembre 2026 · Nouvelle surveillance des performances (tendances, temps jusqu’au premier octet et alertes par seuil) ; le WAF peut utiliser un moteur de détection SafeLine auto-hébergé ; les listes et les sites des Services Web gèrent les opérations par lot ; les règles STUN peuvent ouvrir automatiquement les ports du pare-feu de cette machine ; correction des échecs d’émission DNS-01 dus aux CNAME génériques ; les journaux des Services Web peuvent masquer les sources bannies et les requêtes de la console elle-même.

## [v0.6.2.3](/fr/news/v0.6.2.3)

2 novembre 2026 · La topologie réseau devient une page à part avec sept dispositions, zoom et mode plein écran de fenêtre, et des catégories de masquage au choix ; la page d’accueil et les README affichent des captures dans chaque langue, avec une vidéo de visite de 2 minutes.

## [v0.6.2.2](/fr/news/v0.6.2.2)

1er novembre 2026 · Les Services Web gagnent une vue topologie, peuvent prendre l’IP réelle du visiteur auprès de proxys de confiance et affichent les connexions en direct de chaque site, avec fermeture et bannissement ; Redirection de ports, Routage par protocole, STUN et frps disposent de listes d’autorisation et de blocage par IP source ; le DNS dynamique écrit les remarques chez le fournisseur et peut y supprimer les enregistrements ; Gately peut mettre à niveau son propre conteneur depuis la page Docker ; le Terminal Web permet la sélection par appui long sur téléphone.

## [v0.6.2](/fr/news/v0.6.2)

31 octobre 2026 · Le nouveau module Routage par protocole permet à HTTPS, SSH, RDP, OpenVPN et d’autres de partager un même port ; toutes les longues listes peuvent être groupées et triées ; le DNS dynamique peut utiliser les adresses publiques STUN ; STUN fonctionne derrière plusieurs NAT ; plusieurs renforcements de sécurité.

## [v0.6.1.1](/fr/news/v0.6.1.1)

30 octobre 2026 · La prévention d’intrusion bannit le sondage de chemins web ; une nouvelle page Utilisateurs d’auth gère les utilisateurs partagés et OIDC ; les utilisateurs partagés peuvent changer eux-mêmes leur mot de passe ; les sites peuvent fermer la connexion en cas de refus ; les routes des tunnels Cloudflare à jeton se modifient dans Gately ; les journaux reçoivent un filtre Exclure.

## [v0.6.1](/fr/news/v0.6.1)

30 octobre 2026 · Correctif de sécurité : les comptes et les jetons d’API en lecture seule ne peuvent plus lire en clair les variables d’environnement ni les commandes de démarrage des conteneurs ; les certificats peuvent être émis en IPv4 uniquement ou en IPv6 uniquement ; la page Docker ne met plus à niveau ni ne recrée le conteneur de Gately lui-même ; les notifications et les journaux suivent la langue de l’interface d’administration.

## [v0.6.0.4](/fr/news/v0.6.0.4)

29 octobre 2026 · Ajoute une application LuCI pour OpenWrt afin de voir l’état, de démarrer et d’arrêter le service et de définir le port d’administration depuis LuCI ; les sites de redirection acceptent des variables comme `{path}`, `{1}` et `{STUN_NomRègle_PORT}` ; les utilisateurs partagés et les clients OIDC des paramètres globaux s’affichent en résumé et se modifient dans une boîte de dialogue.

## [v0.6.0.3](/fr/news/v0.6.0.3)

28 octobre 2026 · Corrige un Gately local recevant du HTTP en clair depuis une source PROXY de confiance qui ne reconnaissait pas l’usage de HTTPS par le visiteur, ce qui faisait échouer la connexion OIDC avec un émetteur https, laissait des cookies sans Secure et créait des boucles de redirection.

## [v0.6.0.2](/fr/news/v0.6.0.2)

28 octobre 2026 · Corrige le terminal web bloqué sur « Connexion » lorsqu’il est ouvert depuis le bureau fnOS, File Browser qui ne démarrait pas avec un répertoire de configuration relatif, et les réponses de l’API dans une langue autre que le chinois simplifié qui traduisaient en partie le texte chinois saisi par les utilisateurs.

## [v0.6.0.1](/fr/news/v0.6.0.1)

27 octobre 2026 · Le DNS dynamique et la redirection de ports gagnent les groupes, le tri et la recherche ; les pages Service web, DNS dynamique et redirection de ports permettent de grouper en lot, de faire glisser plusieurs éléments cochés ensemble et de renommer les groupes.

## [v0.6.0](/fr/news/v0.6.0)

27 octobre 2026 · Plusieurs instances frpc et frps ; le Service web envoie les utilisateurs connectés vers leurs propres backends, regroupe, trie et recherche les sites, et peut transmettre les cookies de connexion Gately ; les certificats détectent la zone DNS des sous-domaines délégués ; les mots de passe et clés enregistrés peuvent être affichés à la demande.

## [v0.5.9.1](/fr/news/v0.5.9.1)

26 octobre 2026 · Les écouteurs HTTPS du Service web peuvent accepter HTTP sur le même port (redirection ou service) ; les instances File Browser acceptent des URL d'accès ; l'enregistrement d'une instance File Browser ou d'un tunnel Cloudflare modifié n'échoue plus.

## [v0.5.9](/fr/news/v0.5.9)

26 octobre 2026 · File Browser géré par Gately ; certificats d'adresse IP et de courte durée ; installation, mise à jour, retour arrière et désinstallation en ligne de frp, cloudflared, Tailscale et File Browser ; les utilisateurs partagés des services de fichiers passent sur une page dédiée.

## [v0.5.8.1](/fr/news/v0.5.8.1)

26 octobre 2026 · WebDAV, FTP et SMB partagent une liste d'utilisateurs partagés ; le Service web gagne le type de site Fermer la connexion et journalise le Referer et le User-Agent ; frps peut écouter sur des adresses choisies ; les boutons de copie fonctionnent en HTTP.

## [v0.5.8](/fr/news/v0.5.8)

25 octobre 2026 · Un fournisseur d'identité OIDC intégré permet aux utilisateurs partagés de se connecter à Gitea, Grafana et d'autres applications ; nouveau Service SMB ; tous les journaux sont filtrables ; les groupes de règles du WAF se réordonnent ; les en-têtes des sites se choisissent parmi les entrées courantes.

## [v0.5.7](/fr/news/v0.5.7)

24 octobre 2026 · Les Montages de stockage se connectent aux partages SMB ; les thèmes deviennent 15 palettes en clair et en sombre, avec accent et coins personnalisables ; l'interface d'administration est compressée et le binaire est environ 11 % plus léger.

## [v0.5.6.4](/fr/news/v0.5.6.4)

23 octobre 2026 · Le déploiement automatique des certificats copie vers plusieurs répertoires locaux, chacun avec sa commande, et accepte des noms de fichiers personnalisés pour le certificat et la clé.

## [v0.5.6.3](/fr/news/v0.5.6.3)

23 octobre 2026 · Corrige la vue d'ensemble qui masquait l'interface br-lan sous OpenWrt, ainsi que le fuseau horaire impossible à régler, l'heure restant en UTC.

## [v0.5.6.2](/fr/news/v0.5.6.2)

23 octobre 2026 · Nouvelles pages de connexion et d'erreur du service web, avec la vérification en deux étapes dans une fenêtre. Corrige l'échec de connexion du module Docker sous Docker 29, l'affichage furtif de la vue d'ensemble avant la page de connexion, ainsi que la synchronisation des identifiants Cloudflare et des enregistrements DDNS renommés importés de Lucky.

## [v0.5.6.1](/fr/news/v0.5.6.1)

22 octobre 2026 · Corrige le plantage d'un nft ancien sur l'hôte causé par les règles de pare-feu écrites par l'image Docker.

## [v0.5.6](/fr/news/v0.5.6)

22 octobre 2026 · Ajoute la prévention d'intrusion : règles de journaux à la fail2ban, ports pièges et détection de scan, avec des bannissements synchronisés dans nftables.

## [v0.5.5.2](/fr/news/v0.5.5.2)

22 octobre 2026 · Corrige la lecture possible par d'autres utilisateurs locaux des fichiers sensibles du répertoire de configuration sur fnOS et les systèmes de fichiers similaires.

## [v0.5.5.1](/fr/news/v0.5.5.1)

21 octobre 2026 · Les journaux d'exécution sont écrits sur disque et survivent aux redémarrages et mises à jour.

## [v0.5.5](/fr/news/v0.5.5)

21 octobre 2026 · Import de configurations nginx, déploiement automatique des certificats vers d'autres hôtes et conteneurs, gros envois et WebDAV derrière le WAF, listes d'IP abonnées ; l'auto-diagnostic vérifie désormais la couverture des certificats et les conflits frp.

## [v0.5.4](/fr/news/v0.5.4)

21 octobre 2026 · Mise à niveau en ligne pour les installations binaires Linux : plus besoin de recréer les conteneurs LXC de PVE ; nouveaux paquets OpenWrt (.ipk / .apk).

## [v0.5.3](/fr/news/v0.5.3)

21 octobre 2026 · Application native fnOS qui s'intègre à la passerelle unifiée fnOS et connecte les administrateurs fnOS avec leur session fnOS ; les modules fonctionnels peuvent être activés ou désactivés.

## [v0.5.2](/fr/news/v0.5.2)

20 octobre 2026 · Le routage SNI permet à plusieurs services de partager le port 443, les Services Web peuvent remplacer nginx, les utilisateurs partagés gagnent TOTP et l'authentification unique, et une comparaison des performances avec haproxy + nginx.

## [v0.5.0](/fr/news/v0.5.0)

19 octobre 2026 · Journal d'audit, historique de configuration et restauration, passkeys, comptes en lecture seule, ACME HTTP-01, HTTP/3, protocole PROXY, métriques Prometheus globales, auto-diagnostic réseau et versions signées.
