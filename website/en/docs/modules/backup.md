# Configuration Backup and Scheduled Backup

All of Gately's configuration is stored in `*.json` files in the configuration directory. The backup feature packs these files, and supports manual download, scheduled writes to multiple locations, and restoring by overwriting with a backup. The management UI is under the sidebar "System" group → "Settings" → the "Backup & Migration" tab.

## Backup contents

A backup is a zip file containing every `*.json` at the root of the configuration directory (account, module rules, certificates, token hashes and so on). It includes password hashes, secrets, API token hashes and certificate private keys, so store it carefully. Runtime logs (`logs/`) and the audit log (`audit.log`) are not included.

The file name format is `gately-backup-YYYYMMDD-HHMMSS.zip`, and encrypted backups use `gately-backup-YYYYMMDD-HHMMSS.zip.enc`.

## Manual backup and restore

### Download a backup

The "Download backup" button on the "Back up Gately configuration" card downloads a zip of the current configuration. Manually downloaded backups are not encrypted.

### Restore

On the "Restore configuration" card, choose a backup file, click "Restore", and type the word "restore" in the confirmation dialog.

- Restoring overwrites all current configuration. The upload size limit is 64 MB.
- Before overwriting, the existing configuration is copied to a `backup-<time>` folder in the configuration directory (time format `YYYYMMDD-HHMMSS`), and the UI shows that path after a successful restore.
- Gately restarts automatically after the restore.
- The backup must contain the account configuration; a backup without it is rejected.
- Only `*.json` files located at the zip root, not hidden, and containing valid JSON are accepted from the backup. All other entries are ignored.
- When you choose a `.enc` file, a "Backup passphrase" input appears. Enter the passphrase that was set when the backup was created. A wrong passphrase or a damaged file makes the restore fail.

The backup download and restore APIs accept login sessions only, and [API tokens](/en/docs/reference/api) cannot call them.

## Scheduled backup

On the "Scheduled backup" card below the "Backup & Migration" tab, the switch at the top right turns the feature on or off. The module ID is `backup`, and the token scopes are `backup:read` and `backup:write`.

| Setting | Description |
| --- | --- |
| Schedule | A 5-field cron expression, default `0 3 * * *` (daily at 03:00). Descriptors such as `@daily` and `@weekly` also work |
| Retention | Keep the latest N backups for each destination, from 1 to 365, default 7 |
| Encryption passphrase | Leave empty for no encryption; when set, `.zip.enc` backups are produced. If a passphrase is already set, leaving the field empty keeps it unchanged |
| Local directory | Switch, on by default. Writes to the `backups` folder in the configuration directory |

Click "Save settings" to apply the schedule, and "Back up now" to run one immediately. The card shows the time of the last run, how it was triggered (manual or scheduled), the next run time, the success or failure result for each destination, and the number of old backups pruned. Local backup files are listed in the card and can be downloaded.

### Backup destinations

Besides the local directory, you can add several remote destinations. Each one writes to a mount in "Storage Mounts" (a local directory, a remote WebDAV or an SMB share):

| Field | Description |
| --- | --- |
| Name | Required |
| Storage mount | Choose an existing mount from "Storage Mounts". Read-only or disabled mounts cannot be selected |
| Directory | Subdirectory inside the mount; leave empty for the root |
| Enabled | Switch |

If you have no mounts yet, add one in "Storage Mounts" first. Deleting a destination does not delete backups already written.

### Execution flow

1. Pack the configuration directory, and encrypt it if a passphrase is set.
2. Write to the local directory (if enabled) and to each enabled destination, using the same file name everywhere.
3. After a successful write, prune older backups in that location according to the retention count. Pruning only touches files whose names start with `gately-backup-` and end with `.zip` or `.zip.enc`; other files are left as they are.
4. Summarize the results, write them to the module log and publish a notification event.

If neither the local directory nor any destination is enabled, the backup is recorded as failed. Only one backup runs at a time.

### Notification events

| Event | Level | Trigger |
| --- | --- | --- |
| `backup.done` | Info | All locations were written successfully; the message lists the file name and the result for each location |
| `backup.failed` | Error | Packaging failed or writing to any location failed; the message lists the result for each location |

Create a rule that subscribes to `backup.*` in the [Notification Center](/en/docs/modules/notify) to receive backup results through Telegram, email and other channels.

## Encryption

When a passphrase is set, backups are encrypted as follows:

- Key derivation: scrypt (N=32768, r=8, p=1), producing a 32-byte key, with a 16-byte random salt.
- Encryption: AES-256-GCM, with a 12-byte random nonce.
- File format: the magic `GATELYBK1`, salt, nonce and ciphertext concatenated in that order. The magic is also used as GCM additional authenticated data.

The passphrase is stored only in `backup.json` in the configuration directory. If the passphrase is lost, the backup cannot be decrypted. The passphrase can only be viewed and changed after signing in to the admin UI; API tokens cannot read, change or clear it, while the other backup settings can still be changed with a token.

::: warning Note
When `backup.json` and the backup files sit on the same machine, passphrase protection adds limited value. Write encrypted backups to a mount on another device, and record the passphrase separately.
:::
