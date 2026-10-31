# Port Forwarding

Forward a port that Gately listens on to another device or service.

## Capabilities

- TCP and UDP;
- IPv4 and IPv6, with forwarding between v4 and v6;
- Port ranges and multiple ports;
- Multiple targets with failover;
- Connection limits;
- Traffic statistics and runtime logs;
- Sending the PROXY protocol (v1 / v2) to targets, passing the real client address downstream;
- To share one port such as 443 between services by domain or protocol, use [Protocol Routing](/en/docs/modules/protoroute).

## PROXY protocol

Port forwarding only relays TCP, so by default the downstream sees only Gately's own address. Set "PROXY protocol" to v1 or v2 in a rule, and Gately sends a PROXY header right after connecting to the target. The downstream then gets the real client address for logging, rate limiting and access control.

The target must enable the matching receive option, otherwise the connection is rejected. For example:

- haproxy: `bind 192.168.1.8:4434 accept-proxy`;
- nginx: `listen 4435 ssl proxy_protocol;`;
- Gately Web Services: enable "Accept PROXY protocol" on the listener, and add the source to the trusted PROXY sources.

This option applies to TCP only; UDP forwarding is unaffected.

## Typical scenarios

- Expose NAS service ports through a single entry point;
- Bridge IPv6 and IPv4 traffic;
- Configure a primary and a standby backend for one service.

## Deployment tip

Port forwarding listens on host ports directly. For Docker deployments, `--network host` is recommended.

## Groups and sorting

You can give rules a "Group" (up to 64 characters). The list shows groups as collapsible sections: groups appear in the order they first occur, ungrouped rules come last, and the collapsed state is kept in the browser. The search box at the top of the page filters by name, group, listen port or target.

Click "Sort" to enter sort mode. Use the arrows to move rules up and down or to move whole groups. Check several rules and drag any one of them to move them together; dropping onto rules or the header of another group moves them into that group. The toolbar moves the checked rules into a group in one step, or out of their group when the name is left empty. Order and groups only affect the list, and changing them means forwarding is not restarted and open connections stay up.

Hover over a group header and click the pencil button to rename the group: everything in it moves to the new name and keeps its position and order. Choosing a name that is already in use merges the two groups after a confirmation.
