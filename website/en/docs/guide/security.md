# Security Model & Privacy Promise

Gately runs on the network edge and handles DNS provider keys, certificate private keys and other credentials, so security and transparency are its primary design goals.

## Privacy promise

- **Zero outbound connections by default**: no connections to project servers, no device reporting, no remote configuration push.
- **Visible, editable outbound addresses**: public IP lookup, STUN servers and similar addresses are shown in the UI and can be changed or turned off.
- **Self-managed third-party sign-in**: GitHub, Google, QQ and OIDC sign-in use OAuth apps you register yourself, with no relay in between.
- **Self-managed upgrades**: the program never downloads or replaces itself in the background. Online upgrades on Linux binary installs run only after an admin clicks, verify the SHA-256 and keep the previous version; you decide when to upgrade.

## Data protection

- Configuration is plain JSON with file mode 0600, so it can be audited and rolled back.
- Sensitive fields are masked in API output. The eye button in the UI fetches the saved plain text on request and needs write permission. Web user passwords stored as bcrypt hashes, OIDC client secrets and private keys are never returned; shared users with two-factor sign-in can show their QR code again. Docker container environment variables, start commands and process command lines are handled the same way: without Docker write permission they are shown as `********`.
- The cloudflared token is passed through an environment variable and does not appear in the process list; frp and Tailscale tokens and keys are stored with 0600 permissions.

## Authentication and access control

- bcrypt password hashing and HMAC-signed session tokens;
- The default password `gately.cn` is public, and the admin UI keeps reminding you to change it while it is in use; a forgotten password or lost two-factor authenticator can only be reset on the server with `gately reset-password` / `gately reset-totp` (this needs read and write access to the config directory and typing `yes` to confirm); a reset invalidates all sessions and is recorded in the audit log. See [Reset password and two-factor authentication](/en/docs/guide/install#reset); while the default password is still in use, the account, two-step verification, passkeys, additional accounts, API tokens, system settings and Web terminal security settings can be viewed but not changed (restoring a backup or rolling back the system settings or Web terminal to a history version is refused too) until the default password is changed; the new password cannot be the default one, and once changed it cannot be set back;
- Passkey (WebAuthn) sign-in: only the public key is stored, user verification (fingerprint, face or PIN) is required, and the signature counter guards against cloned authenticators; requires access through a domain name over HTTPS (or localhost);
- Two-factor authentication (TOTP, verified against the RFC 6238 test vectors);
- Read-only accounts: can view status and configuration, but cannot make changes, download backups, read secrets or open terminals; sessions are invalidated immediately when the password is reset or the account is deleted; when read-only accounts and API tokens with only read access read the configuration, site shared-user passwords, share link tokens, request and response header values, secrets, and passwords and query values in URLs are shown as `********`;
- Login rate limiting;
- Local reverse proxies: when the admin UI sits behind nginx, haproxy or Gately Web Services on the same host, `X-Real-IP` / `X-Forwarded-For` are trusted only when sent from addresses of this host. LAN-only access, login rate limiting and the audit log use them to get the real source. You can turn this off or add extra trusted proxies under "Settings → Access Control"; when the two headers disagree, the `X-Forwarded-For` entry appended by the proxy wins, and when one is an internal address and the other a public one the public address is used, so a forged internal address cannot get past LAN-only access; have the proxy set both headers (nginx: `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`);
- Secure entrance path, LAN-only access and admin UI HTTPS;
- fnOS unified gateway: when the fnOS package is accessed through the fnOS unified gateway, fnOS controls access and only fnOS administrators can open it. "Sign in with fnOS" is off by default; once enabled, fnOS administrators who are signed in enter as the Gately administrator (skipping two-step verification), while regular fnOS users still sign in. See [fnOS native app](/en/docs/guide/install#fnos);
- Module switches: modules turned off under "Settings → Modules" start no services and serve no admin API, which shrinks the attack surface;
- Multi-token API: each token has read/write access only to the modules selected at creation, can have an expiry time, and records the source IP of its most recent use. Account, two-factor authentication, token management, and the backup, restore, restart and system settings endpoints accept login sessions only. See [API Tokens & OpenAPI](/en/docs/reference/api);
- Site access control: automatic bans on authentication failures, GeoIP country filtering and path rules. See [Web Services](/en/docs/modules/web);
- Configuration backups support encryption (scrypt + AES-GCM). See [Configuration Backup](/en/docs/modules/backup).

## Audit and traceability

The audit log records sign-ins, configuration changes and sensitive access. Config history keeps the version of each configuration file before it was overwritten and supports one-click rollback. See [Audit Log & Config History](/en/docs/guide/audit). Release artifacts come with cosign signatures, build provenance attestations and an SBOM; see [Verifying Releases](/en/docs/guide/verify) for how to check them.

## Defaults for high-risk features

The local shell terminal is disabled by default. The SSH terminal uses a trust-on-first-use policy to record host key fingerprints.

## Reporting security issues

Please report vulnerabilities privately through the repository's Security page and avoid disclosing details in public issues: <https://github.com/wangwanjie/gately/security>

For more practical recommendations, see the [Hardening Guide](/en/docs/guide/hardening).
