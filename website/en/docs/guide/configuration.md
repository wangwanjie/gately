# Configuration & Runtime Options

## Command-line flags

| Flag | Description |
| --- | --- |
| `-c` | Config directory; can also be set with the `GATELY_CONFIG_DIR` environment variable |
| `-listen` | Override the admin UI listen address, e.g. `:16680` |
| `-apply` | Declarative config directory; can also be set with the `GATELY_APPLY_DIR` environment variable. See below |
| `-version` | Print version information |

| Environment variable | Description |
| --- | --- |
| `GATELY_INITIAL_PASSWORD` | Initial administrator password on first start (at least 8 characters); defaults to `gately.cn` when unset; has no effect once the account exists |

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

System and module runtime logs are written to `logs/` in the configuration directory by default, so they are still available on the System logs page and each module page after a restart or upgrade:

- One JSON Lines file per source: `logs/<module ID>.log`, with system logs in `logs/gately.log`. On startup the most recent entries, up to "Log retention lines", are restored from the file;
- New entries are written in batches at most 1 second later. A file is rotated to `.1` once it exceeds the limit (2 MB by default) and only that one previous file is kept, so each source uses at most twice the limit;
- Web Service access logs are large and often reviewed later, so they are archived by day: when the current file crosses into a new day (or exceeds a quarter of the total size, at most 16 MB), it is renamed to `logs/webservice.<start time>.log` and compressed to `.log.gz` in the background; the oldest archives are deleted according to "Access log retention (days)" (7 by default) and "Access log total size (MB)" (the current file plus all compressed archives, 32 MB by default and at most 5% of the partition holding the logs);
- Each module records the log sources it has seen in `logs/<module ID>.sources`, kept across restarts;
- Under Settings → Network & Runtime → Runtime parameters you can turn off "Persist logs", change "Log file limit", "Access log retention (days)" and "Access log total size (MB)", or move "Log directory" to another absolute path (for example a directory on an external disk; it is created if missing and accessible only to the user Gately runs as), effective immediately. After the log directory changes, new logs go to the new directory and files in the old one are left in place. When persistence is off, logs are kept in memory only and cleared on restart;
- Clearing a module's logs in the UI also deletes its files, rotated file, compressed archives and source list;
- Configuration backups do not include `logs/` or the audit log `audit.log`.

### Viewing and filtering {#log-filter}

The System Logs page and the log card at the bottom of each module page use the same log viewer. The following conditions can be combined; changes apply immediately, and new entries are appended only when they match:

- Level and source (the dropdown lists every source in the module configuration plus sources that have appeared in the logs, such as all Web Service listeners, including ones with no access yet or whose logs are only on disk; you can also type one);
- Keywords: space-separated terms must all match, double quotes group a phrase, a leading `-` excludes a term, for example `GET "timeout" -debug`; matches are highlighted. When the interface is in another language, search with the translated text you see on screen;
- Exclude: entries containing any of its terms are hidden, combined with the keywords; terms are written the same way (space-separated, double quotes for phrases), for example `debug "health check"`. To exclude a term that itself starts with `-`, wrap it in double quotes, such as `"-v"`;
- IP under More filters: a full address, a range (such as `10.0.0.0/8`) or part of an address, comma-separated; when an entry has no separate IP field, addresses are recognised in the log text, so sign-in failures and intrusion protection logs work too; prefix `!` or `-` to exclude, e.g. `!1.2.3.4, !10.0.0.0/8` hides those sources, and it can be combined with included items (`10.0.0.0/8, !10.0.0.5`);
- Time range under More filters: from the last 15 minutes to the last 7 days, or a custom start and end;
- The Web Service toolbar also has two display switches that only affect what is shown (the logs are still kept, and the choice is remembered in the browser): Hide banned sources hides requests rejected because the source is banned (behind a CDN, kernel bans do not apply to the CDN's connections, so banned visitors are still rejected at the Web layer and leave 403 / 444 entries), and also every request from sources that are still banned, including the requests that triggered the ban (they show again once the ban is lifted or expires); Hide console requests hides requests carrying an admin console session of this instance, such as the log page polling when you open the console through a Web Service site, and is on by default. These entries carry a "Blocked by=Banned" or "Console user=…" tag, and the API parameters are `hideBanned=1` and `hideAdmin=1` (only administrator sessions count, read-only accounts do not; blocked requests never carry the console user tag);
- Web Service logs can also be filtered by site name or domain and by status code (`404`, `4xx`, `500-599`, comma-separated); WAF Firewall logs can be filtered by domain, and match entries record the domain and rule ID.

Click a source in the log, or a tag such as "Site=…" or "Domain=…" at the end of a line, to filter by that value; in Web service logs, clicking "Site=…" matches the site name exactly and the toolbar shows a removable "Site: …" tag (API parameter `site`), so sites with similar names stay out of the results. When the entries in memory are not enough, Load earlier logs continues into the history kept in the log files (including Web Service compressed archives; archives outside a time range filter are skipped), so with log persistence on, filtering is not limited by the number of retained log lines. `GET /api/logs` and `GET /api/<module>/logs` accept the same filter parameters; see the [API reference](/en/docs/reference/api-reference).

### Export {#log-export}

"Export" in the log toolbar exports every entry matching the current filters, including the history kept in log files and compressed archives, oldest first, and the browser downloads it directly:

- CSV (spreadsheet): UTF-8 with BOM, opens directly in Excel; columns are time, level, source, message, and fields such as IP, domain, site and status code; cells starting with `=`, `+`, `-` or `@` get a leading single quote so spreadsheet software never runs paths submitted by visitors as formulas;
- JSON Lines: one entry per line, with the same fields as the logs API;
- Messages are exported in the interface language; read-only accounts and API tokens with only read permission can also export, and passwords and tokens in addresses are hidden just as when viewing;
- The API is `GET /api/logs/export` and `GET /api/<module>/logs/export`, with `format` set to `csv` or `jsonl` and the other filter parameters the same as for viewing logs; the source list API is `GET /api/logs/sources` and `GET /api/<module>/logs/sources`.

## Backup and restore

"Settings" provides configuration backup and restore, and is also the entry point for importing Lucky configurations.
