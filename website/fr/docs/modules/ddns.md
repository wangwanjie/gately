# DNS dynamique (DDNS)

L'IP publique d'une connexion internet domestique change avec le temps. Le DDNS met à jour automatiquement vos enregistrements DNS après un changement d'IP, afin que votre domaine pointe toujours vers votre adresse actuelle.

## Fournisseurs pris en charge

Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2, ainsi qu'un rappel personnalisé (pour les fournisseurs non intégrés ou les points de terminaison auto-hébergés).

| Fournisseur | Identifiants | Utilisable pour la validation DNS des certificats |
| --- | --- | --- |
| Amazon Route 53 | Access Key ID / Secret Access Key, ID de zone hébergée facultatif | Oui |
| OVHcloud | Application Key / Secret et Consumer Key, point d'accès ovh-eu / ovh-ca / ovh-us | Oui |
| Gandi | Personal Access Token | Oui |
| NS1 | API Key | Oui |
| Bunny DNS | Account API Key | Oui |
| Namecheap | Mot de passe Dynamic DNS du domaine (enregistrements A uniquement) | Non |
| DynDNS2 | Nom d'utilisateur, mot de passe et URL de mise à jour, pour No-IP, Dynu et les autres services compatibles avec le protocole dyndns2 | Non |

## Obtention de l'IP

- **URL** : interroge une adresse de consultation d'IP publique (remplacez-la par le service de votre choix, ou hébergez le vôtre) ;
- **Interface réseau** : lit directement l'adresse d'une interface réseau précise, adapté aux appareils qui détiennent une IP publique ;
- **Commande** : exécute une commande et utilise sa sortie, adapté aux configurations réseau atypiques.

IPv4 et IPv6 sont tous deux pris en charge.

## Variables de la valeur d'enregistrement {#record-vars}

Les valeurs d'enregistrement peuvent utiliser ces variables, remplacées par les valeurs actuelles à chaque synchronisation :

- `{ipv4Addr}`, `{ipv6Addr}` : les adresses IPv4 et IPv6 obtenues par la tâche (utilisées par défaut lorsque la valeur d'un enregistrement A ou AAAA est vide) ;
- `{STUN_nom de règle_PORT}`, `{STUN_nom de règle_IP}`, `{STUN_nom de règle_ADDR}` : port public, IP et IP:port actuels d'une règle STUN ; le nom de règle peut contenir des tirets bas. Par exemple un enregistrement TXT `port={STUN_ssh_PORT}`, ou un enregistrement A `{STUN_ssh_IP}`.

Quand l'adresse publique d'une règle STUN change, les tâches qui y font référence se synchronisent aussitôt, sans attendre la vérification suivante. Tant que la règle est désactivée ou n'a pas encore réussi la traversée, l'enregistrement est ignoré pour ce tour et son état en indique la raison. Les variables STUN sont aussi utilisables dans le Webhook de la tâche.

## Notifications

Un Webhook peut être appelé lorsque l'IP change ou qu'une synchronisation échoue.

## Recommandations

- Créez chez votre fournisseur DNS une clé de sous-compte limitée à la modification des enregistrements DNS. Évitez d'utiliser la clé du compte principal ;
- Après avoir créé une tâche, cliquez sur « Forcer la synchronisation » pour vérifier que l'enregistrement a bien été mis à jour ;
- Pour retirer un enregistrement devenu inutile, cliquez sur l'icône de suppression à droite de cet enregistrement dans la liste, sans ouvrir l'éditeur ; il est seulement retiré de la tâche, l'enregistrement DNS existant chez le fournisseur est conservé et la tâche garde au moins un enregistrement ;
- L'adresse de consultation de l'IP publique ne dépend plus d'aucun service exploité par le projet, et vous pouvez la modifier à tout moment dans une tâche.

## Groupes et tri

Vous pouvez attribuer un « Groupe » (64 caractères au plus) à les tâches. La liste affiche les groupes sous forme de sections repliables : les groupes apparaissent dans l’ordre de leur première occurrence, les tâches sans groupe viennent en dernier, et l’état replié est conservé dans le navigateur. La zone de recherche en haut de la page filtre par nom, groupe, domaine ou fournisseur.

Cliquez sur « Trier » pour passer en mode tri. Les flèches déplacent les tâches vers le haut ou le bas, ou des groupes entiers. Cochez-en plusieurs et faites glisser l’un d’eux pour les déplacer ensemble ; déposés sur un élément ou l’en-tête d’un autre groupe, ils rejoignent ce groupe. La barre d’outils place d’un coup les éléments cochés dans un groupe, ou les retire de leur groupe si le nom est laissé vide. L’ordre et les groupes n’affectent que la liste, et les modifier signifie que les tâches ne sont pas redémarrées.

Survolez l’en-tête d’un groupe et cliquez sur le crayon pour le renommer : tous ses éléments passent sous le nouveau nom et gardent leur position et leur ordre. Un nom déjà utilisé fusionne les deux groupes après confirmation.
