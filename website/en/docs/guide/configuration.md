# Configuration & Runtime Options

## Command-line flags

| Flag | Description |
| --- | --- |
| `-c` | Config directory; can also be set with the `GATELY_CONFIG_DIR` environment variable |
| `-listen` | Override the admin UI listen address, e.g. `:16680` |
| `-apply` | Declarative config directory; can also be set with the `GATELY_APPLY_DIR` environment variable. See below |
| `-gateway-socket` | Also serve the admin UI on this Unix socket, for example for the fnOS unified gateway |
| `-gateway-prefix` | Path prefix used when accessed through the gateway, for example `/app/gately` |
| `-api-docs` | Export the API docs (`openapi.json` and `API.md`) to the given directory and exit |
| `-version` | Print version information |

| Environment variable | Description |
| --- | --- |
| `GATELY_INITIAL_PASSWORD` | Initial administrator password on first start (at least 8 characters); defaults to `gately.cn` when unset; has no effect once the account exists. If it is shorter than 10 characters or too weak, the first sign-in asks you to change it |
| `GATELY_LANG` | Language code read first when the server language is "Follow system"; `LC_ALL`, `LC_MESSAGES` and `LANG` follow in that order |
| `GATELY_LOG_LEVEL` | Lowest level of runtime logs that is recorded: `debug`, `info`, `warn` or `error`. Applies when "Log level" in the settings is "Default"; `info` when not set |

## Config directory

All configuration lives in the config directory, with one plain-text `<name>.json` file per module, written atomically with file mode 0600. This means you can:

- Inspect it with any text editor;
- Put the directory in git, audit every change with diffs and roll back at any time;
- Back up by simply copying the directory.

::: warning Note
`ssl.json` contains certificate private keys. Before committing it to version control, make sure the repository is private, or add the file to your ignore list.
:::

## Declarative configuration

With `-apply <dir>`, every start copies the `<name>.json` files in that directory over the config directory; files with identical content are not rewritten. This suits distributing configuration from a git repository, a Kubernetes ConfigMap or a deployment script.

- Each file is validated as JSON before writing; if any file is invalid, Gately refuses to start;
- `account.json` and `tokens.json` cannot be overwritten; accounts and tokens can only be managed in the admin UI;
- The overwritten content is saved in the [config history](/en/docs/guide/audit) and can be rolled back at any time.

## External program directory

Tailscale, frp, cloudflared and File Browser supervise the official external programs. Gately looks for each executable in this order:

1. The path set in the module settings;
2. The `bin/` subdirectory of the config directory;
3. `PATH`.

Each module page can install, update and roll back its program online. Programs are installed in `bin/` and older versions are kept in `bin/.versions/`; see [Online install, update and rollback](/en/docs/modules/tunnels#install). You can also put the binaries into the config directory yourself, for example `./gately-data/bin/frpc` in Docker deployments; Tailscale needs both `tailscale` and `tailscaled`.

## Runtime logs

System and module runtime logs are written to `logs/` in the configuration directory by default, so they are still available on the "System Logs" tab of "System → Logs" and on each module page after a restart or upgrade:

- One JSON Lines file per source: `logs/<module ID>.log`, with system logs in `logs/gately.log`. On startup the most recent entries, up to "Log retention lines", are restored from the file;
- New entries are written in batches at most 1 second later. A file is rotated to `.1` once it exceeds the limit (2 MB by default) and only that one previous file is kept, so each source uses at most twice the limit;
- Web Service access logs are large and often reviewed later, so they are archived by day: when the current file crosses into a new day (or exceeds a quarter of the total size, at most 16 MB), it is renamed to `logs/webservice.<start time>.log` and compressed to `.log.gz` in the background; the oldest archives are deleted according to "Access log retention (days)" (7 by default) and "Access log total size (MB)" (the current file plus all compressed archives, 32 MB by default and at most 5% of the partition holding the logs);
- Each module records the log sources it has seen in `logs/<module ID>.sources`, kept across restarts;
- Under Settings → Network & Runtime → Runtime parameters you can turn off "Persist logs", change "Log file limit", "Access log retention (days)" and "Access log total size (MB)", or move "Log directory" to another absolute path (for example a directory on an external disk; it is created if missing and accessible only to the user Gately runs as), effective immediately. After the log directory changes, new logs go to the new directory and files in the old one are left in place. When persistence is off, logs are kept in memory only and cleared on restart;
- "Log level" decides which runtime logs are recorded. The default is "Info and above": debug logs are not recorded and do not reach the log pages, log files or standard error. One line per connection for port forwarding and protocol routing, client disconnects and TLS handshake failures for unrelated domains are debug level; to see them, change "Log level" to "Debug and above (everything)". It takes effect as soon as it is saved and adds noticeable overhead under many connections. Web Services access logs are info level and are no longer recorded at "Warning and above" or "Errors only". The `GATELY_LOG_LEVEL` environment variable sets the level too; a specific level chosen in the settings takes precedence;
- Runtime logs are also echoed to standard error (what `docker logs` and `journalctl` show), in the server language. The echo is written in batches in the background and is completed before the process exits normally or restarts; when standard error is blocked or too slow and more than 1 MB is pending, new echo lines are dropped and one line reports how many, while the log pages and log files are not affected. Docker's default logging driver does not limit the size of container logs; the deployment example [docker-compose.yml](https://github.com/wangwanjie/gately/blob/main/deploy/docker-compose.yml) sets a limit with `max-size` and `max-file` under `logging.options`;
- Clearing a module's logs in the UI also deletes its files, rotated file, compressed archives and source list;
- Configuration backups do not include `logs/` or the audit log `audit.log`.

### Viewing and filtering {#log-filter}

The "System Logs" tab of the Logs page and the log card at the bottom of each module page use the same log viewer. The following conditions can be combined; changes apply immediately, and new entries are appended only when they match:

- Level and source (the dropdown lists every source in the module configuration plus sources that have appeared in the logs, such as all Web Service listeners, including ones with no access yet or whose logs are only on disk; you can also type one);
- Keywords: space-separated terms must all match, double quotes group a phrase, a leading `-` excludes a term, for example `GET "timeout" -debug`; matches are highlighted. When the interface is in another language, search with the translated text you see on screen;
- Exclude: entries containing any of its terms are hidden, combined with the keywords; terms are written the same way (space-separated, double quotes for phrases), for example `debug "health check"`. To exclude a term that itself starts with `-`, wrap it in double quotes, such as `"-v"`;
- IP under More filters: a full address, a range (such as `10.0.0.0/8`) or part of an address, comma-separated; when an entry has no separate IP field, addresses are recognised in the log text, so sign-in failures and intrusion protection logs work too; prefix `!` or `-` to exclude, e.g. `!1.2.3.4, !10.0.0.0/8` hides those sources, and it can be combined with included items (`10.0.0.0/8, !10.0.0.5`);
- Time range under More filters: from the last 15 minutes to the last 7 days, or a custom start and end;
- The Web Service toolbar also has two display switches that only affect what is shown (the logs are still kept, and the choice is remembered in the browser): Hide banned sources hides requests rejected because the source is banned (behind a CDN, kernel bans do not apply to the CDN's connections, so banned visitors are still rejected at the Web layer and leave 403 / 444 entries), and also every request from sources that are still banned, including the requests that triggered the ban (they show again once the ban is lifted or expires); Hide console requests hides requests carrying an admin console session of this instance, such as the log page polling when you open the console through a Web Service site, and is on by default. These entries carry a "Blocked by=Banned" or "Console user=…" tag, and the API parameters are `hideBanned=1` and `hideAdmin=1` (only administrator sessions count, read-only accounts do not; blocked requests never carry the console user tag);
- Web Service logs can also be filtered by site name or domain and by status code (`404`, `4xx`, `500-599`, comma-separated); WAF Firewall logs can be filtered by domain, and match entries record the domain and rule ID.

Click a source in the log, or a tag such as "Site=…" or "Domain=…" at the end of a line, to filter by that value; in Web service logs, clicking "Site=…" matches the site name exactly and the toolbar shows a removable "Site: …" tag (API parameter `site`), so sites with similar names stay out of the results. The API also accepts a `listener` parameter (the listener key, matched exactly); when two listeners share a name, their log source names get the listen address, such as `HTTPS (0.0.0.0:443)` and `HTTPS ([::]:443)`, while unique names stay unchanged. When the entries in memory are not enough, Load earlier logs continues into the history kept in the log files (including Web Service compressed archives; archives outside a time range filter are skipped), so with log persistence on, filtering is not limited by the number of retained log lines. `GET /api/logs` and `GET /api/<module>/logs` accept the same filter parameters; see the [API reference](/en/docs/reference/api-reference).

The log area can be resized: drag the bar below it (on touch screens, press and drag), double-click the bar to restore the default height, or use the arrow keys when the bar has focus. Log cards on pages and the site log dialog remember their heights separately, in the current browser. The full-screen button at the right of the toolbar lets the log fill the whole window; click it again or press Esc to exit. While you scroll up through history, new logs are still appended while the view stays where you are; a "Jump to latest" button appears in the lower-right corner of the log area and shows how many new logs have arrived. Scrolling back to the bottom or clicking the button resumes auto scrolling.

### Export {#log-export}

"Export" in the log toolbar exports every entry matching the current filters, including the history kept in log files and compressed archives, oldest first, and the browser downloads it directly:

- CSV (spreadsheet): UTF-8 with BOM, opens directly in Excel; columns are time, level, source, message, and fields such as IP, domain, site and status code; cells starting with `=`, `+`, `-` or `@` get a leading single quote so spreadsheet software never runs paths submitted by visitors as formulas;
- JSON Lines: one entry per line, with the same fields as the logs API;
- Messages are exported in the interface language; read-only accounts and API tokens with only read permission can also export, and passwords and tokens in addresses are hidden just as when viewing;
- The API is `GET /api/logs/export` and `GET /api/<module>/logs/export`, with `format` set to `csv` or `jsonl` and the other filter parameters the same as for viewing logs; the source list API is `GET /api/logs/sources` and `GET /api/<module>/logs/sources`.

## Confirm after saving and automatic rollback {#confirm}

When you manage Gately remotely through a domain, changing port forwarding, protocol routing, the web service or global settings can cut off the very path the admin UI uses. So after saving these, the admin UI checks over a freshly opened connection that it can still reach Gately:

- If access works, it is confirmed silently in the background;
- If it can't be confirmed, a countdown dialog appears; it confirms on its own once access comes back, or you can click "Roll back now";
- If it still isn't confirmed when time runs out, Gately restarts and restores the previous configuration on startup. The changed version is kept in [Config History](/en/docs/guide/audit). The page reloads once Gately is back and tells you which changes were rolled back.

Confirmation uses a WebSocket: browsers always open a new connection for it, so it goes through the changed forwarding and listeners. Long-lived connections opened before the change still reach the old target after a hot update and prove nothing about the new configuration.

Settings that only take effect after a restart (admin UI listen addresses, module switches) are recorded when you restart from the admin UI, and the timer starts after the restart. If Gately starts 3 times in a row without a confirmation (for example the new configuration keeps it from running), the change is rolled back too. If you open the admin UI at a new address while a change is pending, a dialog asks you to "Keep changes" or "Roll back now".

Adjust the time limit under "Settings → Access Control → Console listener → Confirm after saving" (30–600 seconds, 90 by default), or turn it off. It only applies to saves from the admin UI; changes made with API tokens and scripts are not affected. The API is `GET /api/system/confirm`.

### Warning before saving: current access path {#access-path}

Gately can tell which of its own forwarding your connection to the admin UI passes through: port forwarding and protocol routing record a hop when they connect to a target on this machine, and the web service adds signed path information when it proxies to the admin UI (it can't be forged from outside). The "Current access path" card at the top of "Settings → Access Control" shows the path, for example `Visitor → Protocol routing "vps-443" → Web service "main" › site "admin.example.com" → Admin UI`. Programs on this machine such as frp or Cloudflare Tunnel show as "This machine".

Before saving, the admin UI asks the server whether the change affects this path. If it does, a dialog lists why and highlights the affected hop; you can cancel or "Save anyway":

- changing forwarding settings of a rule, listener or site on the path (renaming, grouping or remarks don't count), disabling or deleting it, or moving the site to another listener;
- turning off a module on the path (its global switch or the "Modules" setting);
- changing the admin UI listen address you are using;
- turning on "LAN access only" while connecting from outside the LAN, or "Force HTTPS" while on HTTP.

Besides the warning, confirmation and automatic rollback above still apply, so paths Gately can't see (for example another reverse proxy in front) are protected too. The API is `GET /api/system/access-path` and `POST /api/system/access-path/check`.

## Backup and restore

"Settings" provides configuration backup and restore, and is also the entry point for importing Lucky configurations.

When restoring, you can check "Keep the current administrator account" (API parameter `keepAccount=1`): `account.json`, `tokens.json` and the third-party login entries of the system settings in the backup are ignored, the current account, password, two-step verification, read-only users, API tokens and third-party login to the admin UI stay unchanged while the other settings come from the backup, and a backup without `account.json` can also be restored, which suits moving the configuration of another instance over. Restoring is refused while the default password is in use; change the password first.
