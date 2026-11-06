# Diagnostics bundle

When Gately misbehaves, export a diagnostics bundle from **Settings → Diagnostics** and attach it to your [GitHub issue](https://github.com/wangwanjie/gately/issues/new). It helps maintainers find the cause without asking you for screenshots and copy-pasted logs.

The bundle is generated on your own machine and is never uploaded anywhere. Click **Preview content** before downloading to see exactly what will be exported. The zip contains only plain text and JSON, so you can also unzip it and review every file.

## What is exported

| File | Content |
| --- | --- |
| `README.txt` | A bilingual description of the bundle |
| `summary.json` | Gately version and build info, OS and architecture, CPU model and core count, memory / disk totals, uptime, Go runtime stats, admin UI port numbers, network interface **names** with address **categories** (for example `private: 1, public: 2`) |
| `modules.json` | Module list and the status summary shown on the overview page |
| `config/*.json` | Configuration **structure**: field names, numbers, switches, and enum values such as protocol and mode |
| `logs/*.log` | Recent runtime logs of the system and every module, including those persisted before a restart (can be turned off) |
| `logs/crash.log` | Abnormal exit records: the message and stacks of Go runtime crashes (panic, fatal error), and the approximate time the process was killed (for example by the OOM killer); present only after an abnormal exit (can be turned off) |

## What is never exported

- Passwords, password hashes, API tokens, TOTP secrets, session keys, OAuth client secrets, DNS provider keys, SSH passwords and private keys, certificates and private keys. `account.json` and `tokens.json` are never read.
- Domain names, host names, real IP addresses and MAC addresses. They are replaced by placeholders such as `<domain>`, `<ipv4:private>`, `<ipv4:public>` and `<mac>`; ports are kept.
- The configuration directory path and your home directory (replaced by `<config-dir>` and `<home>`).
- Names, remarks and any other text you typed in the configuration: every string that is not a known enum value becomes `<redacted>`.
- File contents, Docker containers, images and environment variables, terminal sessions and traffic content.

## How redaction works

- **Configuration**: values are removed by default. Only field names, numbers, booleans, a fixed list of enum fields (`protocol`, `mode`, `provider`, …) and timestamps survive. Address fields keep only the port and the IP category.
- **Logs**: IP addresses, domains, e-mails, URLs, `password=…` style pairs, `Bearer` tokens, long token-like strings, MAC addresses and user directories are masked. Public service domains such as `github.com` and `letsencrypt.org` are kept because they show whether a failure is a network problem.
- A name you typed (for example a rule remark) can still appear in a log line if it does not look like an IP or domain. Skim `logs/*.log` before posting, and remove anything you do not want to share.

## Troubleshooting abnormal exits

While running, Gately writes crash details to `crash.log` in the config directory. After the process ends unexpectedly, the system log on the next start says whether the last run crashed or was killed:

- **Crashed**: `crash.log` contains the reason and the stacks of all goroutines; export the diagnostics bundle and attach it to the issue. For fatal errors such as `concurrent map writes`, Go prints the `fatal error: …` line only to standard error, which you can find in `docker logs`.
- **Ended unexpectedly**: the process left nothing behind, usually because the system killed it for running out of memory (OOM), it was ended with `kill -9` / `docker kill`, or the power went out. Run `dmesg -T | grep -i -E "oom|killed process"` on the host to check for OOM.

## API

`GET /api/system/diagnostics` (login session only, API tokens are rejected). `?logs=0` excludes logs and `?preview=1` returns the files as JSON.
