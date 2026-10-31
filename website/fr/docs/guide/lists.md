# Groupes et tri des listes

Les listes comportant de nombreuses entrées peuvent être groupées, triées manuellement et filtrées par recherche, avec les mêmes commandes sur toutes les pages. L'ordre et les groupes n'agissent que sur l'affichage dans l'interface d'administration ; les modifier ne redémarre pas les instances en cours et ne coupe aucune connexion.

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

Cliquez sur « Trier » pour passer en mode de tri. Toutes les entrées sont alors affichées, et la recherche et les filtres sont suspendus :

- Utilisez les flèches pour monter ou descendre une entrée dans son groupe, ou celles de l'en-tête pour déplacer tout le groupe ;
- Sur ordinateur, vous pouvez faire glisser les entrées directement ; les déposer sur une entrée ou l'en-tête d'un autre groupe les ajoute à ce groupe ;
- Après avoir coché plusieurs entrées (la case de l'en-tête sélectionne tout le groupe), faites glisser l'une d'elles et toutes les entrées sélectionnées se déplacent ensemble dans leur ordre actuel ;
- La barre d'outils permet de « Déplacer dans le groupe » les entrées sélectionnées en une fois ; laisser le champ vide les retire de leur groupe.

Les téléphones et autres écrans étroits ne prennent pas en charge le glisser-déposer ; utilisez les flèches et la barre d'outils pour les mêmes opérations.

## Remarques par page

- **DNS dynamique** : « Trier les enregistrements » sur la carte d'une tâche ouvre une boîte de dialogue pour régler l'ordre et les groupes des enregistrements DNS de cette tâche. L'enregistrement prend effet immédiatement, sans nouvelle synchronisation. Lors de l'édition d'une tâche, la même boîte de dialogue est disponible dans l'onglet « Enregistrements DNS » et est enregistrée avec la tâche. Lorsque les enregistrements ont des groupes, ils s'affichent par groupe sur la carte, et chaque groupe se replie séparément.
- **frp** : La liste des proxys peut être filtrée par instance frpc ; le mode de tri affiche tous les proxys et règle l'ordre complet.
- **Jetons d'API** : Tant qu'aucun tri manuel n'a été fait, les jetons s'affichent par date de création, du plus récent au plus ancien. Une fois l'ordre ajusté, ils s'affichent dans l'ordre enregistré, et les nouveaux jetons sont placés en tête.
- **Utilisateurs d'auth** : « Sites utilisant les utilisateurs partagés » s'affiche par écouteur, et les sites d'un écouteur sont regroupés selon leurs groupes dans Services Web. Écouteurs et groupes se replient séparément, et la barre d'outils permet de tout replier ou déplier d'un clic.

## API

Tous les points de terminaison de listes génériques proposent `PUT <chemin de la liste>-order` (corps `{"keys": [...]}`, qui doit contenir toutes les entrées) pour modifier l'ordre et `PUT <chemin de la liste>-group` (corps `{"keys": [...], "group": "..."}`) pour définir le groupe en lot, par exemple `PUT /api/stun/rules-group`. Les enregistrements d'une tâche DDNS utilisent `PUT /api/ddns/tasks/{key}/records-layout`, et les jetons d'API utilisent `PUT /api/tokens/order` et `PUT /api/tokens/group`. Voir la [Référence de l'API](/fr/docs/reference/api-reference) pour plus de détails.
