# Paquet de diagnostic

Quand Gately se comporte mal, exportez un paquet de diagnostic depuis **Paramètres → Diagnostic** et joignez-le à votre [ticket GitHub](https://github.com/wangwanjie/gately/issues/new). Il aide les mainteneurs à trouver la cause sans vous demander de captures d'écran ni de journaux copiés-collés.

Le paquet est généré sur votre propre machine et n'est jamais envoyé nulle part. Cliquez sur **Prévisualiser le contenu** avant de le télécharger pour voir exactement ce qui sera exporté. Le zip ne contient que du texte brut et du JSON : vous pouvez aussi le décompresser et relire chaque fichier.

## Ce qui est exporté

| Fichier | Contenu |
| --- | --- |
| `README.txt` | Une description bilingue du paquet |
| `summary.json` | Version et informations de build de Gately, système d'exploitation et architecture, modèle de CPU et nombre de cœurs, totaux de mémoire / disque, durée de fonctionnement, statistiques du runtime Go, numéros de port de l'interface d'administration, **noms** des interfaces réseau avec les **catégories** d'adresses (par exemple `private: 1, public: 2`) |
| `modules.json` | Liste des modules et résumé d'état affiché dans la page de vue d'ensemble |
| `config/*.json` | **Structure** de la configuration : noms de champs, nombres, interrupteurs et valeurs énumérées comme le protocole et le mode |
| `logs/*.log` | Journaux d'exécution récents du système et de chaque module, y compris ceux enregistrés sur disque avant un redémarrage (désactivable) |
| `logs/crash.log` | Enregistrement des arrêts anormaux : message et piles des plantages du runtime Go (panic, fatal error), et heure approximative à laquelle le processus a été tué (par exemple par l'OOM killer faute de mémoire) ; présent uniquement après un arrêt anormal (désactivable) |

## Ce qui n'est jamais exporté

- Mots de passe, empreintes de mots de passe, jetons d'API, secrets TOTP, clés de session, secrets client OAuth, clés des fournisseurs DNS, mots de passe et clés privées SSH, certificats et clés privées. `account.json` et `tokens.json` ne sont jamais lus.
- Noms de domaine, noms d'hôte, adresses IP réelles et adresses MAC. Ils sont remplacés par des marqueurs comme `<domain>`, `<ipv4:private>`, `<ipv4:public>` et `<mac>` ; les ports sont conservés.
- Le chemin du répertoire de configuration et votre répertoire personnel (remplacés par `<config-dir>` et `<home>`).
- Les noms, remarques et tout autre texte saisi dans la configuration : toute chaîne qui n'est pas une valeur énumérée connue devient `<redacted>`.
- Le contenu des fichiers, les conteneurs, images et variables d'environnement Docker, les sessions de terminal et le contenu du trafic.

## Fonctionnement du masquage

- **Configuration** : les valeurs sont retirées par défaut. Seuls subsistent les noms de champs, les nombres, les booléens, une liste fixe de champs énumérés (`protocol`, `mode`, `provider`, …) et les horodatages. Les champs d'adresse ne gardent que le port et la catégorie d'IP.
- **Journaux** : les adresses IP, domaines, e-mails, URL, paires du type `password=…`, jetons `Bearer`, longues chaînes ressemblant à des jetons, adresses MAC et répertoires d'utilisateur sont masqués. Les domaines de services publics comme `github.com` et `letsencrypt.org` sont conservés, car ils montrent si un échec vient du réseau.
- Un nom que vous avez saisi (par exemple la remarque d'une règle) peut encore apparaître dans une ligne de journal s'il ne ressemble ni à une IP ni à un domaine. Parcourez `logs/*.log` avant de publier et retirez tout ce que vous ne voulez pas partager.

## Diagnostiquer un arrêt anormal

Pendant son exécution, Gately écrit les détails des plantages dans `crash.log` dans le répertoire de configuration. Après un arrêt inattendu, le journal système du démarrage suivant indique si la dernière exécution a planté ou a été tuée :

- **Plantage** : `crash.log` contient la cause et les piles de toutes les goroutines ; exportez le paquet de diagnostic et joignez-le à l'issue. Pour les erreurs fatales comme `concurrent map writes`, Go n'écrit la ligne `fatal error: …` que sur la sortie d'erreur standard, visible avec `docker logs`.
- **Arrêt inattendu** : le processus n'a rien laissé, en général parce que le système l'a tué faute de mémoire (OOM), qu'il a été arrêté par `kill -9` / `docker kill` ou qu'il y a eu une coupure de courant. Lancez `dmesg -T | grep -i -E "oom|killed process"` sur l'hôte pour vérifier un OOM.

## API

`GET /api/system/diagnostics` (session de connexion uniquement, les jetons d'API sont refusés). `?logs=0` exclut les journaux et `?preview=1` renvoie les fichiers en JSON.
