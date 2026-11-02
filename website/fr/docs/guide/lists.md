# Groupes, tri, opérations en lot et duplication des listes

Les listes comportant de nombreuses entrées peuvent être groupées, triées manuellement, filtrées par recherche et modifiées en lot, avec les mêmes commandes sur toutes les pages. L'ordre et les groupes n'agissent que sur l'affichage dans l'interface d'administration ; les modifier ne redémarre pas les instances en cours et ne coupe aucune connexion.

## Listes prises en charge

| Page | Listes |
| --- | --- |
| DNS dynamique | Tâches ; enregistrements DNS d'une tâche |
| Traversée STUN | Règles |
| Redirection de ports | Règles |
| Certificats SSL | Certificats |
| Services Web | Sites d'un écouteur (les écouteurs eux-mêmes peuvent monter et descendre) |
| Pare-feu WAF | Instances |
| Prévention d'intrusion | Règles de journaux |
| frp | Instances frpc, instances frps, proxys |
| Tunnel Cloudflare | Tunnels |
| File Browser | Instances |
| Montages de stockage | Montages |
| Wake-on-LAN | Appareils |
| Tâches planifiées | Tâches |
| Notifications | Canaux, règles |
| Terminal Web | Connexions enregistrées |
| Jetons d'API | Jetons |

La plupart de ces listes proposent une « Vue en cartes » et une « Vue compacte », à basculer en haut à droite de la liste ; sur les écrans étroits, la vue en cartes est toujours utilisée.

## Groupes

Renseignez « Groupe » dans la boîte de dialogue d'édition (64 caractères au maximum ; vous pouvez choisir un groupe existant). Laissez le champ vide pour aucun groupe. Dès qu'une entrée possède un groupe, la liste s'affiche en sections repliables par groupe : les groupes apparaissent dans l'ordre de leur première occurrence, les entrées sans groupe viennent en dernier et l'état replié de chaque groupe est enregistré dans le navigateur. La zone de recherche filtre les entrées par nom, groupe, remarque et champs similaires, et déplie tous les groupes pendant la recherche.

Survolez l'en-tête d'un groupe et cliquez sur le bouton crayon pour le renommer : toutes les entrées du groupe prennent le nouveau nom et conservent leur position et leur ordre. Si vous lui donnez le nom d'un groupe existant, les deux groupes sont fusionnés après confirmation.

## Mode de tri

Cliquez sur « Tri et lot » pour passer en mode de tri. Toutes les entrées sont alors affichées, et la recherche et les filtres sont suspendus :

- Utilisez les flèches pour monter ou descendre une entrée dans son groupe, ou celles de l'en-tête pour déplacer tout le groupe ;
- Sur ordinateur, vous pouvez faire glisser les entrées directement ; les déposer sur une entrée ou l'en-tête d'un autre groupe les ajoute à ce groupe ;
- Après avoir coché plusieurs entrées (la case de l'en-tête sélectionne tout le groupe), faites glisser l'une d'elles et toutes les entrées sélectionnées se déplacent ensemble dans leur ordre actuel ;
- La barre d'outils permet de « Déplacer dans le groupe » les entrées sélectionnées en une fois ; laisser le champ vide les retire de leur groupe.

Les téléphones et autres écrans étroits ne prennent pas en charge le glisser-déposer ; utilisez les flèches et la barre d'outils pour les mêmes opérations.

## Opérations en lot

Après avoir coché des entrées en mode de tri, la barre d'outils permet aussi de :

- **Activer / Désactiver** : enregistre toutes les entrées cochées en une fois ; les entrées déjà dans l'état visé ne sont pas redémarrées ;
- **Supprimer** : la boîte de confirmation liste les entrées à supprimer, et la suppression est définitive. Si une entrée ne peut pas être supprimée (par exemple un certificat encore utilisé par un site), aucune n'est supprimée et la raison est affichée.

Une opération en lot traite jusqu'à 500 entrées. Elle n'est enregistrée que si toutes les entrées passent la validation ; si l'une échoue, rien n'est modifié. Les appareils Wake-on-LAN n'ont pas d'interrupteur d'activation et ne proposent que la suppression en lot ; les jetons d'API ne proposent pas encore l'activation, la désactivation ni la suppression en lot. Services Web propose aussi la modification en lot des domaines et des options, voir [Services Web](/fr/docs/modules/web#operations-en-lot).

## Dupliquer des éléments

Tous les éléments du tableau ci-dessus, sauf les jetons d'API, peuvent être dupliqués, de même que les listes d'IP abonnées, les destinations de sauvegarde, les préréglages d'identifiants du Terminal web, les enregistrements d'une tâche DDNS et les sites d'un écouteur du Service web. Cliquez sur « Dupliquer » sur un élément pour ouvrir la boîte d'ajout avec cet élément comme modèle ; le nom reçoit le suffixe « (copie) ». Modifiez les paramètres qui doivent différer et enregistrez pour ajouter un nouvel élément. L'original reste inchangé et la copie est placée juste après lui.

- **Les secrets sont repris de l'original** : les mots de passe, jetons et clés privées enregistrés restent masqués dans la boîte de dialogue. Si vous ne les modifiez pas, la copie utilise les valeurs de l'original et le texte en clair ne transite jamais par le navigateur. Le bouton en forme d'œil affiche le texte en clair de l'original. Pour les connexions et préréglages d'identifiants du Terminal web, il faut accepter l'accord d'utilisation et passer la vérification en deux étapes avant de réutiliser des secrets.
- **Le contenu géré par le système n'est pas copié** : les certificats obtenus via ACME ou lus depuis des fichiers sont émis ou relus avec les paramètres de la copie ; un certificat téléversé est copié avec sa clé privée. Les historiques de déploiement des certificats sont effacés, les listes d'IP abonnées sont retéléchargées et les liens de partage des sites ne sont pas copiés, car leurs jetons contournent l'authentification.
- **Cibles de déploiement du certificat** : elles sont désactivées dans la copie pour qu'elle n'écrive pas dans les mêmes fichiers que le certificat d'origine. Activez-les après avoir vérifié que les noms de fichiers ne se chevauchent pas.
- **Service web** : en dupliquant un site, vous pouvez le placer sous un autre écouteur (par exemple d'un écouteur HTTP vers un écouteur HTTPS). En dupliquant un écouteur, vous pouvez choisir de copier aussi ses sites.
- La copie conserve l'état d'activation de l'original. Avant d'enregistrer, modifiez les ports, domaines, enregistrements et autres paramètres qui doivent différer pour éviter tout conflit avec l'original.

## Remarques par page

- **DNS dynamique** : « Trier les enregistrements » sur la carte d'une tâche ouvre une boîte de dialogue pour régler l'ordre et les groupes des enregistrements DNS de cette tâche. L'enregistrement prend effet immédiatement, sans nouvelle synchronisation. Lors de l'édition d'une tâche, la même boîte de dialogue est disponible dans l'onglet « Enregistrements DNS » et est enregistrée avec la tâche. Lorsque les enregistrements ont des groupes, ils s'affichent par groupe sur la carte, et chaque groupe se replie séparément.
- **frp** : La liste des proxys peut être filtrée par instance frpc ; le mode de tri affiche tous les proxys et règle l'ordre complet.
- **Jetons d'API** : Tant qu'aucun tri manuel n'a été fait, les jetons s'affichent par date de création, du plus récent au plus ancien. Une fois l'ordre ajusté, ils s'affichent dans l'ordre enregistré, et les nouveaux jetons sont placés en tête.
- **Utilisateurs d'auth** : « Sites utilisant les utilisateurs partagés » s'affiche par écouteur, et les sites d'un écouteur sont regroupés selon leurs groupes dans Services Web. Écouteurs et groupes se replient séparément, et la barre d'outils permet de tout replier ou déplier d'un clic.

## API

Tous les points de terminaison de listes génériques proposent `PUT <chemin de la liste>-order` (corps `{"keys": [...]}`, qui doit contenir toutes les entrées) pour modifier l'ordre et `PUT <chemin de la liste>-group` (corps `{"keys": [...], "group": "..."}`) pour définir le groupe en lot, `PUT <chemin de la liste>-enable` (corps `{"keys": [...], "enable": true}`) pour activer ou désactiver en lot et `POST <chemin de la liste>-delete` (corps `{"keys": [...]}`) pour supprimer en lot, par exemple `PUT /api/stun/rules-group` et `POST /api/portforward/rules-delete`. Les enregistrements d'une tâche DDNS utilisent `PUT /api/ddns/tasks/{key}/records-layout`, et les jetons d'API utilisent `PUT /api/tokens/order` et `PUT /api/tokens/group`. Voir la [Référence de l'API](/fr/docs/reference/api-reference) pour plus de détails.

La duplication utilise `POST <chemin de la liste>/{key}/copy`. Le corps est le nouvel élément, et les secrets restés au marqueur `********` reprennent les valeurs de l'élément `{key}`, par exemple `POST /api/frp/clients/{key}/copy`.
