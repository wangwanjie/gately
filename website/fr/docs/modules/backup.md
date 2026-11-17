# Sauvegarde de la configuration et sauvegarde planifiée

Toute la configuration de Gately est stockée dans des fichiers `*.json` du répertoire de configuration. La fonction de sauvegarde empaquette ces fichiers, et permet le téléchargement manuel, l'écriture planifiée vers plusieurs emplacements et la restauration par écrasement avec une sauvegarde. L'interface d'administration se trouve dans le groupe « Système » de la barre latérale → « Paramètres » → l'onglet « Sauvegarde et migration ».

## Contenu de la sauvegarde

Une sauvegarde est un fichier zip contenant tous les `*.json` situés à la racine du répertoire de configuration (compte, règles des modules, certificats, hachages de jetons, etc.). Elle inclut les hachages de mots de passe, les secrets, les hachages des jetons d'API et les clés privées des certificats ; conservez-la donc avec soin. Les journaux d'exécution (`logs/`) et le journal d'audit (`audit.log`) ne sont pas inclus.

Le format du nom de fichier est `gately-backup-YYYYMMDD-HHMMSS.zip`, et les sauvegardes chiffrées utilisent `gately-backup-YYYYMMDD-HHMMSS.zip.enc`.

## Sauvegarde et restauration manuelles

### Télécharger une sauvegarde

Le bouton « Télécharger la sauvegarde » de la carte « Sauvegarder la configuration de Gately » télécharge un zip de la configuration actuelle. Les sauvegardes téléchargées manuellement ne sont pas chiffrées.

### Restauration

Sur la carte « Restaurer la configuration », choisissez un fichier de sauvegarde, cliquez sur « Restaurer », puis saisissez le mot « restore » dans la boîte de confirmation.

- La restauration écrase toute la configuration actuelle. La taille maximale du téléversement est de 64 Mo.
- Avant l'écrasement, la configuration existante est copiée dans un dossier `backup-<time>` du répertoire de configuration (format de l'heure `YYYYMMDD-HHMMSS`), et l'interface affiche ce chemin après une restauration réussie.
- Gately redémarre automatiquement après la restauration.
- Par défaut, la configuration du compte de la sauvegarde écrase le compte actuel, et une sauvegarde sans `account.json` est rejetée. Si vous cochez « Conserver le compte administrateur actuel », le compte n'est pas écrasé : le compte actuel, le mot de passe, l'authentification à deux facteurs, les comptes en lecture seule, les jetons d'API et la connexion tierce de l'interface d'administration restent inchangés, et la sauvegarde peut ne pas contenir `account.json` ; pratique pour importer les règles d'une autre machine.
- Lorsque la configuration du compte est restaurée, une nouvelle clé de session est utilisée et toutes les sessions de connexion émises avant la restauration deviennent invalides.
- Tant que le mot de passe par défaut est utilisé, la restauration est rejetée ; il faut d'abord modifier le mot de passe administrateur.
- Seuls les fichiers `*.json` situés à la racine du zip, non cachés et contenant du JSON valide sont acceptés. Toutes les autres entrées sont ignorées.
- Lorsque vous choisissez un fichier `.enc`, un champ « Phrase secrète de la sauvegarde » apparaît. Saisissez la phrase secrète définie lors de la création de la sauvegarde. Une phrase secrète erronée ou un fichier endommagé fait échouer la restauration.

Les API de téléchargement et de restauration des sauvegardes n'acceptent que les sessions de connexion, et les [jetons d'API](/fr/docs/reference/api) ne peuvent pas les appeler.

## Sauvegarde planifiée

Sur la carte « Sauvegarde planifiée » sous l'onglet « Sauvegarde et migration », l'interrupteur en haut à droite active ou désactive la fonction. L'ID du module est `backup`, et les scopes de jeton sont `backup:read` et `backup:write`.

| Réglage | Description |
| --- | --- |
| Planification | Une expression cron à 5 champs, `0 3 * * *` par défaut (tous les jours à 03:00). Les descripteurs tels que `@daily` et `@weekly` fonctionnent aussi |
| Rétention | Conserve les N dernières sauvegardes pour chaque destination, de 1 à 365, 7 par défaut |
| Phrase secrète de chiffrement | Laissez vide pour ne pas chiffrer ; une fois définie, des sauvegardes `.zip.enc` sont produites. Si une phrase secrète est déjà définie, laisser le champ vide la conserve inchangée |
| Répertoire local | Interrupteur, activé par défaut. Écrit dans le dossier `backups` du répertoire de configuration |

Cliquez sur « Enregistrer les paramètres » pour appliquer la planification, et sur « Sauvegarder maintenant » pour en lancer une immédiatement. La carte affiche l'heure de la dernière exécution, son mode de déclenchement (manuel ou planifié), l'heure de la prochaine exécution, le résultat (succès ou échec) pour chaque destination et le nombre d'anciennes sauvegardes supprimées. Les fichiers de sauvegarde locaux sont listés dans la carte et peuvent être téléchargés.

### Destinations de sauvegarde

Outre le répertoire local, vous pouvez ajouter plusieurs destinations distantes. Chacune écrit vers un montage de « Montages de stockage » (un répertoire local, un WebDAV distant ou un partage SMB) :

| Champ | Description |
| --- | --- |
| Nom | Obligatoire |
| Montage de stockage | Choisissez un montage existant dans « Montages de stockage ». Les montages en lecture seule ou désactivés ne peuvent pas être sélectionnés |
| Répertoire | Sous-répertoire dans le montage ; laissez vide pour la racine |
| Activé | Interrupteur |

Si vous n'avez pas encore de montage, ajoutez-en d'abord un dans « Montages de stockage ». Supprimer une destination ne supprime pas les sauvegardes déjà écrites.

### Déroulement de l'exécution

1. Empaqueter le répertoire de configuration, et le chiffrer si une phrase secrète est définie.
2. Écrire dans le répertoire local (s'il est activé) et vers chaque destination activée, avec le même nom de fichier partout.
3. Après une écriture réussie, supprimer les sauvegardes plus anciennes de cet emplacement selon le nombre de rétention. L'élagage ne touche que les fichiers dont le nom commence par `gately-backup-` et se termine par `.zip` ou `.zip.enc` ; les autres fichiers restent intacts.
4. Résumer les résultats, les écrire dans le journal du module et publier un événement de notification.

Si ni le répertoire local ni aucune destination n'est activé, la sauvegarde est enregistrée comme échouée. Une seule sauvegarde s'exécute à la fois.

### Événements de notification

| Événement | Niveau | Déclencheur |
| --- | --- | --- |
| `backup.done` | Info | Tous les emplacements ont été écrits avec succès ; le message indique le nom du fichier et le résultat pour chaque emplacement |
| `backup.failed` | Erreur | L'empaquetage a échoué ou l'écriture vers un emplacement a échoué ; le message indique le résultat pour chaque emplacement |

Créez une règle abonnée à `backup.*` dans le [Centre de notifications](/fr/docs/modules/notify) pour recevoir les résultats des sauvegardes via Telegram, e-mail et d'autres canaux.

## Chiffrement

Lorsqu'une phrase secrète est définie, les sauvegardes sont chiffrées ainsi :

- Dérivation de clé : scrypt (N=32768, r=8, p=1), produisant une clé de 32 octets, avec un sel aléatoire de 16 octets.
- Chiffrement : AES-256-GCM, avec un nonce aléatoire de 12 octets.
- Format de fichier : le magic `GATELYBK1`, le sel, le nonce et le texte chiffré concaténés dans cet ordre. Le magic sert aussi de données authentifiées additionnelles pour GCM.

La phrase secrète est stockée uniquement dans `backup.json` du répertoire de configuration. Si la phrase secrète est perdue, la sauvegarde ne peut pas être déchiffrée. La phrase secrète ne peut être consultée et modifiée qu'après connexion à l'interface d'administration ; les jetons d'API ne peuvent ni la lire, ni la modifier, ni l'effacer, mais les autres réglages de sauvegarde restent modifiables avec un jeton.

::: warning Remarque
Lorsque `backup.json` et les fichiers de sauvegarde se trouvent sur la même machine, la protection par phrase secrète apporte peu. Écrivez les sauvegardes chiffrées sur un montage situé sur un autre appareil, et notez la phrase secrète séparément.
:::
