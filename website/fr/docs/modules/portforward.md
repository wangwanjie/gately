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
- Pour que plusieurs services partagent un même port comme le 443 selon le domaine ou le protocole, utilisez le [Routage par protocole](/fr/docs/modules/protoroute).

## Protocole PROXY

La redirection de ports ne fait que relayer le TCP : par défaut, le service en aval ne voit que l'adresse de Gately. Si vous réglez « Protocole PROXY » sur v1 ou v2 dans une règle, Gately envoie d'abord un en-tête PROXY après s'être connecté à la cible ; le service en aval obtient alors l'adresse réelle du client pour ses journaux, sa limitation de débit et son contrôle d'accès.

La cible doit activer l'option de réception correspondante, sinon la connexion sera refusée. Par exemple :

- haproxy : `bind 192.168.1.8:4434 accept-proxy` ;
- nginx : `listen 4435 ssl proxy_protocol;` ;
- Service Web Gately : activez « Accepter le protocole PROXY » sur l'écouteur et ajoutez la source aux sources PROXY de confiance.

Cette option ne concerne que le TCP ; la redirection UDP n'est pas affectée.

## Scénarios typiques

- Exposer les ports des services d'un NAS via un point d'entrée unique ;
- Faire le pont entre le trafic IPv6 et IPv4 ;
- Configurer un backend principal et un backend de secours pour un même service.

## Conseil de déploiement

La redirection de ports écoute directement sur les ports de l'hôte. Pour un déploiement Docker, `--network host` est recommandé.

## Groupes et tri

Vous pouvez attribuer un « Groupe » (64 caractères au plus) à les règles. La liste affiche les groupes sous forme de sections repliables : les groupes apparaissent dans l’ordre de leur première occurrence, les règles sans groupe viennent en dernier, et l’état replié est conservé dans le navigateur. La zone de recherche en haut de la page filtre par nom, groupe, port d’écoute ou cible.

Cliquez sur « Trier » pour passer en mode tri. Les flèches déplacent les règles vers le haut ou le bas, ou des groupes entiers. Cochez-en plusieurs et faites glisser l’un d’eux pour les déplacer ensemble ; déposés sur un élément ou l’en-tête d’un autre groupe, ils rejoignent ce groupe. La barre d’outils place d’un coup les éléments cochés dans un groupe, ou les retire de leur groupe si le nom est laissé vide. L’ordre et les groupes n’affectent que la liste, et les modifier signifie que le transfert n’est pas redémarré et les connexions ouvertes sont conservées.

Survolez l’en-tête d’un groupe et cliquez sur le crayon pour le renommer : tous ses éléments passent sous le nouveau nom et gardent leur position et leur ordre. Un nom déjà utilisé fusionne les deux groupes après confirmation.
