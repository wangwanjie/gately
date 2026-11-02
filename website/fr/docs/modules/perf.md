# Performances

Performances réunit sur une page les données de l'hôte, du processus Gately et de chaque site, et envoie une alerte via le [Centre de notifications](/fr/docs/modules/notify) quand une métrique reste au-dessus de son seuil, puis un avis de rétablissement quand la condition cesse. Ouvrez-le depuis la barre latérale, Système → Performances.

Il couvre des cas comme un CPU saturé en pleine nuit, un afflux de connexions sur un site ou un backend qui ralentit sans que personne ne s'en aperçoive, et ne conserve ses données qu'en mémoire. Pour un historique de plus de 24 heures, une agrégation entre instances ou des requêtes complexes, utilisez [Prometheus et Grafana](/fr/docs/reference/monitoring).

## Tableau de bord

Basculez entre 60 secondes, 1 heure et 24 heures en haut à droite de la page.

### Hôte et processus Gately

| Graphique | Contenu |
| --- | --- |
| CPU de l'hôte | Utilisation moyenne de tous les cœurs de la machine |
| Mémoire / disque de l'hôte | Mémoire utilisée par rapport à la mémoire physique (cache récupérable non compté), part utilisée de la partition du répertoire de configuration |
| Charge système | Charge sur 1 minute |
| CPU du processus Gately | Part de la machine entière utilisée par le processus Gately, mesurée comme le CPU de l'hôte |
| Mémoire du processus Gately | Mémoire résidente (Mo) |
| Descripteurs de fichiers / goroutines | Descripteurs de fichiers ouverts et goroutines Go du processus |

Un échantillon est pris toutes les 5 secondes : la plage de 60 secondes affiche chaque échantillon ; celle d'1 heure, un point par minute (la moyenne de la minute) ; celle de 24 heures, un point toutes les 5 minutes. La ligne pointillée des graphiques CPU est le pic de chaque point, pour que les pics brefs ne disparaissent pas dans la moyenne. Survolez un graphique pour lire les valeurs à un instant donné.

Certaines plateformes ne fournissent pas toutes les métriques (par exemple pas d'utilisation CPU sous macOS) ; ces graphiques affichent « Aucune donnée ».

### Sites

Quand le module Services web est actif, tous les sites en cours d'exécution (sites par défaut compris) sont listés :

| Colonne | Signification |
| --- | --- |
| En cours | Requêtes que le site traite en ce moment, y compris les connexions longues comme WebSocket |
| Requêtes/s | Moyenne des requêtes par seconde sur environ les 60 dernières secondes |
| Taux de 5xx | Réponses 5xx en pourcentage des réponses terminées sur environ les 60 dernières secondes |
| Latence P95 | 95e centile estimé du temps jusqu'au premier octet sur environ les 60 dernières secondes |
| Backends | Backends sains / tous les backends, et connexions en cours de relais |

Cliquez sur un site pour ouvrir ses [statistiques](/fr/docs/modules/web#site-stats) avec les tendances sur 24 heures des requêtes, des erreurs et de la latence.

### Temps jusqu'au premier octet

La latence est le « temps jusqu'au premier octet » : de l'arrivée de la requête à l'envoi des en-têtes de réponse, ce qui reflète la réactivité du backend ou du site lui-même. Ne sont pas comptés :

- le temps de transfert du corps de la réponse (les gros téléchargements n'augmentent pas la latence) ;
- WebSocket et les autres requêtes avec changement de protocole ;
- les requêtes coupées par le contrôle d'accès ou par un administrateur.

Chaque requête tombe dans l'une de 50 tranches fines (de 1 ms à 60 s, environ 10 % à 25 % d'écart entre voisines). Chaque tranche cumule le nombre de requêtes et le temps total ; les centiles utilisent le temps moyen de la tranche concernée, si bien que l'erreur ne dépasse jamais la largeur de la tranche, et rien n'est conservé sur les requêtes individuelles. L'histogramme `gately_site_request_duration_seconds` exporté vers Prometheus utilise 13 bornes courantes parmi celles-ci.

## Règles d'alerte

Cliquez sur « Ajouter une règle d'alerte ». À la création, un modèle permet de préremplir une règle courante (CPU de l'hôte élevé, espace disque faible, erreurs 5xx du site, réponses lentes du site, etc.) à ajuster ensuite.

| Champ | Description |
| --- | --- |
| Nom | Obligatoire, 64 caractères au plus, affiché dans le titre de la notification |
| Métrique | Voir le tableau ci-dessous |
| Seuil | Opérateur `>`, `>=`, `<` ou `<=` et une valeur ; l'unité dépend de la métrique |
| Durée | De 1 à 1440 minutes. L'alerte n'est envoyée que si la condition se maintient aussi longtemps ; un seul passage sous le seuil relance le compteur |
| Sites | Pour les métriques de sites et de nœuds de backend ; vide pour tous les sites |
| Requêtes minimum | Pour le taux de 5xx et la latence, 20 par défaut. Pas d'évaluation tant que les ~60 dernières secondes comptent moins de requêtes, afin qu'une ou deux erreurs ou requêtes lentes sur un site calme ne déclenchent pas d'alerte |
| Niveau d'alerte | Avertissement ou erreur ; fixe le niveau de l'événement de notification |
| Remarque, activer | Facultatif |

| Métrique | Unité | Évaluée pour |
| --- | --- | --- |
| Utilisation CPU de l'hôte | % | Cet hôte |
| Utilisation mémoire de l'hôte | % | Cet hôte |
| Utilisation disque de l'hôte (partition du répertoire de configuration) | % | Cet hôte |
| Utilisation CPU du processus Gately | % | Processus Gately |
| Mémoire résidente du processus Gately | Mo | Processus Gately |
| Requêtes en cours du site | nombre | Chaque site |
| Requêtes par seconde du site | req/s | Chaque site |
| Taux de réponses 5xx du site | % | Chaque site |
| Temps jusqu'au premier octet P95 du site | ms | Chaque site |
| Connexions du nœud de backend | nombre | Chaque nœud de backend |

Les métriques de sites et de nœuds de backend sont évaluées et alertées objet par objet : une seule règle « taux de 5xx > 5 % » peut alerter séparément sur deux sites. « Requêtes par seconde du site < 1 » détecte un trafic soudainement interrompu.

### Évaluation et notifications

1. Toutes les règles activées sont évaluées toutes les 5 secondes.
2. Quand la condition commence à être remplie, l'objet passe « En attente » ; après la durée, il passe « En alerte » et un événement `perf.alert` est publié.
3. Quand la condition d'une alerte déclenchée cesse, un événement `perf.recovered` est publié. Le taux de 5xx et la latence d'un site ne peuvent pas être évalués si les ~60 dernières secondes comptent trop peu de requêtes ; cela compte aussi comme un rétablissement, que l'avis précise.
4. Quand un site est supprimé ou désactivé, ses alertes disparaissent sans avis de rétablissement.

Le haut de la page liste les objets en attente et en alerte avec leur valeur actuelle et leur durée.

Modifier le nom, le niveau ou la remarque d'une règle conserve le compteur en cours ; modifier la métrique, le seuil, la durée, les sites ou les requêtes minimum, ou désactiver la règle, efface ses compteurs et alertes. Compteurs et états d'alerte ne vivent qu'en mémoire et repartent de zéro après un redémarrage.

Les alertes passent par le Centre de notifications : ajoutez une règle de notification abonnée à « Performances : alerte déclenchée » et « Performances : alerte rétablie » (le modèle « Alertes de performances » le fait), sinon les alertes sont seulement écrites dans le journal de cette page et la page affiche un avertissement. Le silence du Centre de notifications s'applique aussi aux alertes de performances ; chaque objet a sa propre source d'événement, ils ne se masquent donc pas entre eux. La durée évite déjà les alertes intermittentes, un silence est donc rarement utile.

## Ressources et limites

- L'historique n'est conservé qu'en mémoire : l'hôte et le processus ont un point par minute sur 24 heures ; la latence des sites conserve P50 et P95 par minute, environ 23 Ko par site avec du trafic.
- Jusqu'à 200 règles d'alerte et 200 sites par règle.
- Quand le module Services web est actif, chaque requête coûte une lecture d'horloge supplémentaire et quelques mises à jour de compteurs.

## Métriques de supervision

Quand Performances est actif, `/api/metrics` exporte aussi les métriques de l'hôte et du processus du dernier échantillon et le nombre d'alertes déclenchées par règle ; voir [Supervision](/fr/docs/reference/monitoring).

## API

| Point d'accès | Description |
| --- | --- |
| `GET /api/perf/overview?range=60s\|1h\|24h` | Historique de l'hôte et du processus |
| `GET /api/perf/sites` | Métriques de chaque site sur environ les 60 dernières secondes |
| `GET /api/perf/alerts` | Objets en attente et en alerte |
| `GET /api/perf/metrics` | Métriques disponibles pour les alertes |
| `/api/perf/rules` | Créer, lire, modifier et supprimer des règles d'alerte, les activer, les ordonner et les grouper |

La lecture requiert le droit `perf:read` et la modification des règles `perf:write` ; voir [Jetons d'API](/fr/docs/reference/api).
