# Configuration et options d'exécution

## Options de ligne de commande

| Option | Description |
| --- | --- |
| `-c` | Dossier de configuration ; peut aussi être défini avec la variable d'environnement `GATELY_CONFIG_DIR` |
| `-listen` | Remplace l'adresse d'écoute de l'interface d'administration, p. ex. `:16680` |
| `-apply` | Dossier de configuration déclarative ; peut aussi être défini avec la variable d'environnement `GATELY_APPLY_DIR`, voir ci-dessous |
| `-gateway-socket` | Sert aussi l'interface d'administration sur ce socket Unix, par exemple pour la passerelle unifiée de fnOS |
| `-gateway-prefix` | Préfixe de chemin utilisé lors d'un accès via la passerelle, par exemple `/app/gately` |
| `-api-docs` | Exporte la documentation de l'API (`openapi.json` et `API.md`) dans le dossier indiqué, puis quitte |
| `-version` | Affiche les informations de version |

| Variable d'environnement | Description |
| --- | --- |
| `GATELY_INITIAL_PASSWORD` | Mot de passe administrateur initial au premier démarrage (8 caractères minimum) ; à défaut, le mot de passe par défaut `gately.cn` est utilisé. Sans effet si le compte existe déjà |
| `GATELY_LANG` | Code de langue lu en premier lorsque la langue du serveur est « Suivre le système » ; `LC_ALL`, `LC_MESSAGES` et `LANG` viennent ensuite, dans cet ordre |

## Dossier de configuration

Toute la configuration réside dans le dossier de configuration, avec un fichier texte `<nom>.json` par module, écrit de façon atomique avec les droits 0600. Vous pouvez donc :

- l'inspecter avec n'importe quel éditeur de texte ;
- placer le dossier dans git, auditer chaque changement avec des diff et revenir en arrière à tout moment ;
- le sauvegarder en copiant simplement le dossier.

::: warning Attention
`ssl.json` contient les clés privées des certificats. Avant de le valider dans un gestionnaire de versions, assurez-vous que le dépôt est privé, ou ajoutez le fichier à votre liste d'exclusion.
:::

## Configuration déclarative

Avec `-apply <dossier>`, chaque démarrage copie les fichiers `<nom>.json` de ce dossier par-dessus le dossier de configuration ; les fichiers au contenu identique ne sont pas réécrits. C'est pratique pour distribuer la configuration depuis un dépôt git, un ConfigMap Kubernetes ou un script de déploiement.

- Chaque fichier est vérifié comme JSON valide avant l'écriture ; si un seul fichier est invalide, Gately refuse de démarrer ;
- `account.json` et `tokens.json` ne peuvent pas être écrasés : le compte et les jetons se gèrent uniquement dans l'interface d'administration ;
- L'ancien contenu écrasé est conservé dans l'[historique de configuration](/fr/docs/guide/audit) et peut être restauré à tout moment.

## Dossier des programmes externes

Tailscale, frp, cloudflared et File Browser supervisent les programmes externes officiels. Gately cherche chaque exécutable dans cet ordre :

1. Le chemin défini dans les paramètres du module ;
2. Le sous-dossier `bin/` du dossier de configuration ;
3. `PATH`.

Chaque page de module permet l'installation, la mise à jour et le retour arrière en ligne : les programmes sont installés dans `bin/` et les anciennes versions conservées dans `bin/.versions/`. Voir [Installation, mise à jour et retour arrière en ligne](/fr/docs/modules/tunnels#install). Vous pouvez aussi placer les binaires vous-même dans le dossier de configuration, avec Docker par exemple `./gately-data/bin/frpc` ; Tailscale nécessite à la fois `tailscale` et `tailscaled`.

## Journaux d'exécution

Les journaux d'exécution du système et de chaque module sont écrits par défaut dans `logs/` du répertoire de configuration ; ils restent donc consultables dans la page des journaux système et dans chaque module après un redémarrage ou une mise à jour :

- Un fichier JSON Lines par source : `logs/<ID du module>.log`, et `logs/gately.log` pour le système. Au démarrage, les entrées les plus récentes, dans la limite de « Lignes de journal conservées », sont restaurées depuis le fichier ;
- Les nouvelles entrées sont écrites par lots, au plus 1 seconde plus tard. Un fichier qui dépasse la limite (2 Mo par défaut) est renouvelé en `.1` et seul ce fichier précédent est conservé : chaque source occupe au plus le double de la limite ;
- Les journaux d'accès du Service Web, volumineux et souvent consultés après coup, sont archivés par jour : quand le fichier courant change de jour (ou dépasse le quart de la taille totale, 16 Mo au plus), il est renommé en `logs/webservice.<heure de début>.log` puis compressé en arrière-plan en `.log.gz` ; les archives les plus anciennes sont supprimées selon « Conservation des journaux d'accès (jours) » (7 par défaut) et « Taille totale des journaux d'accès (Mo) » (fichier courant et toutes les archives compressées, 32 Mo par défaut et au plus 5 % de la partition qui contient les journaux) ;
- Chaque module enregistre les sources de journal apparues dans `logs/<ID du module>.sources`, conservé après un redémarrage ;
- Dans Paramètres → Réseau et exécution → Paramètres d'exécution, vous pouvez désactiver « Conserver les journaux sur disque », modifier « Limite du fichier journal », « Conservation des journaux d'accès (jours) » et « Taille totale des journaux d'accès (Mo) », ou déplacer « Répertoire des journaux » vers un autre chemin absolu (par exemple un répertoire sur un disque externe ; créé s'il n'existe pas, accessible uniquement à l'utilisateur sous lequel tourne Gately), avec effet immédiat. Après un changement de répertoire, les nouveaux journaux vont dans le nouveau répertoire et les fichiers de l'ancien restent en place. Désactivés, les journaux restent uniquement en mémoire et sont effacés au redémarrage ;
- Effacer les journaux d'un module dans l'interface supprime aussi ses fichiers, le fichier renouvelé, les archives compressées et la liste des sources ;
- Les sauvegardes de configuration n'incluent ni `logs/` ni le journal d'audit `audit.log`.

### Consulter et filtrer {#log-filter}

La page Journaux système et la carte de journaux en bas de chaque page de module utilisent le même visionneur. Les conditions suivantes se combinent ; les modifications s'appliquent immédiatement et seules les nouvelles entrées correspondantes sont ajoutées :

- Niveau et source (la liste déroulante propose toutes les sources de la configuration du module et celles apparues dans les journaux, par exemple tous les écouteurs du Service Web, y compris ceux encore sans accès ou dont les journaux sont seulement sur disque ; la saisie directe reste possible) ;
- Mots-clés : les termes séparés par des espaces doivent tous correspondre, les guillemets doubles regroupent une expression, un `-` initial exclut un terme, par exemple `GET "timeout" -debug` ; les correspondances sont surlignées. Si l'interface est dans une autre langue, recherchez avec le texte traduit affiché à l'écran ;
- Exclure : les entrées contenant l'un des termes sont masquées, en plus des mots-clés ; même syntaxe (termes séparés par des espaces, guillemets doubles pour une expression), par exemple `debug "health check"`. Pour exclure un terme qui commence lui-même par `-`, mettez-le entre guillemets doubles, comme `"-v"` ;
- IP dans Plus de filtres : adresse complète, plage (comme `10.0.0.0/8`) ou partie d'adresse, séparées par des virgules ; lorsqu'une entrée n'a pas de champ IP propre, les adresses sont reconnues dans le texte, ce qui couvre aussi les échecs de connexion et les journaux de protection contre les intrusions ; le préfixe `!` ou `-` exclut, par exemple `!1.2.3.4, !10.0.0.0/8` masque ces sources, et il peut être combiné avec des éléments inclus (`10.0.0.0/8, !10.0.0.5`) ;
- Période dans Plus de filtres : des 15 dernières minutes aux 7 derniers jours, ou un début et une fin personnalisés ;
- La barre d'outils du Service Web propose aussi deux interrupteurs d'affichage qui n'agissent que sur ce qui est montré (les journaux sont conservés et le choix est mémorisé dans le navigateur) : Masquer les sources bannies masque les requêtes refusées parce que la source est bannie (derrière un CDN, les bannissements du noyau ne s'appliquent pas aux connexions du CDN, les visiteurs bannis sont donc refusés au niveau web et laissent des entrées 403 / 444), ainsi que toutes les requêtes des sources encore bannies, y compris celles qui ont déclenché le bannissement (elles réapparaissent quand le bannissement est levé ou expire) ; Masquer les requêtes de la console masque les requêtes portant une session de la console d'administration de cette instance, comme l'interrogation de la page des journaux lorsque vous ouvrez la console via un site du Service Web, et est activé par défaut. Ces entrées portent l'étiquette « Bloqué par=Banni » ou « Utilisateur de la console=… », et les paramètres de l'API sont `hideBanned=1` et `hideAdmin=1` (seules les sessions administrateur comptent, pas les comptes en lecture seule ; les requêtes bloquées ne portent jamais l'étiquette d'utilisateur de la console) ;
- Les journaux du Service Web se filtrent aussi par nom de site ou domaine et par code d'état (`404`, `4xx`, `500-599`, séparés par des virgules) ; ceux du Pare-feu WAF par domaine, et les détections enregistrent le domaine et l'ID de la règle.

Cliquez sur une source dans le journal, ou sur une étiquette comme « Site=… » ou « Domaine=… » en fin de ligne, pour filtrer sur cette valeur ; dans les journaux du Service Web, « Site=… » filtre sur le nom du site avec une correspondance exacte et la barre d'outils affiche une étiquette « Site : … » amovible (paramètre d'API `site`), de sorte que les entrées de sites aux noms proches ne se mélangent pas. L'API accepte aussi un paramètre `listener` (clé de l'écouteur, correspondance exacte) ; quand deux écouteurs portent le même nom, leurs noms de source dans les journaux reçoivent l'adresse d'écoute, par exemple `HTTPS (0.0.0.0:443)` et `HTTPS ([::]:443)`, et les noms uniques restent inchangés. Quand les entrées en mémoire ne suffisent pas, Charger les journaux plus anciens poursuit dans l'historique conservé dans les fichiers journaux (y compris les archives compressées du Service Web ; avec un filtre de période, les archives hors période sont ignorées) : avec la persistance des journaux activée, le filtrage n'est donc pas limité par le nombre de lignes conservées. `GET /api/logs` et `GET /api/<module>/logs` acceptent les mêmes paramètres de filtre ; voir la [référence de l'API](/fr/docs/reference/api-reference).

La hauteur de la zone des journaux est réglable : faites glisser la barre située en dessous (sur écran tactile, maintenez puis faites glisser), double-cliquez sur la barre pour rétablir la hauteur par défaut, ou utilisez les flèches quand la barre a le focus. Les cartes de journaux des pages et la boîte de dialogue des journaux du site mémorisent leur hauteur séparément, dans le navigateur actuel. Le bouton plein écran à droite de la barre d’outils fait occuper toute la fenêtre aux journaux ; cliquez à nouveau ou appuyez sur Échap pour quitter. Lorsque vous remontez dans l’historique, les nouveaux journaux continuent d’être ajoutés sans ramener la vue en bas ; « Aller au plus récent » apparaît en bas à droite de la zone avec le nombre de journaux arrivés entre-temps. Revenir en bas ou cliquer sur le bouton relance le défilement automatique.

### Exporter {#log-export}

« Exporter » dans la barre d'outils des journaux exporte toutes les entrées correspondant aux filtres actuels, y compris l'historique conservé dans les fichiers journaux et les archives compressées, de la plus ancienne à la plus récente, et le navigateur les télécharge directement :

- CSV (tableur) : UTF-8 avec BOM, s'ouvre directement dans Excel ; les colonnes sont l'heure, le niveau, la source, le message et des champs comme l'IP, le domaine, le site et le code d'état ; les cellules commençant par `=`, `+`, `-` ou `@` sont précédées d'une apostrophe afin que le tableur n'exécute jamais comme formule les chemins envoyés par les visiteurs ;
- JSON Lines : une entrée par ligne, avec les mêmes champs que l'API des journaux ;
- Les messages sont exportés dans la langue de l'interface ; les comptes en lecture seule et les jetons d'API disposant seulement de la permission de lecture peuvent aussi exporter, et les mots de passe et jetons présents dans les adresses sont masqués comme à l'affichage ;
- L'API est `GET /api/logs/export` et `GET /api/<module>/logs/export`, avec `format` à `csv` ou `jsonl` et les autres paramètres de filtre identiques à ceux de la consultation des journaux ; l'API de la liste des sources est `GET /api/logs/sources` et `GET /api/<module>/logs/sources`.

## Sauvegarde et restauration

« Paramètres » propose la sauvegarde et la restauration de la configuration, et sert aussi de point d'entrée pour importer des configurations Lucky.
