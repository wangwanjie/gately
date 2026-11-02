# Docker

Gérez les conteneurs, projets Compose, images, réseaux et volumes de l'hôte via le socket Docker. Le point d'entrée est « Docker » dans la barre latérale, et la page comporte six onglets : « Conteneurs », « Compose », « Images », « Réseaux », « Volumes » et « Système ». Lorsque Docker est inaccessible, le haut de la page affiche le détail de l'erreur et un bouton raccourci pour ouvrir les paramètres.

## Activation

Lorsque Gately s'exécute dans un conteneur, montez le socket Docker de l'hôte :

```bash
-v /var/run/docker.sock:/var/run/docker.sock
```

L'adresse Docker vaut par défaut `unix:///var/run/docker.sock` et peut être modifiée dans les paramètres de l'onglet « Système ».

::: warning Remarque de sécurité
Monter `docker.sock` revient à accorder un accès de niveau root à l'hôte. Si vous n'avez pas besoin de gérer Docker, retirez ce montage et protégez l'interface d'administration (authentification à deux facteurs, entrée de sécurité, accès limité au réseau local).
:::

## Conteneurs

- La liste peut être filtrée par « Tous », « En cours » et « Arrêtés ». Les cartes affichent l'image, le réseau, les ports, les montages et l'état des mises à jour ;
- Actions : démarrer, arrêter, redémarrer, mettre en pause, reprendre, arrêt forcé (kill), renommer, supprimer (avec ou sans volumes anonymes), valider (commit) en image ;
- **Autoprotection** : Gately reconnaît son propre conteneur et son projet Compose, et refuse les opérations qui couperaient l'accès à lui-même : arrêter, forcer l'arrêt, mettre en pause ou supprimer son conteneur, et exécuter `down` sur son projet. La modification avec recréation, l'exécution de `up` sur son projet ou sa suppression doivent d'abord arrêter l'ancien conteneur ; le processus Gately s'arrêterait avec lui et ne pourrait pas terminer la recréation, elles sont donc aussi refusées et les boutons correspondants sont grisés dans l'interface. La protection s'applique aussi lorsque l'API est appelée avec un nom de conteneur ou un préfixe d'ID.
- **Mettre à jour Gately lui-même** : lors de la mise à jour de son propre conteneur, Gately récupère d'abord la nouvelle image et, seulement s'il existe une nouvelle version, crée à partir de son image actuelle un conteneur auxiliaire temporaire (`<nom du conteneur>-upgrade-<heure>`). L'auxiliaire arrête l'ancien conteneur, crée et démarre le nouveau avec la même configuration puis supprime l'ancien ; si le nouveau ne démarre pas, l'ancien est restauré. Pendant ce temps l'interface affiche un écran d'attente et se recharge au retour de Gately ; le nouveau Gately écrit la sortie de l'auxiliaire dans le journal Docker puis le supprime. Les mises à jour groupées traitent Gately en dernier et ne le mettent pas à jour si un autre conteneur a échoué. L'auxiliaire accède au socket Docker par le même montage que Gately (par exemple `/var/run/docker.sock:/var/run/docker.sock`) ; sans ce montage, la mise à jour est refusée. Ne recréez pas Gately avec `docker compose up -d`, `docker restart` ou des commandes similaires dans le shell local du Terminal web : dans un déploiement Docker, ce shell s'exécute dans le conteneur de Gately, la commande s'arrête donc avec Gately et laisse l'ancien conteneur arrêté à côté d'un nouveau portant un nom temporaire (`<préfixe d'ID>_<nom du conteneur>`). Dans ce cas, exécutez de nouveau `docker compose up -d` dans le répertoire Compose de l'hôte pour rétablir la situation. La mise à jour sur l'hôte avec `docker compose pull && docker compose up -d` fonctionne toujours comme avant.
- **Informations sensibles** : lorsqu'un compte en lecture seule ou un jeton d'API ne disposant que de `docker:read` consulte un conteneur, seuls les noms des variables d'environnement sont affichés ; les valeurs, Command, Entrypoint, les arguments de démarrage et les arguments des commandes de la liste des processus apparaissent sous la forme `********`. Les détails de l'image masquent de même les variables d'environnement, Cmd, Entrypoint et les commandes de l'historique de construction (valider un conteneur en image y écrit ces paramètres tels quels). Les administrateurs disposant de l'accès en écriture obtiennent automatiquement le texte en clair à l'ouverture du formulaire de modification.
- **Création et modification** : le formulaire couvre le nom, l'image (qui peut être téléchargée avant la création), le démarrage après création, la commande et l'Entrypoint, les variables d'environnement, les redirections de ports (TCP/UDP, avec une IP hôte facultative), les montages (dossier bind et volume, éventuellement en lecture seule), le réseau principal et les réseaux supplémentaires, l'IP fixe, les alias, la politique de redémarrage (`no`, `always`, `unless-stopped`, `on-failure` avec un nombre maximal de tentatives), le mode privilégié, l'ajout ou le retrait de capabilities, les périphériques, l'utilisateur, le répertoire de travail, le nom d'hôte, le DNS, les labels, la limite de mémoire (Mo), le nombre de CPU, le TTY et le maintien de stdin ouvert. La modification recrée le conteneur à partir de sa configuration d'origine ; les réglages non couverts par le formulaire conservent leurs valeurs d'origine, et une recréation en échec est annulée automatiquement ;
- Onglets de la page de détail :
  - Aperçu : état, code de sortie, nombre de redémarrages, état de santé, détails réseau, montages, ainsi que le projet Compose et le service auxquels il appartient ;
  - Journaux : consultation des journaux récents, avec horodatage facultatif ;
  - Terminal : ouvre un terminal interactif dans un conteneur en cours d'exécution (`/bin/sh` par défaut ; la commande et l'utilisateur peuvent être précisés ; les couleurs suivent la préférence du terminal Web) ;
  - Fichiers : parcourir les fichiers du conteneur, modifier un texte en ligne (jusqu'à 1 Mio), téléverser, télécharger (les dossiers sont empaquetés en tar.gz), renommer, créer des dossiers, supprimer. La suppression exécute `rm -rf` dans le conteneur, qui doit donc être en cours d'exécution ;
  - Surveillance : échantillons de CPU, mémoire, réseau et E/S disque, nombre de processus, ainsi que la liste des processus du conteneur.

## Compose

Les projets Compose sont agrégés à partir des labels des conteneurs tels que `com.docker.compose.project`. Ils affichent un état (en cours, partiellement en cours, arrêté) et leurs services.

- Démarrer, arrêter, redémarrer, mettre en pause, reprendre, `down` (avec ou sans volumes nommés) et consulter les journaux combinés (chaque ligne est préfixée par le nom du service) ;
- Créer un projet : collez le contenu du fichier compose ; Gately l'écrit dans `compose/<name>/compose.yaml` du répertoire de configuration et exécute up. Ces projets sont marqués « managed » ; vous pouvez modifier le fichier Compose, l'enregistrer et l'appliquer, ou supprimer le projet entier ;
- Pour les projets existants, le premier fichier compose peut être modifié en ligne (le fichier doit être accessible en écriture), avec l'option d'exécuter up immédiatement après la réécriture ;
- `up`, le téléchargement des images, les builds et la lecture/écriture des fichiers dépendent de la commande `docker compose` et de l'accès au chemin du fichier compose. Si elle est indisponible, l'interface affiche « docker compose command not found ».

## Images

- La liste affiche les tags, la taille, la date de création, le nombre de conteneurs qui référencent chaque image et son statut « dangling ». Les détails incluent l'historique des couches, les variables d'environnement, les ports exposés, etc. ;
- **Pull** : l'authentification utilise les comptes de registre définis dans les paramètres de l'onglet « Système ». Si un pull direct échoue et que des adresses de miroir sont configurées, elles sont essayées dans l'ordre et l'image est re-taguée avec son nom d'origine. La recherche sur Docker Hub est prise en charge ;
- Push, ajout de tag, suppression (avec option de forçage), build (Dockerfile en ligne ou URL Git, avec arguments de build), ainsi qu'import et export tar (format `docker save`) ;
- **Vérification des mises à jour** : vérifiez un seul conteneur ou utilisez « Tout vérifier » ; les résultats sont signalés sur chaque conteneur ;
- **Mise à niveau en un clic** : télécharge la nouvelle image et recrée le conteneur avec sa configuration existante ; une mise à niveau en échec revient automatiquement au conteneur d'origine. La sélection multiple permet les mises à niveau par lot ;
- « Nettoyer les images orphelines » supprime en un clic toutes les images sans tag et inutilisées.

## Réseaux et volumes

- Réseaux : la liste affiche le pilote, la portée, le sous-réseau, la passerelle et les conteneurs rattachés. Vous pouvez créer (pilote, sous-réseau, passerelle, réseau interne, IPv6) et supprimer des réseaux, ainsi que connecter ou déconnecter des conteneurs. Les réseaux intégrés sont protégés ;
- Volumes : affiche le pilote, le point de montage, la taille, le nombre de références et les conteneurs utilisant chaque volume. Vous pouvez créer, supprimer (avec option de forçage), exporter en tar.gz et importer depuis un tar.gz. L'export et l'import utilisent un conteneur auxiliaire alpine temporaire, et l'image est téléchargée automatiquement si elle est absente.

## Système

L'onglet « Système » contient :

- Infos sur l'hôte et Docker : version, version de l'API, OS et architecture, noyau, CPU et mémoire, pilote de stockage, répertoire de données, nombre de conteneurs et d'images, version du CLI Compose ;
- Utilisation du disque : nombre, taille et espace récupérable pour les images, conteneurs, volumes et le cache de build ;
- Nettoyage : conteneurs arrêtés, images orphelines, toutes les images inutilisées, réseaux inutilisés, volumes inutilisés (les données sont définitivement perdues), cache de build ;
- Paramètres :

| Champ | Description |
| --- | --- |
| `host` | Adresse Docker, par défaut `unix:///var/run/docker.sock` |
| `mirrors` | Liste d'adresses de miroir, essayées dans l'ordre lorsqu'un pull direct échoue. Indépendante de `registry-mirrors` dans `/etc/docker/daemon.json`, que Docker utilise automatiquement et que la page des réglages affiche en lecture seule sous « Miroirs du démon » |
| `registries` | Identifiants de registres privés (`server`, `username`, `password`) ; le mot de passe est masqué dans l'interface |
| `autoCheckHours` | Si supérieur à 0, les mises à jour des images des conteneurs sont vérifiées automatiquement à cet intervalle en heures ; 0 désactive la vérification ; les images plus récentes publient un événement `docker.update`, une seule fois par nouvelle image |

## Tâches en arrière-plan

Les opérations longues telles que pull, push, build, vérification par lot, mise à niveau par lot et Compose up s'exécutent comme tâches en arrière-plan. Le panneau des tâches en bas de la page affiche la sortie en direct et l'état (en cours, terminé, erreur, arrêté), permet de consulter l'historique et d'effacer les tâches terminées. Les tâches encore en cours sont restaurées après un rafraîchissement de la page.

Une tâche en cours peut être arrêtée avec « Arrêter » dans le panneau des tâches ou dans « Système → Historique des tâches » ; après confirmation :

- Pull, push, build d'image et vérification des mises à jour s'interrompent aussitôt, et Docker abandonne le téléchargement ou le build correspondant : aucune image à moitié téléchargée ne subsiste ;
- Les commandes Compose reçoivent d'abord un signal d'interruption (le même que Ctrl+C dans un terminal) et se terminent d'elles-mêmes ; si elles ne sont pas sorties au bout de 30 secondes, elles sont tuées de force ;
- La mise à niveau par lot s'arrête une fois le conteneur en cours traité, et les conteneurs suivants ne sont pas mis à niveau. Un conteneur en cours de recréation (de l'arrêt de l'ancien à la création et au démarrage du nouveau) ignore l'interruption et termine sa recréation ; en cas d'échec, le conteneur d'origine est restauré comme d'habitude, sans rester dans un état intermédiaire.

Après l'arrêt, la tâche passe à l'état « Arrêtée » et le journal d'exécution consigne « Tâche arrêtée ». Les tâches déjà terminées au moment de l'arrêt affichent leur résultat réel. Une tâche bloquée se nettoie avec « Effacer les terminées » une fois arrêtée. L'API correspondante est `POST /api/docker/tasks/{id}/cancel`.

## Notifications d'événements

Gately s'abonne au flux d'événements Docker et publie un événement `docker.exited` lorsqu'un conteneur se termine de façon anormale (code de sortie non nul) ou est tué par l'OOM killer. Vous pouvez configurer les canaux de réception dans le [Centre de notifications](/fr/docs/modules/notify). Un arrêt normal avec le code de sortie 0 ne déclenche pas de notification. Lorsque Docker est temporairement indisponible, l'abonnement est retenté toutes les 30 secondes.

## API

Consultez l'[API Docker](/fr/docs/reference/docker-api) pour la référence complète de l'API.
