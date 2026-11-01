# Network Diagnostics

Run Ping, traceroute, port probe, DNS lookup, speed test and LAN scan in your browser. Menu location: "Tools → Network Diagnostics" in the sidebar.

## Background tasks

Each click on "Start" creates a background task. The API returns the task ID immediately, and the UI polls for output.

- Up to 4 tasks run at the same time; beyond that a "try again later" message is shown;
- A single task runs for at most 5 minutes and is cancelled automatically on timeout;
- The latest 50 tasks are kept, and running tasks are never cleaned up; each task keeps at most 500 lines of output;
- A running task can be cancelled with "Stop". "Recent tasks" at the bottom of the page lists past tasks, and clicking one shows its output;
- Task states: running, done, failed, cancelled.

## Ping

Sends ICMP echo requests to the target and outputs each round-trip time, the packet loss rate and the minimum / average / maximum latency.

| Parameter | Description |
| --- | --- |
| Target | Hostname or IP, IPv4 and IPv6 supported |
| Count | Default 4, up to 20 |

Each request times out after 2 seconds, with an interval of about 0.7 seconds. An unprivileged UDP-style ICMP socket is tried first, then a raw socket. Inside a Docker container this needs the `NET_RAW` capability or the `net.ipv4.ping_group_range` setting; if permissions are insufficient, use port probe instead. If the target does not respond at all, the task ends as failed.

## Traceroute

Calls a system command to trace the path and outputs the command's raw result.

- Windows uses `tracert`; other systems prefer `traceroute`, then `tracepath`;
- At most 20 hops, with no reverse lookup (IPs are shown directly);
- If the system has no matching command, the task reports an error. In Docker images you can install the `traceroute` package.

## Port probe

Makes TCP connection attempts to the target and lists only the open ports.

| Parameter | Description |
| --- | --- |
| Target | Hostname or IP |
| Ports | For example `22,80,443,8000-8100`; comma-separated lists and ranges are supported, with ports from 1 to 65535 |

- At most 1024 ports per probe;
- Concurrency of 100, with a 1.5-second connection timeout per port;
- Common ports show the service name (ssh, http, https, mysql, redis, postgres, rdp, etc.) and connection time.

## DNS lookup

| Parameter | Description |
| --- | --- |
| Domain / IP | The name to query; enter an IP for the `PTR` type |
| Type | `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `PTR`; default `A` |
| DNS server | Leave empty to use the system resolver; you can enter `223.5.5.5`, `223.5.5.5:5353` or a DoH address such as `https://dns.alidns.com/dns-query` |

Queries time out after 8 seconds and output the records and elapsed time.

## Speed test

A download speed test with 4 concurrent streams. It prints the current rate (Mbps) every second, and at the end prints the total downloaded and the average rate.

| Parameter | Description |
| --- | --- |
| Download URL | A full `http(s)://` URL. Leave empty to use the Cloudflare speed test endpoint (`https://speed.cloudflare.com/__down?bytes=50000000`). The dropdown lists common public test files from Cloudflare, Vultr (Tokyo, Singapore, Los Angeles), Hetzner (Singapore, Germany) and OVH (France); any other URL can be typed in. A file that finishes downloading is requested again until the duration ends |
| Duration | In seconds, 2 to 30 in the UI, default 8 |

Speed test requests do not follow redirects, so they cannot be steered to addresses that probing is blocked from.

## LAN scan

Scans a subnet for online devices and shows the results in a table: IP, MAC, hostname, open ports.

| Parameter | Description |
| --- | --- |
| Subnet | CIDR, for example `192.168.1.0/24`. Leave empty to use the subnet of the first private IPv4 interface on this machine (the dropdown lists the available local subnets, and subnets that are too large are shrunk to /24) |

Limits and how it works:

- Only IPv4 private subnets are supported (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`);
- At most 1024 addresses per scan, equivalent to a /22; network and broadcast addresses are excluded automatically;
- For each address, the common ports 80, 443, 22, 445, 139, 3389, 8080, 62078, 5000 and 53 are probed, and a connection that succeeds or is refused counts as online;
- MAC addresses come from the system neighbor table, and a host that has an entry in the neighbor table but no open common port is also listed as online;
- Hostnames are resolved with reverse DNS (1.5-second timeout);
- For devices with a MAC, click "Add to Wake-on-LAN" to add them to the device list of [Wake-on-LAN](/en/docs/modules/tools).

## Probe blocklist

"Probe blocklist" at the top right of the page sets the targets that the diagnostic tools may not access.

- Cloud provider metadata addresses are always blocked: `169.254.169.254/32`, `100.100.100.200/32`, `fd00:ec2::254/128`, which prevents the tools from being used to steal instance credentials;
- You can add IPs or subnets, one per line, such as `10.0.0.0/8`;
- If any address a target hostname resolves to matches the list, the task fails with an error. The speed test download URL is checked too, and scans skip matching addresses.

::: tip
The diagnostic tools run on the host or container where Gately runs, so results reflect the network of that environment. To limit what can be probed, add sensitive internal subnets to the probe blocklist.
:::

## Network self-check

The "Self-check" button on the overview page (also offered in the first-start onboarding guide) checks common network and configuration problems in one go. Items that need attention are listed first, with suggestions:

| Check | Description |
| --- | --- |
| Public address | Gets the outbound IPv4 / IPv6 through the public IP lookup URLs used by DDNS |
| NAT | Whether this machine holds a public IPv4 / IPv6 address directly; a 100.64.0.0/10 address on a network interface suggests carrier-grade NAT (overlay interfaces such as Tailscale are excluded) |
| System time | Estimates clock offset from the response time of the lookup services; more than 60 seconds breaks two-factor codes and certificate validation |
| Ports 80 / 443 | Whether each port is free, already in use, or cannot be listened on due to missing permission |
| DDNS resolution | For records that use a dynamic address, whether the domain currently resolves to this machine's outbound address |
| Certificates | Certificates that have expired or expire within 14 days |
| HTTPS site certificates | Sites on enabled HTTPS listeners whose domains have no matching certificate (these domains fail during the TLS handshake) |
| frp conflicts | Duplicate proxy names in frpc, and proxy name, domain or remote port conflicts reported by frps in the last 30 minutes of logs (common when several devices copy the same frpc config) |
| Module status | Modules in a warning or error state on the overview |

The self-check queries the configured public IP lookup services and runs only when you click it. Read-only accounts and API tokens cannot use it.
