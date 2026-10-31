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
| `gately_upstream_up{listener,site,upstream}` | gauge | Indique si le serveur amont est en bonne santé |

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
```

## Tableau de bord Grafana

Dans Grafana, choisissez Import, téléversez [deploy/grafana/gately-dashboard.json](../../deploy/grafana/gately-dashboard.json) et sélectionnez votre source de données Prometheus.
