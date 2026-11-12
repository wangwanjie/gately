# Métriques (Prometheus / Grafana)

Gately exporte des métriques globales au format texte Prometheus (0.0.4) sur `/api/metrics`, couvrant le processus, la redirection de ports, STUN, DDNS, les certificats et les services Web.

## Configuration de la collecte

1. Dans la page « Jetons d'API », créez un jeton disposant uniquement de la permission de lecture « Métriques » (`metrics:read`). Ce jeton ne peut atteindre aucun autre point de terminaison.
2. Ajoutez à `scrape_configs` dans Prometheus :

```yaml
scrape_configs:
  - job_name: gately
    metrics_path: /api/metrics
    scheme: http
    authorization:
      credentials: gtl_xxxx
    static_configs:
      - targets: ["gately.example.lan:16680"]
```

## Métriques

| Métrique | Type | Description |
| --- | --- | --- |
| `gately_build_info{version,commit,goversion}` | gauge | Informations de build, toujours 1 |
| `gately_uptime_seconds` | gauge | Durée de fonctionnement du processus |
| `gately_goroutines` / `gately_memory_alloc_bytes` | gauge | Goroutines et mémoire du tas |
| `gately_cert_days_remaining{cert}` | gauge | Jours restants avant l'expiration du certificat |
| `gately_cert_expiry_timestamp_seconds{cert}` | gauge | Date d'expiration du certificat |
| `gately_ddns_record_up{task,record,type}` | gauge | Indique si la dernière synchronisation a réussi (1 = succès) |
| `gately_ddns_last_update_timestamp_seconds{...}` | gauge | Date de la dernière modification de l'enregistrement |
| `gately_stun_up{rule}` | gauge | Indique si la règle STUN a un mappage fonctionnel |
| `gately_stun_connections_total{rule}` | counter | Connexions traitées par la redirection STUN intégrée |
| `gately_portforward_active_connections{rule}` | gauge | Connexions ouvertes d'une règle de redirection |
| `gately_portforward_connections_total{rule}` | counter | Connexions acceptées d'une règle de redirection |
| `gately_portforward_bytes_total{rule,direction}` | counter | Octets relayés (`in` = du client vers la cible) |
| `gately_protoroute_active_connections{rule}` | gauge | Connexions ouvertes d'une règle de routage par protocole |
| `gately_protoroute_connections_total{rule}` | counter | Connexions acceptées d'une règle de routage par protocole |
| `gately_protoroute_bytes_total{rule,direction}` | counter | Octets relayés par une règle de routage par protocole (`in` = du client vers la cible) |
| `gately_protoroute_route_connections_total{rule,route}` | counter | Connexions envoyées vers chaque route ; `route` est le nom de la route (son numéro si elle n'a pas de nom), `default` pour la cible par défaut |
| `gately_site_requests_total{listener,site}` | counter | Requêtes par site |
| `gately_site_responses_total{listener,site,class}` | counter | Réponses par classe de statut (2xx/3xx/4xx/5xx) |
| `gately_site_response_bytes_total` / `gately_site_errors_total` / `gately_site_active_requests` | counter / counter / gauge | Octets de réponse, erreurs de backend, requêtes en cours |
| `gately_site_blocked_total{listener,site,reason}` | counter | Requêtes bloquées par motif (`ban`, `ip`, `country`, `rateLimit`, `waf`) |
| `gately_site_request_duration_seconds{listener,site,le}` | histogram | Histogramme du temps jusqu'au premier octet du site en secondes, 13 tranches `le` de 0,005 à 60 ; hors WebSocket et autres changements de protocole et requêtes coupées |
| `gately_upstream_up{listener,site,upstream[,route]}` | gauge | Indique si le serveur amont est en bonne santé ; les backends par chemin et par utilisateur portent en plus une étiquette `route` (préfixe de chemin, ou `@` suivi de la liste des utilisateurs) |
| `gately_upstream_requests_total{listener,site,upstream[,route]}` | counter | Requêtes transmises au serveur amont |
| `gately_upstream_response_bytes_total{listener,site,upstream[,route]}` | counter | Octets du corps de réponse renvoyés par le serveur amont (hors données après passage en WebSocket) |
| `gately_listener_requests_total{listener,action}` | counter | Requêtes traitées par l'écouteur lui-même : `https_redirect` pour le HTTP en clair redirigé vers HTTPS, `not_found` pour le 404 lorsqu'aucun site ne correspond |
| `gately_host_cpu_usage_percent` / `gately_host_memory_used_percent` / `gately_host_disk_used_percent` / `gately_host_load1` | gauge | Utilisation CPU, mémoire et partition du répertoire de configuration de l'hôte et charge sur 1 minute du dernier échantillon (exportés quand le module Performances est actif ; les valeurs illisibles sont omises) |
| `gately_process_cpu_usage_percent` / `gately_process_resident_memory_bytes` / `gately_process_open_fds` | gauge | CPU du processus Gately (part de la machine entière), mémoire résidente et descripteurs de fichiers ouverts du dernier échantillon (exportés quand le module Performances est actif) |
| `gately_perf_alerts_firing{rule}` | gauge | Nombre d'objets en alerte pour chaque règle d'alerte de performances activée |
| `gately_guard_banned` | gauge | Adresses IP ou réseaux actuellement dans la liste de bannissement |
| `gately_guard_firewall_active` | gauge | Indique si les règles de pare-feu de la prévention d'intrusion (nftables ou iptables) sont en place (1) ou non (0) |
| `gately_guard_dropped_packets_total{rule}` | counter | Paquets abandonnés par les règles du pare-feu, par règle |
| `gately_guard_jail_matches_total{jail}` | counter | Lignes de journal correspondant à chaque règle de journaux |
| `gately_guard_jail_bans_total{jail}` | counter | Bannissements déclenchés par chaque règle de journaux |
| `gately_knock_packets_total{result}` | counter | Paquets de knock par résultat (`accepted`, `rejected`, `ignored`) |
| `gately_knock_grants` | gauge | Adresses sources détenant actuellement une autorisation de knock |
| `gately_knock_firewall_active` | gauge | Indique si les règles de blocage du pare-feu du port knocking sont en place (1) ou non (0) |
| `gately_knock_firewall_dropped_total` | counter | Nouvelles connexions vers des ports protégés abandonnées par le pare-feu |

## Exemples d'alertes

```yaml
groups:
  - name: gately
    rules:
      - alert: GatelyCertExpiring
        expr: gately_cert_days_remaining < 14
        for: 1h
      - alert: GatelyDDNSFailing
        expr: gately_ddns_record_up == 0
        for: 15m
      - alert: GatelyUpstreamDown
        expr: gately_upstream_up == 0
        for: 2m
      - alert: GatelySiteSlow
        expr: histogram_quantile(0.95, sum by (le, listener, site) (rate(gately_site_request_duration_seconds_bucket[5m]))) > 2
        for: 5m
```

## Tableau de bord Grafana

Dans Grafana, choisissez Import, téléversez [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) et sélectionnez votre source de données Prometheus.
