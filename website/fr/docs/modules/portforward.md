# Redirection de ports

Redirigez un port sur lequel Gately écoute vers un autre appareil ou service.

## Fonctions

- TCP et UDP ;
- IPv4 et IPv6, avec redirection entre v4 et v6 ;
- Plages de ports et ports multiples ;
- Cibles multiples avec basculement ;
- Limites de connexions ;
- Statistiques de trafic et journaux d'exécution ;
- Envoi du protocole PROXY (v1 / v2) à la cible, pour transmettre l'adresse réelle du client en aval ;
- Contrôle d'accès par IP source (liste d'autorisation / de blocage), pouvant utiliser les listes d'IP abonnées des Services Web ;
- Pour que plusieurs services partagent un même port comme le 443 selon le domaine ou le protocole, utilisez le [Routage par protocole](/fr/docs/modules/protoroute).

## Protocole PROXY

La redirection de ports ne fait que relayer le TCP : par défaut, le service en aval ne voit que l'adresse de Gately. Si vous réglez « Protocole PROXY » sur v1 ou v2 dans une règle, Gately envoie d'abord un en-tête PROXY après s'être connecté à la cible ; le service en aval obtient alors l'adresse réelle du client pour ses journaux, sa limitation de débit et son contrôle d'accès.

La cible doit activer l'option de réception correspondante, sinon la connexion sera refusée. Par exemple :

- haproxy : `bind 192.168.1.8:4434 accept-proxy` ;
- nginx : `listen 4435 ssl proxy_protocol;` ;
- Service Web Gately : activez « Accepter le protocole PROXY » sur l'écouteur et ajoutez la source aux sources PROXY de confiance.

Cette option ne concerne que le TCP ; la redirection UDP n'est pas affectée.

## Contrôle d'accès par IP source

Chaque règle peut utiliser une liste d'autorisation ou de blocage qui décide d'après l'IP source si le trafic est relayé. La liste se compose d'IP / sous-réseaux saisis à la main et des listes d'IP abonnées des Services Web ; correspondre à l'une d'elles revient à figurer dans la liste. Les listes abonnées sont téléchargées périodiquement par le module Services Web, une actualisation s'applique immédiatement et une liste compte comme vide jusqu'à son premier téléchargement réussi.

- La vérification a lieu avant la connexion à la cible : une connexion TCP qui échoue est fermée, et UDP est vérifié par paquet : les paquets qui échouent sont abandonnés et les paquets montants des sessions existantes suivent immédiatement la liste modifiée ;
- Les bannissements de la Prévention des intrusions passent en premier : une adresse bannie est refusée même dans un sous-réseau autorisé ;
- Modifier la liste n'interrompt pas l'écoute et les connexions TCP établies ne sont pas affectées ;
- La carte de la règle affiche le nombre de refus depuis le démarrage de Gately (connexions en TCP, paquets en UDP), et le niveau débogage du journal consigne chaque refus ;
- Si un proxy inverse ou un autre relais se trouve devant la règle, la source vue par Gately est son adresse.

Lors d'une importation depuis Lucky, le mode sécurisé de la règle est aussi converti : le mode liste d'autorisation reprend les IP / sous-réseaux de la « liste blanche » du filtre IP de Lucky, le mode liste de blocage reprend la « liste noire » et la « liste noire globale ». Envoyez `lucky_ipfilter.lkcf` (ou une sauvegarde complète) dans la même importation ; les entrées de base IP, d'enregistrement automatique ou de plage d'adresses ne peuvent pas être importées, le résultat le signale. Si le mode sécurisé est désactivé, aucune liste n'est définie ; s'il désigne une règle de filtre IP personnalisée, son action décide : accepter devient une liste d'autorisation, refuser une liste de blocage (fusionnée elle aussi avec la liste noire globale).

## Scénarios typiques

- Exposer les ports des services d'un NAS via un point d'entrée unique ;
- Faire le pont entre le trafic IPv6 et IPv4 ;
- Configurer un backend principal et un backend de secours pour un même service.

## Ouvrir le port dans le pare-feu {#auto-firewall}

Avec « Ouvrir le port dans le pare-feu » activé, Gately autorise dans le pare-feu de cet hôte uniquement les ports sur lesquels la règle écoute : les ports consécutifs sont regroupés en une seule règle, TCP / UDP et IPv4 / IPv6 suivent les protocoles de la règle et, avec une adresse d’écoute renseignée, seules les connexions vers cette adresse sont autorisées. La règle est retirée quand elle est désactivée ou supprimée. Voir [Services Web · Ouvrir le port dans le pare-feu](/fr/docs/modules/web#auto-firewall) pour les cas d’usage, les pare-feu pris en charge et l’autorisation à la main.

## Conseil de déploiement

La redirection de ports écoute directement sur les ports de l'hôte. Pour un déploiement Docker, `--network host` est recommandé.

## Groupes et tri

Vous pouvez attribuer un « Groupe » (64 caractères au plus) à les règles. La liste affiche les groupes sous forme de sections repliables : les groupes apparaissent dans l’ordre de leur première occurrence, les règles sans groupe viennent en dernier, et l’état replié est conservé dans le navigateur. La zone de recherche en haut de la page filtre par nom, groupe, port d’écoute ou cible.

Cliquez sur « Trier » pour passer en mode tri. Les flèches déplacent les règles vers le haut ou le bas, ou des groupes entiers. Cochez-en plusieurs et faites glisser l’un d’eux pour les déplacer ensemble ; déposés sur un élément ou l’en-tête d’un autre groupe, ils rejoignent ce groupe. La barre d’outils place d’un coup les éléments cochés dans un groupe, ou les retire de leur groupe si le nom est laissé vide. L’ordre et les groupes n’affectent que la liste, et les modifier signifie que le transfert n’est pas redémarré et les connexions ouvertes sont conservées.

Survolez l’en-tête d’un groupe et cliquez sur le crayon pour le renommer : tous ses éléments passent sous le nouveau nom et gardent leur position et leur ordre. Un nom déjà utilisé fusionne les deux groupes après confirmation.
