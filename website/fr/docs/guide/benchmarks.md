# Comparatif de performances

Cette page compare Gately aux outils courants qu'il peut remplacer, sur la même machine et avec les mêmes limites de CPU : reverse proxy, authentification web, WAF, prévention d'intrusion, Port Knocking, DNS dynamique, ainsi que le comportement sur un routeur OpenWrt. Tous les chiffres proviennent de mesures effectuées en novembre 2026. Les scripts de test, les configurations et les résultats bruts se trouvent dans le répertoire [tools/benchmark](https://github.com/wangwanjie/gately/tree/main/tools/benchmark) du dépôt, et vous pouvez les reproduire.

<script setup>
const H = 'Plus c’est grand, mieux c’est'
const L = 'Plus c’est petit, mieux c’est'
</script>

## Résumé

- **Reverse proxy** : nginx et HAProxy, écrits en C, ont le débit le plus élevé : environ 3.5 fois celui de Gately en HTTP et environ 2.6 fois en HTTPS. Parmi les passerelles écrites en Go, Gately fait partie du premier groupe : son débit HTTPS est supérieur de 43 % à celui de Caddy, égal à 2.3 fois celui de Lucky et inférieur de 9 % à celui de Traefik ; son taux de nouvelles connexions TLS est le plus élevé des 7 programmes.
- **Pas de ralentissement avec de nombreux sites** : avec 1000 sites, le débit de Gately ne baisse que de 6 %, ce qui dépasse HAProxy, Traefik et Caddy, derrière nginx seulement.
- **Authentification web** : pour les requêtes d'utilisateurs connectés, Gately atteint 1.9 fois nginx + Authelia et 3.7 fois Caddy + Authelia, avec un tiers de leur mémoire.
- **WAF** : avec le même OWASP CRS 4.25, Gately traite les requêtes ordinaires environ 70 % plus vite que nginx + ModSecurity, avec des résultats de blocage identiques ; pour l'inspection du corps des requêtes POST, ModSecurity est plus rapide et utilise beaucoup moins de mémoire.
- **Prévention d'intrusion** : avec 10,000 IP d'attaque, Gately termine les bannissements en 12 secondes, contre 51 secondes pour fail2ban ; à volume de journaux égal, le CPU représente moins de la moitié de celui de fail2ban. En lecture de fichier de journal, la latence d'un bannissement individuel est supérieure d'environ 0.2 seconde à celle de fail2ban.
- **Port Knocking** : la latence de frappe de Gately et de fwknop reste inférieure à 10 ms, et une fois l'accès ouvert le trafic ne passe plus par le programme de frappe ; le knock de fnOS transfère le trafic par sa passerelle sur un Linux ordinaire, et son débit en connexions courtes ne représente que 3 % de celui d'une connexion directe.
- **Consommation de ressources** : pour une seule fonction, Gately utilise plus de mémoire que de petits outils comme fail2ban, fwknop ou ddns-go ; lorsqu'un seul Gately assure à la fois reverse proxy, WAF, authentification, DDNS, prévention d'intrusion et Port Knocking, il occupe environ 91 MiB, contre environ 204 MiB au total avec des outils indépendants. Voir [Tout-en-un](#tout-en-un).

## Environnement de test

| Élément | Description |
| --- | --- |
| Hôte | Intel Core i5-13500T, 32 GB de mémoire, fnOS (KVM) |
| VM de test de charge | Debian 13 (noyau 6.12), 8 vCPU liés à 8 threads de 4 cœurs performants de l'hôte, 8 GB de mémoire, Docker 29 |
| Répartition du CPU | Programme testé sur les vCPU 0–1, nginx d'origine sur les vCPU 2–3, outils de charge sur les vCPU 4–7, sans interférence entre eux |
| Routeur | VM ImmortalWrt 25.12.1, 2 vCPU (deux threads du même cœur performant), 1 GB de mémoire |
| Outils de charge | wrk 4.1, oha 1.17, h2load 1.64 (nghttp2) |
| Certificat | Tous les programmes partagent le même certificat ECDSA P-256, TLS 1.3 |
| Origine | nginx renvoie 1 KB de texte ; 100 MB pour le test de gros fichier |

Versions testées : Gately v0.6.8.1, nginx 1.29.8, HAProxy 3.2.26, Caddy 2.11.9, Traefik 3.7.19, Lucky 3.8.1, Nginx Proxy Manager 2.17.0 (OpenResty 1.31.8), Authelia 4.39.28, ModSecurity 3.0.16 (OWASP CRS 4.25.0), fail2ban 1.1.0, fwknop 2.6.11, le knock de fnOS fn-knock 2.5.11, ddns-go 6.17.9. À l'exception de fail2ban et fwknop, qui utilisent les paquets Debian, tout tourne à partir d'images Docker officielles avec le réseau en mode host.

**Méthode de mesure**

- Débit : chaque test dure 20 secondes, sur 3 passes, et l'on retient la médiane. HTTP et HTTPS utilisent wrk avec 128 connexions persistantes ; les nouvelles connexions TLS utilisent oha avec 64 en parallèle et une nouvelle connexion par requête ; HTTP/2 utilise h2load avec 64 connexions et 10 flux simultanés par connexion.
- Mémoire : somme du PSS de tous les processus du conteneur testé (la mémoire partagée entre processus est répartie proportionnellement, donc comptée une seule fois). « Au repos » est la valeur 10 secondes après la fin du démarrage et de la configuration ; « pic » est le maximum pendant le test de charge.
- CPU : temps CPU du cgroup du conteneur testé, où 100 % correspond à un vCPU.
- Démarrage : le programme est redémarré une fois la configuration prête, on mesure le temps jusqu'à la première requête correctement servie, et l'on retient la médiane de 3 essais.
- Les journaux d'accès sont désactivés pour tous les programmes. nginx a le pool de connexions persistantes vers l'origine activé et une limite de requêtes par connexion relevée (réglage courant) ; les autres programmes gardent leurs réglages par défaut.

## Reverse proxy

Le domaine `bench.test` est proxifié vers l'origine, avec HTTP sur le port 8080 et HTTPS sur le port 8443 (Nginx Proxy Manager ne peut utiliser que 80/443).

<BenchBars title="Requêtes HTTPS par seconde (connexions persistantes)" unit="QPS" :better-label="H" :items="[['nginx',85434],['HAProxy',83953],['Traefik',35812],['Nginx Proxy Manager',32816],['Gately',32764],['Caddy',22871],['Lucky',14376]]" />

<BenchBars title="Nouvelles connexions TLS par seconde (un handshake complet par requête)" unit="conn./s" :better-label="H" :items="[['Gately',4636],['Traefik',4482],['Nginx Proxy Manager',4015],['Caddy',3987],['nginx',3796],['Lucky',3621],['HAProxy',3293]]" />

<BenchBars title="Mémoire au repos" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['nginx',15.7],['HAProxy',36.4],['Gately',47.6],['Caddy',48.8],['Traefik',89.0],['Lucky',124.6],['Nginx Proxy Manager',155.7]]" note="Les 47.6 MiB de Gately comprennent tous les modules fonctionnels (DDNS, certificats, WAF, prévention d’intrusion, etc. tournent dans le même processus) ; Lucky compte 2 processus et Nginx Proxy Manager 22 (nginx, le backend Node.js, etc.)." />

| Programme | HTTP | HTTPS | HTTP/2 | TLS nouvelles/s | Fichier de 100 MB | CPU à 5000 QPS | Mémoire au repos | Pic en charge | Par connexion inactive | Temps de redémarrage | Image |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 35,364 | 32,764 | 21,591 | **4,636** | 4,157 MB/s | 67% | 47.6 MiB | 107 MiB | 21.2 KiB | 0.40 s | 229 MB |
| nginx | **123,343** | **85,434** | 78,110 | 3,796 | 2,161 MB/s | 55% | **15.7 MiB** | **25 MiB** | **0.6 KiB** | **0.32 s** | 93 MB |
| HAProxy | 117,923 | 83,953 | **90,633** | 3,293 | 3,912 MB/s | **52%** | 36.4 MiB | 56 MiB | 1.0 KiB | 0.33 s | 58 MB |
| Caddy | 24,459 | 22,871 | 11,746 | 3,987 | 4,116 MB/s | 68% | 48.8 MiB | 132 MiB | 27.8 KiB | 0.45 s | 92 MB |
| Traefik | 39,112 | 35,812 | 19,606 | 4,482 | **4,311 MB/s** | 66% | 89.0 MiB | 253 MiB | 24.4 KiB | 0.53 s | 250 MB |
| Lucky | 15,150 | 14,376 | 10,261 | 3,621 | 3,963 MB/s | 80% | 124.6 MiB | 193 MiB | 26.2 KiB | 3.47 s | 58 MB |
| Nginx Proxy Manager | 45,257 | 32,816 | 34,710 | 4,015 | 2,150 MB/s | 58% | 155.7 MiB | 215 MiB | préalloué | 1.50 s | 1,959 MB |

- HTTP, HTTPS et HTTP/2 sont exprimés en requêtes par seconde. « CPU à 5000 QPS » est le CPU utilisé par le programme sous une charge fixe de 5000 requêtes par seconde ; plus c'est bas, plus c'est économe en énergie.
- Par défaut, nginx ferme une connexion après 1000 requêtes, alors que h2load ne se reconnecte pas ; dans la colonne HTTP/2, cette limite a donc été relevée pour nginx et pour Nginx Proxy Manager (Nginx Proxy Manager ne l'a été par configuration personnalisée que pour le test HTTP/2, les autres tests gardant les valeurs par défaut).
- Dans la colonne gros fichier, nginx et Nginx Proxy Manager sont plus bas parce que nginx écrit par défaut dans un fichier temporaire sur disque les réponses qui dépassent le tampon mémoire.
- « Par connexion inactive » est la mémoire ajoutée en maintenant 10,000 connexions persistantes inactives, divisée par le nombre de connexions. Les programmes Go ont un tampon de lecture et d'écriture et une goroutine par connexion, soit environ 20–28 KiB ; nginx et HAProxy utilisent moins de 1 KiB. Nginx Proxy Manager préalloue une table de connexions selon sa configuration, si bien qu'aucune augmentation n'a pu être mesurée.
- L'image de Gately est plus grosse parce qu'elle contient la CLI Docker et les outils nftables et iptables ; le programme lui-même est un fichier unique de 40 MB, et l'archive à télécharger fait 18 MB.

## 1000 sites

Chaque programme est configuré avec 1000 sites en reverse proxy aux domaines différents, et pendant le test de charge chaque requête choisit un domaine au hasard.

<BenchBars title="Requêtes HTTP par seconde avec 1000 sites" unit="QPS" :better-label="H" :items="[['nginx',122958],['Gately',33211],['Traefik',24229],['HAProxy',24057],['Caddy',9819]]" />

| Programme | Requêtes/s | Par rapport à 1 site | Latence p99 | Redémarrage jusqu'à la disponibilité du dernier site | Mémoire au repos | Pic en charge |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 33,211 | -6% | 29.8 ms | **0.43 s** | 94 MiB | 463 MiB |
| nginx | **122,958** | **0%** | **1.2 ms** | 0.54 s | 57 MiB | **58 MiB** |
| HAProxy | 24,057 | -80% | 30.0 ms | **0.37 s** | **51 MiB** | **58 MiB** |
| Caddy | 9,819 | -60% | 52.7 ms | 0.80 s | 80 MiB | 361 MiB |
| Traefik | 24,229 | -38% | 18.8 ms | 0.74 s | 115 MiB | 157 MiB |

- HAProxy compare les règles une par une avec `use_backend ... if { hdr(host) -i domaine }`, ce qui le ralentit à mesure que les sites se multiplient ; un fichier map permet de l'éviter, mais le test conserve ici l'écriture courante « une règle par site ».
- Gately et Caddy maintiennent chacun un pool de connexions vers l'origine par site ; quand les requêtes se répartissent sur 1000 sites, de nombreuses connexions inactives sont conservées en même temps, ce qui élève le pic de mémoire.

## Authentification web

Le site exige une connexion, et le test porte sur les requêtes d'utilisateurs connectés et sur la connexion elle-même. Gately utilise l'authentification web et l'authentification basique intégrées au site ; le groupe de comparaison est nginx ou Caddy avec Authelia en frontal (`auth_request` / `forward_auth`), ainsi que l'`auth_basic` propre à nginx. Tout passe en HTTPS.

<BenchBars title="Requêtes par seconde d’utilisateurs connectés (avec cookie de session)" unit="QPS" :better-label="H" :items="[['Gately',26726],['nginx + Authelia',14291],['Lucky',10786],['Caddy + Authelia',7141]]" />

| Configuration | Requêtes connectées/s | Sans authentification | Authentification basique (bcrypt) | Connexions réussies/s | Pic de mémoire à la connexion | Mémoire au repos |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **26,726** | 32,520 | **28,598** | **38.3** | **70 MiB** | **45 MiB** |
| nginx + Authelia | 14,291 | 83,474 | 40 | 13.4 | 1,082 MiB | 120 MiB |
| Caddy + Authelia | 7,141 | 22,703 | 21,213 | 13.4 | 1,271 MiB | 149 MiB |
| Lucky | 10,786 | 13,841 | 10,107 | voir notes | 209 MiB | 127 MiB |

- Hachage des mots de passe : l'authentification basique de Gately et de nginx/Caddy utilise bcrypt (cost 10), Authelia utilise argon2id (t=3, 64 MiB, p=4), et Lucky enregistre en clair les mots de passe des utilisateurs du site. Chaque connexion à Authelia occupe 64 MiB de mémoire, et avec 8 connexions simultanées la mémoire dépasse 1 GB.
- L'`auth_basic` de nginx recalcule bcrypt à chaque requête, ce qui ne donne que 40 requêtes/s ; Gately et Caddy mettent en cache le résultat de la vérification. Avec apr1 (MD5), nginx atteint 13,082 requêtes/s.
- Mauvais mots de passe : Gately rejette directement une source après 10 échecs (il traite 16,000 requêtes erronées par seconde et le CPU ne calcule plus de hachage) ; Authelia ajoute à chaque échec un délai fixe d'environ 1 seconde. Pendant le verrouillage, les deux rejettent aussi le bon mot de passe de cette source, et les sessions déjà ouvertes ne sont pas touchées.
- Avec 8 connexions simultanées, Lucky a renvoyé `create webauth session error` à chaque fois, et son débit de connexion n'a donc pas pu être mesuré.

## WAF

Les trois utilisent OWASP CRS 4.25.0, niveau de paranoïa 1, en mode blocage. Gately et Caddy utilisent le même moteur Coraza, et nginx utilise ModSecurity v3.

<BenchBars title="Requêtes GET par seconde avec le WAF activé" unit="QPS" :better-label="H" :items="[['Gately',2498],['Caddy + Coraza',2265],['nginx + ModSecurity',1491]]" />

| Programme | GET | GET avec 8 paramètres | POST de formulaire de 4 KB | Trafic d'attaque | Pic de mémoire en charge | Mémoire au repos (activé/désactivé) |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **2,498** | **1,067** | 72 | **1,218** | 754 MiB | 88 / 48 MiB |
| nginx + ModSecurity | 1,491 | 591 | **102** | 1,014 | **73 MiB** | 62 / 24 MiB |
| Caddy + Coraza | 2,265 | 943 | 46 | 1,004 | 792 MiB | 96 / 55 MiB |

| Efficacité de détection | Gately | nginx + ModSecurity | Caddy + Coraza |
| --- | --- | --- | --- |
| Échantillons d'attaque bloqués (557) | 77.2% | 77.2% | 77.2% |
| Faux positifs sur échantillons normaux (389) | 20.1% | 20.1% | 20.1% |
| Cas de régression CRS détectés (2049) | 95.9% | 95.2% | 95.7% |

- Les règles CRS sont lourdes en elles-mêmes : avec le WAF activé, le débit des trois ne représente plus que 1 %–10 % de celui obtenu sans WAF ; pour les sites qui ont besoin d'un WAF, il est conseillé de ne l'activer que sur les interfaces dynamiques.
- Lors de l'inspection du corps des requêtes POST, le moteur Coraza (dans Gately comme dans Caddy) dépasse 700 MiB de mémoire au pic, alors que ModSecurity reste à 73 MiB ; soyez prudent en activant l'inspection du corps sur les appareils à peu de mémoire.
- Les échantillons proviennent de gotestwaf (MIT), des tests de régression fournis avec CRS (Apache-2.0) et de requêtes normales écrites à la main ; les faux positifs viennent surtout d'articles techniques contenant des mots-clés SQL/shell et des interfaces PUT/DELETE (CRS n'autorise pas ces méthodes par défaut).

## Prévention d'intrusion

Scénario : un service web est scanné, et une IP est bannie après 5 réponses 404 en 60 secondes (nftables). fail2ban lit le journal d'accès de nginx avec pyinotify ; Gately lit le même journal avec une règle de fichier de journal équivalente, et un autre groupe mesure la détection de chemins intégrée aux services web de Gately.

<BenchBars title="Temps nécessaire pour bannir les 10,000 IP d’attaque" unit="s" better="lower" :digits="1" :better-label="L" :items="[['Gately',12.3],['fail2ban',51.2]]" />

| Indicateur | Gately (lecture du fichier de journal) | Gately (détection intégrée au service web) | fail2ban |
| --- | --- | --- | --- |
| Latence de bannissement d'un attaquant p50 / p99 | 478 / 996 ms | **10 / 13 ms** | 280 / 864 ms |
| CPU à 1000 lignes/s de journal | **2.9%** | — | 20.4% |
| CPU à 20,000 lignes/s de journal | **35.7%** | — | 83.2% |
| 10,000 IP d'attaque : toutes bannies | **12.3 s** | — | 51.2 s |
| 10,000 IP d'attaque : CPU consommé | **10.1 s de CPU** | — | 60.3 s de CPU |
| Mémoire au repos | 44.1 MiB | 40.7 MiB | **17.6 MiB** |
| Manquées / bannies à tort | 0 / 0 | 0 / 0 | 0 / 0 |

- Gately lit le fichier de journal une fois par seconde, si bien que la latence d'un bannissement individuel se situe entre 0 et 1 seconde, soit en moyenne environ 0.2 seconde de plus que l'inotify de fail2ban ; la détection intégrée au service web compte directement pendant le traitement de la requête et n'a pas ce délai.
- fail2ban lance un processus `nft` à chaque bannissement, ce qui crée une file d'attente quand les attaquants sont nombreux : avec 10,000 IP, la dernière a été bannie 41 secondes après la fin de l'injection.
- fail2ban lui-même pèse 2 MB, mais dépend d'un environnement d'exécution Python d'environ 24 MB.

## Port Knocking

Trois approches sont comparées : le Port Knocking de Gately (autorisation par paquet unique SPA compatible avec fwknop, blocage par nftables), fwknop 2.6.11 (également SPA, blocage par iptables) et le knock de fnOS fn-knock 2.5.11 (autorise l'IP source après une connexion web, sa passerelle transférant le trafic). Gately propose aussi une entrée qui autorise l'accès après une connexion web, mesurée comme un groupe à part. Le service protégé est un nginx sur un port TCP, et le client utilise une adresse de test publique, donc l'exemption « le réseau local n'a pas besoin de frapper » ne s'applique pas.

<BenchBars title="Requêtes par seconde en connexions courtes via le port protégé après ouverture" unit="QPS" :better-label="H" :items="[['fwknop',65046],['Gately',64763],['Gately accès web',63556],['fn-knock',1886]]" note="Une connexion directe à l’origine, sans aucun programme de frappe, donne 67,034 requêtes/s." />

| Indicateur | Gately (SPA) | Gately (accès web) | fwknop | fn-knock |
| --- | --- | --- | --- | --- |
| De la frappe au port joignable p50 / p99 | 8.4 / 13.4 ms | 58 / 73 ms | 9.0 / 18.1 ms | 84 / 143 ms |
| Port protégé sans autorisation | Rejet (délai dépassé) | Rejet (délai dépassé) | Rejet (délai dépassé) | Réinitialisation après le handshake (détectable) |
| Requêtes/s en connexion persistante / courte après ouverture | 143,867 / 64,764 | 143,839 / 63,556 | 148,371 / 65,046 | 140,137 / 1,886 |
| CPU du transfert après ouverture (connexions courtes) | 2% | 1% | 0% | 134% |
| Frappes légitimes avec 50,000 paquets invalides/s depuis une seule source | **10/10** (CPU 26%) | — | 3/8 (CPU 102%) | — |
| Frappes légitimes avec 10,000 paquets invalides/s depuis des sources usurpées | 1/7 | — | **10/10** (CPU 54%) | — |
| Mémoire au repos | 48.7 MiB | 48.7 MiB | **2.2 MiB** | 63.2 MiB (3 processus) |

- Avec Gately et fwknop, le pare-feu laisse passer directement le trafic après ouverture et celui-ci ne traverse pas le programme de frappe, si bien que le débit est identique à celui d'une connexion directe ; fn-knock transfère par sa passerelle sur un Linux ordinaire et interroge en plus le plan de contrôle pour l'autorisation à chaque nouvelle connexion, d'où un débit en connexions courtes de seulement 3 % de celui d'une connexion directe. Dans sa version fpk pour fnOS, fn-knock propose aussi un mode de connexion directe au pare-feu ; c'est la version Docker qui a été mesurée ici.
- La connexion à fn-knock exige de résoudre d'abord une preuve de travail ALTCHA (p50 de 28 ms pour la résolution dans ce test) ; sans la résolution, 50 ms.
- fwknopd traite chaque paquet dans un seul thread ; à 50,000 paquets/s depuis une seule source, il sature un cœur et les frappes légitimes se mettent à échouer. Gately limite le débit par source, si bien qu'une inondation depuis une seule source ne consomme presque pas de CPU.
- Problème connu : quand un attaquant usurpe un grand nombre d'adresses sources différentes, la table de limitation de débit par source de Gately se remplit, le quota de débordement partagé par les nouvelles sources est épuisé par l'inondation et la plupart des frappes légitimes échouent (le CPU de Gately n'est alors qu'à environ 19 %). fwknop ne limite pas le débit et n'est pas affecté tant que le CPU n'est pas saturé.

## DNS dynamique

Chaque programme est configuré avec 20 tâches DDNS et une période de vérification de 30 secondes. L'IP publique est interrogée auprès de l'origine locale et les mises à jour sont envoyées à l'origine locale par un « rappel personnalisé », sans toucher à un vrai fournisseur DNS. Après le démarrage et la première synchronisation, on échantillonne pendant 5 minutes.

<BenchBars title="Mémoire moyenne avec 20 tâches DDNS" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['ddns-go',16.9],['Gately',51.6],['Lucky',131.3]]" />

| Programme | Mémoire moyenne | Mémoire de pointe | CPU moyen | Image |
| --- | --- | --- | --- | --- |
| ddns-go 6.17.9 | **16.9 MiB** | **18.2 MiB** | **0.04%** | 43 MB |
| Gately | 51.6 MiB | 55.4 MiB | 0.16% | 229 MB |
| Lucky 3.8.1 | 131.3 MiB | 134.9 MiB | 0.48% | 58 MB |

- Les trois ne sollicitent le fournisseur que lorsque l'IP change ou que la période de synchronisation forcée arrive, de sorte que la charge permanente reste faible pour tous. Si seul le DDNS est nécessaire, ddns-go est le plus économe en mémoire ; le chiffre de Gately inclut tous ses modules, et le même processus peut aussi faire tourner les certificats, le reverse proxy et d'autres fonctions.

## Routeur OpenWrt

Chaque programme tourne sur une VM de routeur ImmortalWrt 25.12.1 (2 vCPU, 1 GB de mémoire). La machine de charge accède au routeur par un pont du réseau local, et le routeur proxifie vers l'origine située sur la machine de charge. nginx 1.26.3 et HAProxy 3.2.21 proviennent du dépôt de paquets d'ImmortalWrt, Gately utilise le paquet apk officiel d'OpenWrt et Lucky le binaire Linux officiel.

<BenchBars title="Requêtes HTTPS par seconde sur le routeur (connexions persistantes)" unit="QPS" :better-label="H" :items="[['nginx',40665],['HAProxy',36266],['Gately',15669],['Lucky',8113]]" />

| Programme | HTTP | HTTPS | TLS nouvelles/s | CPU à 2000 QPS | Mémoire au repos | Pic en charge | Taille du programme |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 16,439 | 15,669 | 2,665 | 27% | 43 MiB | 65 MiB | 40.5 MB |
| nginx | **49,566** | **40,665** | **3,111** | **12%** | **15 MiB** | **22 MiB** | **0.8 MB** |
| HAProxy | 43,046 | 36,266 | 2,337 | 12% | 24 MiB | 29 MiB | 3.7 MB |
| Lucky | 8,642 | 8,113 | 2,021 | 44% | 64 MiB | 81 MiB | 17.9 MB |

- Sur le routeur, le goulot d'étranglement se situe surtout dans l'envoi et la réception réseau du noyau (interruptions logicielles), et aucun programme n'utilise entièrement les deux cœurs, d'où des écarts plus faibles que sur la machine de charge. Le débit de Gately est environ 2 fois celui de Lucky, avec un tiers de mémoire en moins.
- Le noyau d'OpenWrt n'a pas de smaps ; la mémoire est donc estimée ici à partir de `/proc/<pid>/statm` (somme de la mémoire résidente privée de chaque processus plus une copie de la partie partagée). Pendant le test, la protection contre les inondations SYN du pare-feu par défaut (environ 25 nouvelles connexions par seconde) a été désactivée temporairement, faute de quoi elle aurait plafonné le test de nouvelles connexions TLS.
- Pour un simple reverse proxy, le nginx du dépôt de paquets est le plus petit en taille comme en mémoire ; les 40 MB de Gately comprennent tous les modules, et sur [build.gately.cn](https://build.gately.cn) vous pouvez ne choisir que les modules nécessaires pour générer un paquet d'installation plus petit. Sur les routeurs dotés de peu de flash, vérifiez d'abord l'espace restant.

## Tout-en-un

Un même Gately active simultanément : deux sites en reverse proxy (HTTP et HTTPS, tous deux avec WAF), un site avec authentification web, 20 tâches DDNS, une règle de journal de prévention d'intrusion et une règle de Port Knocking. Le groupe de comparaison est une combinaison d'outils indépendants assurant les mêmes fonctions, en additionnant la mémoire au repos mesurée plus haut pour chacun.

<BenchBars title="Mémoire au repos totale pour le même ensemble de fonctions" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['Gately',90.8],['nginx + ModSecurity + Authelia + ddns-go + fail2ban + fwknop',203.5],['Nginx Proxy Manager + Authelia + ddns-go + fail2ban + fwknop',297.3]]" note="Le groupe Nginx Proxy Manager n’a pas de WAF." />

| Fonction | Outil indépendant | Mémoire au repos | Taille d'installation |
| --- | --- | --- | --- |
| Reverse proxy + WAF | nginx + ModSecurity (CRS 4.25) | 61.9 MiB | Image de 156 MB |
| Reverse proxy (interface graphique) | Nginx Proxy Manager | 155.7 MiB | Image de 1,959 MB |
| Authentification web | Authelia | 104.4 MiB | Image de 110 MB |
| DNS dynamique | ddns-go | 17.4 MiB | Image de 43 MB |
| Prévention d'intrusion | fail2ban | 17.6 MiB | 24 MB (avec Python) |
| Port Knocking | fwknop | 2.2 MiB | 0.3 MB |
| **Tout** | **Gately** | **90.8 MiB** | **Programme de 40 MB (téléchargement de 18 MB)** |

- L'instance tout-en-un de Gately, sous 1000 requêtes/s de trafic HTTPS passant par le WAF, atteint un pic de mémoire de 114 MiB et un CPU de 71 % ; les 20 tâches DDNS se sont toutes synchronisées avec succès.
- L'émission de certificats (certbot, acme.sh) ne s'exécute qu'au renouvellement et n'est pas comptée dans la mémoire résidente.

## Reproduire

```bash
git clone https://github.com/wangwanjie/gately && cd gately/tools/benchmark
# À exécuter en root sur une VM Debian 13 ; nécessite Docker ainsi que wrk, oha et h2load
bash provision-debian.sh
python3 lib/run.py gately 3     # reverse proxy ; remplacez par nginx, haproxy, caddy, traefik, lucky, npm
bash auth/run.sh                # authentification web
bash waf/run.sh                 # WAF
bash guard/run.sh               # prévention d'intrusion
bash knock/run.sh               # Port Knocking
bash scale/run.sh 1000          # 1000 sites
bash ddns/run.sh 20             # DNS dynamique
bash bundle/run.sh              # tout-en-un
bash router/run.sh              # routeur (nécessite en plus une machine OpenWrt)
```

Le README de chaque répertoire décrit la configuration, la méthode de mesure et les limites connues. Les résultats dépendent du matériel, de la virtualisation et de la configuration ; il est conseillé de refaire les tests avec votre propre charge de travail.
