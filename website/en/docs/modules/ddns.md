# Dynamic DNS (DDNS)

The public IP of a home broadband connection changes over time. DDNS updates your DNS records automatically after an IP change, so your domain always points to your current address.

## Supported providers

Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2, and a custom callback (for providers that are not built in, or for self-hosted endpoints).

| Provider | Credentials | Usable for certificate DNS validation |
| --- | --- | --- |
| Amazon Route 53 | Access Key ID / Secret Access Key, optional hosted zone ID | Yes |
| OVHcloud | Application Key / Secret and Consumer Key; endpoint ovh-eu / ovh-ca / ovh-us | Yes |
| Gandi | Personal Access Token | Yes |
| NS1 | API Key | Yes |
| Bunny DNS | Account API Key | Yes |
| Namecheap | The domain's Dynamic DNS password (A records only) | No |
| DynDNS2 | Username, password and update URL; for No-IP, Dynu and other services compatible with the dyndns2 protocol | No |

## How the IP is obtained

- **URL**: request a public-IP lookup address (replace it with any service you like, or host your own);
- **Network interface**: read the address of a specific network interface directly, suited to devices that hold a public IP;
- **Command**: run a command and use its output, suited to unusual network setups.

Both IPv4 and IPv6 are supported.

## Record value variables {#record-vars}

Record values can use these variables, replaced with current values on every sync:

- `{ipv4Addr}`, `{ipv6Addr}`: the IPv4 and IPv6 addresses this task obtains (used by default when an A or AAAA record value is left empty);
- `{STUN_rule name_PORT}`, `{STUN_rule name_IP}`, `{STUN_rule name_ADDR}`: a STUN rule's current public port, IP and IP:port; the rule name may contain underscores. For example a TXT record of `port={STUN_ssh_PORT}`, or an A record of `{STUN_ssh_IP}`.

When a STUN rule's public address changes, tasks that reference it sync right away instead of waiting for the next check. While the rule is disabled or has not traversed yet, the record is skipped for that round and the record status shows why. STUN variables can also be used in the task's Webhook. When they appear only in the Webhook, the Webhook is still called once after the address changes. The rule key can be used instead of the rule name.

## Notifications

A Webhook can be called when the IP changes or a sync fails.

## Recommendations

- Create a sub-account key limited to DNS record editing for your DNS provider. Avoid using the main account key;
- After creating a task, click "Force sync" to confirm that the record has been updated;
- To drop a record you no longer need, click the delete icon at the right of that record in the list, without opening the editor; it is only removed from the task, the existing DNS record at the provider is kept, and a task keeps at least one record;
- The public-IP lookup address no longer depends on any service operated by the project, and you can change it in a task at any time.

## Groups and sorting

You can give tasks a "Group" (up to 64 characters). The list shows groups as collapsible sections: groups appear in the order they first occur, ungrouped tasks come last, and the collapsed state is kept in the browser. The search box at the top of the page filters by name, group, domain or provider.

Click "Sort" to enter sort mode. Use the arrows to move tasks up and down or to move whole groups. Check several tasks and drag any one of them to move them together; dropping onto tasks or the header of another group moves them into that group. The toolbar moves the checked tasks into a group in one step, or out of their group when the name is left empty. Order and groups only affect the list, and changing them means tasks are not restarted.

Hover over a group header and click the pencil button to rename the group: everything in it moves to the new name and keeps its position and order. Choosing a name that is already in use merges the two groups after a confirmation.
