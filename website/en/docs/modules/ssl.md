# SSL Certificates

Issue and renew certificates automatically for use by [Web Services](/en/docs/modules/web), or export them to other software.

When several certificates cover the same domain, the one that expires last is used; the order and groups of the certificate list only affect how it is displayed.

## Capabilities

- **ACME DNS-01** automatic issuance and renewal, reusing the DNS providers from [Dynamic DNS](/en/docs/modules/ddns), with no need to open ports 80 / 443;
- **ACME HTTP-01**: needs no DNS provider, but port 80 of each domain must be reachable from the internet; wildcards are not supported;
- Wildcard certificates and EAB (for example ZeroSSL);
- **Short-lived and IP address certificates**: choose the Let's Encrypt short-lived profile (valid for about 6 days) to issue certificates for public IP addresses;
- Upload an existing certificate, or read it from files;
- Map to a directory: after a certificate changes, the files are written to the specified directory and a script runs, so other services can reload;
- Auto deploy: after a certificate changes, copy it to several local directories, push it to other hosts over SSH and run a command, or restart the Docker containers that use it.

## DNS-01 validation

Choose DNS-01 as the certificate's "Method" and pick a DNS provider. During issuance, Gately adds the `_acme-challenge` TXT record through the provider's API and deletes it after validation.

"Root domain" is the DNS zone that holds the record. When it is left empty, Gately looks up the authoritative zone of the validation record with an SOA query, so a subdomain delegated to another DNS provider (for example `web.example.com` hosted apart from `example.com`) and multi-level suffixes such as `example.com.cn` are detected correctly. If the lookup fails, the zone is inferred from the domain name and the reason is logged. You can also fill in the root domain yourself, for example `web.example.com`.

## Issuance network

The certificate's "Network" option restricts the outbound network used during issuance: "Auto" leaves the choice to the system, and "IPv4 only" or "IPv6 only" makes reaching the CA, calling the DNS provider API and checking that the TXT record has propagated all use only that address family. When the host has an IPv6 address but IPv6 does not actually work, the propagation check in auto mode may time out while querying the authoritative DNS servers; choose "IPv4 only" in that case.

## HTTP-01 validation

Choose HTTP-01 as the certificate's "Method". During issuance, the CA requests `http://<domain>/.well-known/acme-challenge/<token>`:

- If [Web Services](/en/docs/modules/web) already listens on port 80, it answers directly without affecting existing sites;
- If port 80 is free, Gately listens on it temporarily during issuance and closes it right after;
- If your router forwards public port 80 to another local port, enter that port in "Temporary port".

Many residential ISPs (for example in mainland China) block inbound port 80; use DNS-01 in that case.

## Short-lived and IP address certificates

"Certificate profile" corresponds to the ACME profile:

- **Default**: chosen by the CA; 90 days for Let's Encrypt;
- **Short-lived (IP certificate)**: the Let's Encrypt `shortlived` profile, valid for about 6 days. Gately renews it automatically before expiry according to "Short-lived certificate lead hours" in "Renewal settings" (24 hours by default), and retries every hour after a failure;
- You can also enter another profile name supported by the CA.

Enter a public IP address (IPv4 or IPv6, for example `203.0.113.10` or `2001:db8::1`) in the domains to request an IP address certificate; IP addresses and domains can share one certificate. Requirements:

- Use Let's Encrypt, choose the short-lived profile and choose HTTP-01 as the "Method". The CA requests `http://<IP>/.well-known/acme-challenge/<token>`, so port 80 of that IP must be reachable from the internet. The editor shows a hint once you enter an IP; click "Set up automatically" to apply these settings;
- Private addresses, loopback addresses and `100.64.0.0/10` cannot pass validation by a public CA; a custom CA (such as a self-hosted ACME server on your LAN) has none of these restrictions;
- ZeroSSL and Google Trust Services do not issue IP address certificates over ACME.

When HTTPS is accessed by IP address, the browser sends no domain name (SNI), so Gately picks the IP certificate that matches the local address of the connection; behind NAT or port forwarding, where the local address differs from the public IP in the certificate, the IP certificate that expires last is used.

After you switch the profile of an issued certificate between default and short-lived, it is reissued right away with the new profile. For short-lived certificates, an expiry reminder is sent only when the remaining time drops below half the lead hours, which means renewal has failed several times.

## Certificate mapping

Once a certificate is enabled, Gately writes `.crt`, `.key`, `.pem` and `_issuerCertificate.crt` into the mapping directory and runs the change script.

## Auto deploy

Under the certificate's "Auto deploy" you can add several deployment targets, which run automatically after the certificate is issued, renewed or updated:

- **Copy to local directory**: write the certificate to a directory on this machine, then run the "Run after copying" command locally. You can add several directories, each with a different command. If the certificate files in a directory are deleted or changed, the next check copies them again. When Gately runs in Docker, the directory and command are inside the container;
- **Upload to SSH host**: pick an existing SSH connection from [Web Terminal](/en/docs/modules/terminal) to reuse its address, authentication and proxy. The certificate files (`<remark>.crt`, `.key`, `.pem` and `_issuerCertificate.crt`, the same as certificate mapping, so the remark is required) are written to the remote directory, then the "Run after upload" command runs on the remote host, for example `systemctl reload nginx`. The command can use `$certName`, `$certDir`, `$certFile` and `$keyFile`. Files are first written to temporary files with their permissions set and then renamed into place; the private key gets permission 0600;
- **Restart local containers**: after the certificate changes, the selected Docker containers are restarted in turn. Use this for containers that mount the mapping directory and need a restart to load the new certificate.

"Copy to local directory" and "Upload to SSH host" accept file names for the certificate (full chain) and the private key, such as `fullchain.pem` and `privkey.pem`. With both set, only those two files are written and the remark may be empty; left empty, the certificate mapping file names are used. `$certFile` and `$keyFile` in the command point to the files actually written.

Auto deploy never trusts an unknown host: connect to the selected SSH connection once in Web Terminal to record its host key before anything is pushed. Failed deployments are retried every hour and raise a "Certificate: deployment failed" event, which you can subscribe to in the [Notification Center](/en/docs/modules/notify). "Deploy now" on the certificate card ignores the retry interval, deploys to all enabled targets right away and shows the result for each target.

When you edit a certificate, deployments whose target settings did not change are not run again; changing the remote directory, command or containers triggers one new deployment.

## Security note

Private keys are stored in `ssl.json` in the configuration directory. Keep that file safe, and make sure the repository is private if you put it under version control.
