# List groups and sorting

Lists with many entries can be grouped, sorted manually and searched, and the controls work the same way on every page. Order and groups only affect how the admin UI displays the list; changing them does not restart running instances or drop connections.

## Supported lists

| Page | Lists |
| --- | --- |
| Dynamic DNS | Tasks; DNS records within a task |
| STUN Traversal | Rules |
| Port Forwarding | Rules |
| SSL Certificates | Certificates |
| Web Services | Sites within a listener (listeners themselves can be moved up and down) |
| WAF Firewall | Instances |
| Intrusion Prevention | Log rules |
| frp | frpc instances, frps instances, proxies |
| Cloudflare Tunnel | Tunnels |
| File Browser | Instances |
| Storage Mounts | Mounts |
| Wake-on-LAN | Devices |
| Scheduled Tasks | Tasks |
| Notifications | Channels, rules |
| Web Terminal | Saved connections |
| API Tokens | Tokens |

Most of these lists offer both "Card view" and "Compact view", switched at the top right of the list; on narrow screens the card view is always used.

## Groups

Fill in "Group" in the edit dialog (up to 64 characters; you can pick an existing group). Leave it empty for no group. As soon as one entry has a group, the list is shown as collapsible sections by group: groups appear in the order they first occur, ungrouped entries come last, and the collapsed state of each group is saved in the browser. The search box filters entries by name, group, remark and similar fields, and expands all groups while searching.

Hover over a group heading and click the pencil button to rename the group: all entries in the group take the new name and keep their position and order. If you rename it to the name of an existing group, the two groups are merged after confirmation.

## Sort mode

Click "Sort" to enter sort mode. All entries are shown, and search and filters are paused:

- Use the arrows to move entries up or down within a group, or on a group heading to move the whole group;
- On desktop you can drag entries directly; dropping onto an entry or heading of another group adds the entry to that group;
- After selecting several entries (the checkbox on a group heading selects the whole group), drag any one of them and all selected entries move together in their current order;
- The toolbar can move the selected entries to a given group in one step ("Move to group"); leaving it empty removes them from their group.

Phones and other narrow screens do not support dragging; use the arrows and the toolbar to do the same.

## Notes for specific pages

- **Dynamic DNS**: "Sort records" on a task card opens a dialog for the order and groups of the DNS records in that task. Saving takes effect immediately and does not trigger a new sync. When editing a task, the same dialog is also available in the "DNS records" tab and is saved together with the task. When records have groups, the records on the card are shown by group, and each group can be collapsed on its own.
- **frp**: The proxy list can be filtered by frpc instance; sort mode shows all proxies and adjusts the full order.
- **API Tokens**: Until you sort them manually, tokens are shown by creation time, newest first. After the order has been adjusted, they are shown in the saved order, and new tokens are placed first.
- **Auth users**: "Sites using shared users" is shown by listener, and the sites within a listener are grouped by their groups in Web Services. Listeners and groups can each be collapsed, and the toolbar can collapse or expand all of them at once.

## API

Every general list endpoint provides `PUT <list path>-order` (body `{"keys": [...]}`, which must contain all entries) to change the order and `PUT <list path>-group` (body `{"keys": [...], "group": "..."}`) to set the group in bulk, for example `PUT /api/stun/rules-group`. Records within a DDNS task use `PUT /api/ddns/tasks/{key}/records-layout`, and API tokens use `PUT /api/tokens/order` and `PUT /api/tokens/group`. See the [API reference](/en/docs/reference/api-reference) for details.
