# API d'administration de Gately

L'API d'administration de Gately permet la gestion en masse par scripts et automatisation.

Authentification : créez un jeton dans la page « Jetons API » de l'interface d'administration et ajoutez l'en-tête de requête `Authorization: Bearer gtl_xxxx`.

Permissions : chaque jeton ne possède que les permissions cochées à sa création, au format `<module>:read` ou `<module>:write` ; l'écriture inclut la lecture. Les interfaces de compte, d'authentification à deux facteurs, de gestion des jetons, de sauvegarde, de restauration, de redémarrage et de paramètres système exigent une session connectée.

Le corps de la requête utilise `Content-Type: application/json` ; les erreurs sont renvoyées au format `{"error": "description"}`.

Les modules désactivés dans « Paramètres → Modules » n’exposent aucune API : leurs points de terminaison renvoient 403 et n’apparaissent plus dans la documentation de l’API de l’instance en cours d’exécution.

Site officiel : https://gately.cn

Une description lisible par machine se trouve dans `openapi.json` (OpenAPI 3.0) du même répertoire.

## Démarrage rapide

```bash
# Consulter l'état du système (nécessite status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# Lister toutes les tâches DDNS (nécessite ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Synchroniser immédiatement une tâche DDNS (nécessite ddns:write, la clé provient de la liste renvoyée à l'étape précédente)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## Permissions

| Domaine de permission | Nom | Permissions disponibles |
| --- | --- | --- |
| `portforward` | Redirection de ports | `portforward:read`、`portforward:write` |
| `protoroute` | Routage par protocole | `protoroute:read`、`protoroute:write` |
| `ddns` | DNS dynamique | `ddns:read`、`ddns:write` |
| `stun` | Traversée STUN | `stun:read`、`stun:write` |
| `tailscale` | Réseau Tailscale | `tailscale:read`、`tailscale:write` |
| `frp` | Traversée de réseau frp | `frp:read`、`frp:write` |
| `cloudflared` | Tunnel Cloudflare | `cloudflared:read`、`cloudflared:write` |
| `ssl` | Certificat SSL | `ssl:read`、`ssl:write` |
| `waf` | Pare-feu WAF | `waf:read`、`waf:write` |
| `webservice` | Service Web | `webservice:read`、`webservice:write` |
| `guard` | Prévention d'intrusion | `guard:read`、`guard:write` |
| `wol` | Wake-on-LAN | `wol:read`、`wol:write` |
| `webterminal` | Terminal Web | `webterminal:read`、`webterminal:write` |
| `cron` | Tâche planifiée | `cron:read`、`cron:write` |
| `docker` | Gestion Docker | `docker:read`、`docker:write` |
| `webdav` | Service WebDAV | `webdav:read`、`webdav:write` |
| `ftp` | Service FTP | `ftp:read`、`ftp:write` |
| `smb` | Service SMB | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | Montages de stockage | `storage:read`、`storage:write` |
| `notify` | Centre de notifications | `notify:read`、`notify:write` |
| `nettools` | Diagnostic réseau | `nettools:read`、`nettools:write` |
| `backup` | Sauvegarde de configuration | `backup:read`、`backup:write` |
| `status` | État du système | `status:read` |
| `logs` | Journaux système | `logs:read` |
| `metrics` | Métriques (Prometheus) | `metrics:read` |
| `import` | Import de configuration Lucky | `import:write` |
| `fileusers` | Utilisateurs partagés des services de fichiers | `fileusers:read`、`fileusers:write` |

Syntaxe générique : `*:read` désigne la lecture seule sur tous les modules, `*:write` ou `*` la lecture et l'écriture sur tous les modules.

## Redirection de ports

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/portforward/logs` | `portforward:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/portforward/rules` | `portforward:read` | Lister Règle de redirection de ports |
| POST | `/api/portforward/rules` | `portforward:write` | Ajouter Règle de redirection de ports |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | Modifier Règle de redirection de ports |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | Supprimer Règle de redirection de ports |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | Activer ou désactiver Règle de redirection de ports |
| PUT | `/api/portforward/rules-order` | `portforward:write` | Réordonner Règle de redirection de ports |
| PUT | `/api/portforward/rules-group` | `portforward:write` | Définir le groupe de plusieurs éléments : Règle de redirection de ports |
| GET | `/api/portforward/settings` | `portforward:read` | Lire les paramètres globaux de redirection de ports |
| PUT | `/api/portforward/settings` | `portforward:write` | Enregistrer les paramètres globaux de redirection de ports |

## Routage par protocole

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/protoroute/rules` | `protoroute:read` | Lister règle de routage par protocole |
| POST | `/api/protoroute/rules` | `protoroute:write` | Ajouter règle de routage par protocole |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | Modifier règle de routage par protocole |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | Supprimer règle de routage par protocole |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | Activer ou désactiver règle de routage par protocole |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | Réordonner règle de routage par protocole |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | Définir le groupe de plusieurs éléments : règle de routage par protocole |
| GET | `/api/protoroute/settings` | `protoroute:read` | Lire les paramètres du routage par protocole |
| PUT | `/api/protoroute/settings` | `protoroute:write` | Enregistrer les paramètres du routage par protocole |

## DNS dynamique

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/ddns/logs` | `ddns:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/ddns/tasks` | `ddns:read` | Lister Tâche DDNS |
| POST | `/api/ddns/tasks` | `ddns:write` | Ajouter Tâche DDNS |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | Modifier Tâche DDNS |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | Supprimer Tâche DDNS |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | Activer ou désactiver Tâche DDNS |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | Réordonner Tâche DDNS |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | Définir le groupe de plusieurs éléments : Tâche DDNS |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | Afficher les secrets en clair |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | Synchroniser immédiatement une tâche DDNS |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | Supprimer un enregistrement d'une tâche DDNS |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | Réordonner et grouper les enregistrements DNS d'une tâche |
| GET | `/api/ddns/providers` | `ddns:read` | Lister les fournisseurs DNS pris en charge et leurs paramètres requis |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | Lister les cartes réseau de cette machine et leurs adresses |
| POST | `/api/ddns/iptest` | `ddns:write` | Tester l'obtention de l'IP publique |
| GET | `/api/ddns/defaults` | `ddns:read` | Lire les valeurs par défaut DDNS |
| GET | `/api/ddns/settings` | `ddns:read` | Lire les paramètres globaux DDNS |
| PUT | `/api/ddns/settings` | `ddns:write` | Enregistrer les paramètres globaux DDNS |

## Traversée STUN

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/stun/logs` | `stun:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/stun/rules` | `stun:read` | Lister Règle STUN |
| POST | `/api/stun/rules` | `stun:write` | Ajouter Règle STUN |
| PUT | `/api/stun/rules/{key}` | `stun:write` | Modifier Règle STUN |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | Supprimer Règle STUN |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | Activer ou désactiver Règle STUN |
| PUT | `/api/stun/rules-order` | `stun:write` | Réordonner Règle STUN |
| PUT | `/api/stun/rules-group` | `stun:write` | Définir le groupe de plusieurs éléments : Règle STUN |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | Redémarrer une règle STUN |
| GET | `/api/stun/netinterfaces` | `stun:read` | Lister les cartes réseau de cette machine et leurs adresses |
| GET | `/api/stun/settings` | `stun:read` | Lire les paramètres globaux STUN |
| PUT | `/api/stun/settings` | `stun:write` | Enregistrer les paramètres globaux STUN |

## Réseau Tailscale

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/tailscale/settings` | `tailscale:read` | Lire les paramètres Tailscale |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Enregistrer les paramètres Tailscale |
| GET | `/api/tailscale/secrets` | `tailscale:write` | Afficher les secrets en clair |
| GET | `/api/tailscale/status` | `tailscale:read` | Consulter l'état de connexion Tailscale |
| POST | `/api/tailscale/restart` | `tailscale:write` | Redémarrer Tailscale |
| POST | `/api/tailscale/logout` | `tailscale:write` | Se déconnecter de Tailscale |
| GET | `/api/tailscale/install` | `tailscale:read` | Voir les informations d'installation et la progression d'un programme géré |
| POST | `/api/tailscale/install` | `tailscale:write` | Installer ou mettre à jour en ligne un programme géré |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | Revenir à une version précédente conservée |
| DELETE | `/api/tailscale/install` | `tailscale:write` | Désinstaller un programme géré |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | Supprimer une version précédente conservée |
| POST | `/api/tailscale/reset` | `tailscale:write` | Rétablir les paramètres par défaut du module |

## Traversée de réseau frp

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/frp/logs` | `frp:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/frp/clients` | `frp:read` | Lister Client frp |
| POST | `/api/frp/clients` | `frp:write` | Ajouter Client frp |
| PUT | `/api/frp/clients/{key}` | `frp:write` | Modifier Client frp |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | Supprimer Client frp |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | Activer ou désactiver Client frp |
| PUT | `/api/frp/clients-order` | `frp:write` | Réordonner Client frp |
| PUT | `/api/frp/clients-group` | `frp:write` | Définir le groupe de plusieurs éléments : Client frp |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | Afficher les secrets en clair |
| GET | `/api/frp/servers` | `frp:read` | Lister Serveur frp |
| POST | `/api/frp/servers` | `frp:write` | Ajouter Serveur frp |
| PUT | `/api/frp/servers/{key}` | `frp:write` | Modifier Serveur frp |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | Supprimer Serveur frp |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | Activer ou désactiver Serveur frp |
| PUT | `/api/frp/servers-order` | `frp:write` | Réordonner Serveur frp |
| PUT | `/api/frp/servers-group` | `frp:write` | Définir le groupe de plusieurs éléments : Serveur frp |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | Afficher les secrets en clair |
| GET | `/api/frp/proxies` | `frp:read` | Lister proxys frp |
| POST | `/api/frp/proxies` | `frp:write` | Ajouter proxys frp |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | Modifier proxys frp |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | Supprimer proxys frp |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | Activer ou désactiver proxys frp |
| PUT | `/api/frp/proxies-order` | `frp:write` | Réordonner proxys frp |
| PUT | `/api/frp/proxies-group` | `frp:write` | Définir le groupe de plusieurs éléments : proxys frp |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | Afficher les secrets en clair |
| POST | `/api/frp/import` | `frp:write` | Importer une configuration frpc / frps existante (anciens ini et nouveaux toml pris en charge) ; aperçu uniquement si apply vaut false |
| GET | `/api/frp/install` | `frp:read` | Voir les informations d'installation et la progression d'un programme géré |
| POST | `/api/frp/install` | `frp:write` | Installer ou mettre à jour en ligne un programme géré |
| POST | `/api/frp/install/rollback` | `frp:write` | Revenir à une version précédente conservée |
| DELETE | `/api/frp/install` | `frp:write` | Désinstaller un programme géré |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | Supprimer une version précédente conservée |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | Redémarrer l'instance frpc |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | Redémarrer l'instance frps |
| GET | `/api/frp/settings` | `frp:read` | Lire les chemins et versions des programmes |
| PUT | `/api/frp/settings` | `frp:write` | Enregistrer les chemins des programmes |
| POST | `/api/frp/reset` | `frp:write` | Rétablir les paramètres par défaut du module |

## Tunnel Cloudflare

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | Lister Tunnel Cloudflare |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Ajouter Tunnel Cloudflare |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Modifier Tunnel Cloudflare |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Supprimer Tunnel Cloudflare |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Activer ou désactiver Tunnel Cloudflare |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Réordonner Tunnel Cloudflare |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Définir le groupe de plusieurs éléments : Tunnel Cloudflare |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | Afficher les secrets en clair |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Redémarrer un tunnel Cloudflare |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | Lire les routes d'un tunnel à jeton |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | Enregistrer les routes d'un tunnel à jeton |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Lire les paramètres du tunnel Cloudflare |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Enregistrer les paramètres du tunnel Cloudflare |
| GET | `/api/cloudflared/install` | `cloudflared:read` | Voir les informations d'installation et la progression d'un programme géré |
| POST | `/api/cloudflared/install` | `cloudflared:write` | Installer ou mettre à jour en ligne un programme géré |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | Revenir à une version précédente conservée |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | Désinstaller un programme géré |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | Supprimer une version précédente conservée |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | Rétablir les paramètres par défaut du module |

## Certificat SSL

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/ssl/logs` | `ssl:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/ssl/certs` | `ssl:read` | Lister Certificat |
| POST | `/api/ssl/certs` | `ssl:write` | Ajouter Certificat |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | Modifier Certificat |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | Supprimer Certificat |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | Activer ou désactiver Certificat |
| PUT | `/api/ssl/certs-order` | `ssl:write` | Réordonner Certificat |
| PUT | `/api/ssl/certs-group` | `ssl:write` | Définir le groupe de plusieurs éléments : Certificat |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | Afficher les secrets en clair |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | Émettre ou renouveler un certificat immédiatement |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | Télécharger le certificat et la clé privée |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | Déployer un certificat maintenant |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | Lister les cibles de déploiement disponibles |
| GET | `/api/ssl/providers` | `ssl:read` | Lister les fournisseurs de validation DNS pris en charge |
| GET | `/api/ssl/settings` | `ssl:read` | Lire les paramètres globaux des certificats |
| PUT | `/api/ssl/settings` | `ssl:write` | Enregistrer les paramètres globaux des certificats |

## Pare-feu WAF

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/waf/logs` | `waf:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/waf/instances` | `waf:read` | Lister Instance WAF |
| POST | `/api/waf/instances` | `waf:write` | Ajouter Instance WAF |
| PUT | `/api/waf/instances/{key}` | `waf:write` | Modifier Instance WAF |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | Supprimer Instance WAF |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | Activer ou désactiver Instance WAF |
| PUT | `/api/waf/instances-order` | `waf:write` | Réordonner Instance WAF |
| PUT | `/api/waf/instances-group` | `waf:write` | Définir le groupe de plusieurs éléments : Instance WAF |
| GET | `/api/waf/crs` | `waf:read` | Lister les fichiers de règles OWASP CRS |
| GET | `/api/waf/crs/{file}` | `waf:read` | Lire un fichier de règles CRS |

## Service Web

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/webservice/logs` | `webservice:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/webservice/listeners` | `webservice:read` | Lister Écoute du service Web |
| POST | `/api/webservice/listeners` | `webservice:write` | Ajouter Écoute du service Web |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Modifier Écoute du service Web |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Supprimer Écoute du service Web |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Activer ou désactiver Écoute du service Web |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Réordonner Écoute du service Web |
| GET | `/api/webservice/iplists` | `webservice:read` | Lister les listes d’IP abonnées |
| POST | `/api/webservice/iplists` | `webservice:write` | Ajouter Liste d'IP abonnée |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | Modifier Liste d'IP abonnée |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | Supprimer Liste d'IP abonnée |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | Activer ou désactiver Liste d'IP abonnée |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | Réordonner Liste d'IP abonnée |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | Actualiser maintenant une liste d'IP abonnée |
| GET | `/api/webservice/users` | `webservice:read` | Lire les utilisateurs partagés et le fournisseur d'identité OIDC |
| PUT | `/api/webservice/users` | `webservice:write` | Enregistrer utilisateurs partagés et fournisseur d'identité OIDC, configurer les sites en masse |
| POST | `/api/webservice/import/nginx` | `webservice:write` | Importer une configuration nginx |
| GET | `/api/webservice/settings` | `webservice:read` | Lire les paramètres globaux du service Web |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | Afficher en clair les secrets TOTP des utilisateurs partagés (indexés par nom d'utilisateur) |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | Afficher en clair les Client Secret des connexions tierces (indexés par identifiant de méthode) |
| POST | `/api/webservice/totp-secret` | `webservice:write` | Générer une nouvelle clé TOTP pour un utilisateur partagé (non enregistrée ; effective après enregistrement) |
| PUT | `/api/webservice/settings` | `webservice:write` | Enregistrer les paramètres globaux du service Web |
| GET | `/api/webservice/stats` | `webservice:read` | Statistiques par minute du site (?listener=&site=&minutes=) |
| GET | `/api/webservice/metrics` | `webservice:read` | Métriques au format texte Prometheus |
| GET | `/api/webservice/bans` | `webservice:read` | IP actuellement bannies |
| POST | `/api/webservice/bans` | `webservice:write` | Bannir une IP manuellement |
| DELETE | `/api/webservice/bans` | `webservice:write` | Lever un bannissement (le body est {"key": adresse}) |
| GET | `/api/webservice/geoip` | `webservice:read` | État de la base GeoIP |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | Téléverser la base GeoIP (champ multipart file, .mmdb) |
| POST | `/api/webservice/geoip/download` | `webservice:write` | Télécharger la base GeoIP (.mmdb) depuis une adresse |
| DELETE | `/api/webservice/geoip` | `webservice:write` | Supprimer la base GeoIP |

## Prévention d'intrusion

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/guard/logs` | `guard:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/guard/jails` | `guard:read` | Lister règle de prévention des intrusions |
| POST | `/api/guard/jails` | `guard:write` | Ajouter règle de prévention des intrusions |
| PUT | `/api/guard/jails/{key}` | `guard:write` | Modifier règle de prévention des intrusions |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | Supprimer règle de prévention des intrusions |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | Activer ou désactiver règle de prévention des intrusions |
| PUT | `/api/guard/jails-order` | `guard:write` | Réordonner règle de prévention des intrusions |
| PUT | `/api/guard/jails-group` | `guard:write` | Définir le groupe de plusieurs éléments : règle de prévention des intrusions |
| GET | `/api/guard/presets` | `guard:read` | Modèles de règles pour les services courants |
| POST | `/api/guard/jails/test` | `guard:write` | Tester les filtres sur des lignes de journal |
| GET | `/api/guard/status` | `guard:read` | État de la synchronisation du pare-feu et environnement |
| GET | `/api/guard/settings` | `guard:read` | Paramètres globaux |
| PUT | `/api/guard/settings` | `guard:write` | Modifier les paramètres globaux |
| GET | `/api/guard/bans` | `guard:read` | IP actuellement bannies |
| POST | `/api/guard/bans` | `guard:write` | Bannir manuellement une IP ou un réseau |
| DELETE | `/api/guard/bans` | `guard:write` | Lever un bannissement (le body est {"key": adresse}) |
| GET | `/api/guard/firewall/rules` | `guard:read` | Règles écrites par Gately dans nftables (texte) |

## Wake-on-LAN

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/wol/logs` | `wol:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/wol/devices` | `wol:read` | Lister Réveiller l'appareil |
| POST | `/api/wol/devices` | `wol:write` | Ajouter Réveiller l'appareil |
| PUT | `/api/wol/devices/{key}` | `wol:write` | Modifier Réveiller l'appareil |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | Supprimer Réveiller l'appareil |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | Activer ou désactiver Réveiller l'appareil |
| PUT | `/api/wol/devices-order` | `wol:write` | Réordonner Réveiller l'appareil |
| PUT | `/api/wol/devices-group` | `wol:write` | Définir le groupe de plusieurs éléments : Réveiller l'appareil |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | Envoyer un paquet magique de réveil à l'appareil |
| GET | `/api/wol/status` | `wol:read` | Consulter l'état en ligne des appareils |

## Terminal Web

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/webterminal/connections` | `webterminal:read` | Lister Connexion de terminal |
| POST | `/api/webterminal/connections` | `webterminal:write` | Ajouter Connexion de terminal |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | Modifier Connexion de terminal |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | Supprimer Connexion de terminal |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | Activer ou désactiver Connexion de terminal |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | Réordonner Connexion de terminal |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | Définir le groupe de plusieurs éléments : Connexion de terminal |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | Afficher les secrets en clair |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | Effacer la clé d'hôte enregistrée |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | Tester une connexion enregistrée |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | Tester des paramètres de connexion temporaires |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | Accès rapide pour définir la connexion |
| GET | `/api/webterminal/credentials` | `webterminal:read` | Lister Entrée |
| POST | `/api/webterminal/credentials` | `webterminal:write` | Ajouter Entrée |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | Modifier Entrée |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | Supprimer Entrée |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | Activer ou désactiver Entrée |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | Réordonner Entrée |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | Afficher les secrets en clair |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | Ouvrir une session de terminal (WebSocket) |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | Se rattacher à une session de terminal existante (WebSocket) |
| GET | `/api/webterminal/sessions` | `webterminal:read` | Lister les sessions de terminal |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | Consulter les détails d'une session de terminal |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | Modifier la note d'une session de terminal |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | Consulter les statistiques des sessions de terminal |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | Fermer la session de terminal |
| GET | `/api/webterminal/security` | Session connectée uniquement | Consulter l'état des paramètres de sécurité du terminal |
| POST | `/api/webterminal/security/agreement` | Session connectée uniquement | Accepter le contrat d'utilisation du terminal |
| POST | `/api/webterminal/security/2fa/setup` | Session connectée uniquement | Générer une clé d'authentification à deux facteurs du terminal |
| POST | `/api/webterminal/security/2fa/enable` | Session connectée uniquement | Activer l'authentification à deux facteurs du terminal |
| POST | `/api/webterminal/security/2fa/disable` | Session connectée uniquement | Désactiver l'authentification à deux facteurs du terminal |
| POST | `/api/webterminal/security/verify2fa` | Session connectée uniquement | Vérifier le code d'authentification à deux facteurs du terminal |
| GET | `/api/webterminal/security/check2fa` | Session connectée uniquement | Vérifier si l'authentification à deux facteurs du terminal est validée |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | Lister le répertoire distant |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | Lire un fichier distant |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | Écrire un fichier distant |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | Téléverser un fichier vers l'hôte distant |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | Créer un répertoire distant |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | Créer un fichier vide distant |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | Renommer ou déplacer un fichier distant |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | Copier un fichier distant |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | Modifier les permissions d'un fichier distant |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | Supprimer un fichier distant |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | Compresser des fichiers sur l'hôte distant |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | Décompresser des fichiers sur l'hôte distant |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | Prévisualiser le contenu d'une archive distante |
| GET | `/api/webterminal/settings` | `webterminal:read` | Lire les paramètres du terminal Web |
| PUT | `/api/webterminal/settings` | `webterminal:write` | Enregistrer les paramètres du terminal Web |
| GET | `/api/webterminal/shells` | `webterminal:read` | Lister les shells disponibles sur cette machine |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | Lire les commandes rapides globales |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | Enregistrer les commandes rapides globales |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | Lire la disposition en écran partagé |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | Enregistrer la disposition en écran partagé |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | Effacer la disposition en écran partagé |

## Tâche planifiée

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/cron/logs` | `cron:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/cron/tasks` | `cron:read` | Lister Tâche planifiée |
| POST | `/api/cron/tasks` | `cron:write` | Ajouter Tâche planifiée |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | Modifier Tâche planifiée |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | Supprimer Tâche planifiée |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | Activer ou désactiver Tâche planifiée |
| PUT | `/api/cron/tasks-order` | `cron:write` | Réordonner Tâche planifiée |
| PUT | `/api/cron/tasks-group` | `cron:write` | Définir le groupe de plusieurs éléments : Tâche planifiée |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | Exécuter immédiatement une tâche planifiée |
| POST | `/api/cron/parse` | `cron:write` | Analyser et prévisualiser une expression Cron |

## Gestion Docker

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/docker/logs` | `docker:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/docker/tasks` | `docker:read` | Lister les tâches d'arrière-plan |
| GET | `/api/docker/tasks/{id}` | `docker:read` | Consulter la progression et la sortie d'une tâche d'arrière-plan |
| DELETE | `/api/docker/tasks` | `docker:write` | Nettoyer les tâches d'arrière-plan terminées |
| GET | `/api/docker/images` | `docker:read` | Lister les images |
| POST | `/api/docker/images/pull` | `docker:write` | Télécharger une image |
| POST | `/api/docker/images/push` | `docker:write` | Pousser une image |
| POST | `/api/docker/images/search` | `docker:write` | Rechercher des images |
| POST | `/api/docker/images/load` | `docker:write` | Importer un fichier d'image |
| POST | `/api/docker/images/build` | `docker:write` | Construire une image |
| GET | `/api/docker/images/{id}` | `docker:read` | Consulter les détails d'une image |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | Étiqueter l'image |
| DELETE | `/api/docker/images/{id}` | `docker:write` | Supprimer une image |
| GET | `/api/docker/images/{id}/export` | `docker:write` | Exporter une image |
| GET | `/api/docker/networks` | `docker:read` | Lister les réseaux |
| POST | `/api/docker/networks` | `docker:write` | Créer un réseau |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | Supprimer un réseau |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | Connecter le conteneur à un réseau |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | Déconnecter le conteneur d'un réseau |
| GET | `/api/docker/volumes` | `docker:read` | Lister les volumes |
| POST | `/api/docker/volumes` | `docker:write` | Créer un volume |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | Supprimer un volume |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | Exporter un volume |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | Importer des données dans un volume |
| GET | `/api/docker/compose/projects` | `docker:read` | Lister les projets Compose |
| POST | `/api/docker/compose/projects` | `docker:write` | Créer un projet Compose |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | Démarrer un projet Compose |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | Télécharger les images d'un projet Compose |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | Exécuter une action sur un projet Compose |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | Lire le fichier de configuration Compose |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | Enregistrer le fichier de configuration Compose |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | Consulter les journaux d'un projet Compose |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | Supprimer un projet Compose |
| GET | `/api/docker/info` | `docker:read` | Consulter les informations du moteur Docker |
| GET | `/api/docker/disk-usage` | `docker:read` | Consulter l'espace disque utilisé par Docker |
| POST | `/api/docker/prune` | `docker:write` | Nettoyer les ressources Docker inutiles |
| GET | `/api/docker/settings` | `docker:read` | Lire les paramètres du module Docker |
| PUT | `/api/docker/settings` | `docker:write` | Enregistrer les paramètres du module Docker |
| GET | `/api/docker/settings/secrets` | `docker:write` | Afficher en clair les mots de passe des registres (clés `registry:<adresse du registre>`) |
| GET | `/api/docker/containers` | `docker:read` | Lister les conteneurs |
| POST | `/api/docker/containers` | `docker:write` | Créer un conteneur |
| POST | `/api/docker/containers/check-all` | `docker:write` | Vérifier les mises à jour de toutes les images de conteneurs |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | Mettre à niveau des conteneurs en masse |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | Vérifier si une image a une nouvelle version |
| GET | `/api/docker/containers/{id}` | `docker:read` | Consulter les détails d'un conteneur |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | Afficher en clair les variables d'environnement et la commande de démarrage d'un conteneur |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | Supprimer un conteneur |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | Modifier la configuration du conteneur et le recréer |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | Renommer un conteneur |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | Valider le conteneur comme image |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | Vérifier si l'image d'un conteneur a une mise à jour |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | Mettre à niveau l'image du conteneur et le recréer |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | Exécuter une action sur un conteneur |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | Consulter les journaux d'un conteneur |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | Consulter l'utilisation des ressources d'un conteneur |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | Consulter les processus d'un conteneur |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | Lister le répertoire dans le conteneur |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | Lire un fichier dans le conteneur |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | Télécharger un fichier du conteneur |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | Écrire un fichier dans le conteneur |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | Créer un répertoire dans le conteneur |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | Renommer un fichier dans le conteneur |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | Téléverser un fichier vers le conteneur |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | Supprimer un fichier dans le conteneur |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | Ouvrir un terminal dans le conteneur (WebSocket) |

## Service WebDAV

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/webdav/logs` | `webdav:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/webdav/settings` | `webdav:read` | Lire les paramètres WebDAV |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | Afficher en clair les mots de passe des utilisateurs locaux |
| GET | `/api/webdav/shared-users` | `webdav:read` | Lire les utilisateurs partagés des services de fichiers (ancien point d’accès) |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | Enregistrer les utilisateurs partagés des services de fichiers (ancien point d’accès) |
| PUT | `/api/webdav/settings` | `webdav:write` | Enregistrer les paramètres WebDAV |

## Service FTP

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/ftp/logs` | `ftp:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/ftp/settings` | `ftp:read` | Lire les paramètres FTP |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | Afficher en clair les mots de passe des utilisateurs locaux |
| GET | `/api/ftp/shared-users` | `ftp:read` | Lire les utilisateurs partagés des services de fichiers (ancien point d’accès) |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | Enregistrer les utilisateurs partagés des services de fichiers (ancien point d’accès) |
| PUT | `/api/ftp/settings` | `ftp:write` | Enregistrer les paramètres FTP |

## Service SMB

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/smb/logs` | `smb:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/smb/settings` | `smb:read` | Lire les paramètres SMB |
| GET | `/api/smb/settings/secrets` | `smb:write` | Afficher en clair les mots de passe des utilisateurs locaux |
| GET | `/api/smb/shared-users` | `smb:read` | Lire les utilisateurs partagés des services de fichiers (ancien point d’accès) |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | Enregistrer les utilisateurs partagés des services de fichiers (ancien point d’accès) |
| PUT | `/api/smb/settings` | `smb:write` | Enregistrer les paramètres SMB |

## File Browser

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | Lister Instance File Browser |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | Ajouter Instance File Browser |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Modifier Instance File Browser |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Supprimer Instance File Browser |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | Activer ou désactiver Instance File Browser |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | Réordonner Instance File Browser |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | Définir le groupe de plusieurs éléments : Instance File Browser |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | Afficher les secrets en clair |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | Redémarrer une instance File Browser |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | Lire les paramètres du programme File Browser |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | Enregistrer les paramètres du programme File Browser |
| GET | `/api/filebrowser/install` | `filebrowser:read` | Voir les informations d'installation et la progression d'un programme géré |
| POST | `/api/filebrowser/install` | `filebrowser:write` | Installer ou mettre à jour en ligne un programme géré |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | Revenir à une version précédente conservée |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | Désinstaller un programme géré |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | Supprimer une version précédente conservée |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | Rétablir les paramètres par défaut du module |

## Montages de stockage

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/storage/logs` | `storage:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/storage/mounts` | `storage:read` | Lister Montages de stockage |
| POST | `/api/storage/mounts` | `storage:write` | Ajouter Montages de stockage |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | Modifier Montages de stockage |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | Supprimer Montages de stockage |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | Activer ou désactiver Montages de stockage |
| PUT | `/api/storage/mounts-order` | `storage:write` | Réordonner Montages de stockage |
| PUT | `/api/storage/mounts-group` | `storage:write` | Définir le groupe de plusieurs éléments : Montages de stockage |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | Afficher les secrets en clair |
| GET | `/api/storage/files/{key}/list` | `storage:write` | Lister le répertoire de stockage |
| GET | `/api/storage/files/{key}/download` | `storage:write` | Télécharger un fichier du stockage |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | Téléverser un fichier vers le stockage |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | Créer un répertoire de stockage |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | Renommer ou déplacer un fichier de stockage |
| DELETE | `/api/storage/files/{key}` | `storage:write` | Supprimer un fichier du stockage |

## Centre de notifications

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/notify/logs` | `notify:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/notify/channels` | `notify:read` | Lister Canaux de notification |
| POST | `/api/notify/channels` | `notify:write` | Ajouter Canaux de notification |
| PUT | `/api/notify/channels/{key}` | `notify:write` | Modifier Canaux de notification |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | Supprimer Canaux de notification |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | Activer ou désactiver Canaux de notification |
| PUT | `/api/notify/channels-order` | `notify:write` | Réordonner Canaux de notification |
| PUT | `/api/notify/channels-group` | `notify:write` | Définir le groupe de plusieurs éléments : Canaux de notification |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | Afficher les secrets en clair |
| GET | `/api/notify/rules` | `notify:read` | Lister Règles de notification |
| POST | `/api/notify/rules` | `notify:write` | Ajouter Règles de notification |
| PUT | `/api/notify/rules/{key}` | `notify:write` | Modifier Règles de notification |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | Supprimer Règles de notification |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | Activer ou désactiver Règles de notification |
| PUT | `/api/notify/rules-order` | `notify:write` | Réordonner Règles de notification |
| PUT | `/api/notify/rules-group` | `notify:write` | Définir le groupe de plusieurs éléments : Règles de notification |
| GET | `/api/notify/event-types` | `notify:read` | Types d'événements auxquels s'abonner |
| GET | `/api/notify/history` | `notify:read` | Dernières livraisons |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | Envoyer une notification de test au canal |

## Diagnostic réseau

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/nettools/logs` | `nettools:write` | Vider les journaux d'exécution d'un module |
| POST | `/api/nettools/run` | `nettools:write` | Exécuter un outil de diagnostic et renvoyer la tâche (tool : ping / traceroute / tcp / dns / speed / scan) |
| GET | `/api/nettools/tasks` | `nettools:read` | Dernières tâches de diagnostic |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | Détails et sortie de la tâche |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | Annuler une tâche en cours ou supprimer une tâche terminée |
| GET | `/api/nettools/settings` | `nettools:read` | Paramètres de diagnostic |
| PUT | `/api/nettools/settings` | `nettools:write` | Modifier la liste des sondages interdits |
| GET | `/api/nettools/networks` | `nettools:read` | Segments de réseau locaux utilisables pour l'analyse du réseau local |

## Sauvegarde de configuration

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | Consulter les journaux d'exécution d'un module |
| DELETE | `/api/backup/logs` | `backup:write` | Vider les journaux d'exécution d'un module |
| GET | `/api/backup/targets` | `backup:read` | Lister Destination de sauvegarde |
| POST | `/api/backup/targets` | `backup:write` | Ajouter Destination de sauvegarde |
| PUT | `/api/backup/targets/{key}` | `backup:write` | Modifier Destination de sauvegarde |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | Supprimer Destination de sauvegarde |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | Activer ou désactiver Destination de sauvegarde |
| PUT | `/api/backup/targets-order` | `backup:write` | Réordonner Destination de sauvegarde |
| GET | `/api/backup/settings` | `backup:read` | Paramètres de sauvegarde |
| PUT | `/api/backup/settings` | `backup:write` | Modifier les paramètres de sauvegarde (un espace réservé pour la phrase secrète la laisse inchangée) |
| GET | `/api/backup/secrets` | `backup:write` | Afficher les secrets en clair |
| POST | `/api/backup/run` | `backup:write` | Lancer une sauvegarde immédiatement |
| GET | `/api/backup/status` | `backup:read` | Résultat de la dernière sauvegarde et heure de la prochaine exécution |
| GET | `/api/backup/mounts` | `backup:read` | Points de montage de stockage utilisables comme destinations de sauvegarde |
| GET | `/api/backup/local` | Session connectée uniquement | Liste des fichiers de sauvegarde locaux |
| GET | `/api/backup/local/{name}` | Session connectée uniquement | Télécharger un fichier de sauvegarde local |

## Utilisateurs partagés des services de fichiers

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | Lire les utilisateurs partagés des services de fichiers |
| PUT | `/api/fileusers` | `fileusers:write` | Enregistrer les utilisateurs partagés des services de fichiers |
| GET | `/api/fileusers/secrets` | `fileusers:write` | Afficher en clair les mots de passe des utilisateurs partagés |

## Système

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/session` | Tout jeton valide | Consulter l'identité authentifiée actuelle |
| GET | `/api/status` | `status:read` | Consulter l'état du système et des modules |
| GET | `/api/logs` | `logs:read` | Consulter les journaux système |
| GET | `/api/system/settings` | Session connectée uniquement | Lire les paramètres globaux |
| PUT | `/api/system/settings` | Session connectée uniquement | Enregistrer les paramètres globaux |
| GET | `/api/system/settings/secrets` | Session connectée uniquement | Afficher en clair les Client Secret de la connexion tierce |
| GET | `/api/system/info` | `status:read` | Consulter les informations système |
| GET | `/api/system/update` | `status:read` | Rechercher une nouvelle version |
| POST | `/api/system/upgrade` | Session connectée uniquement | Mettre à niveau en ligne vers la dernière version |
| GET | `/api/system/upgrade/status` | Session connectée uniquement | Voir la progression de la mise à niveau en ligne |
| POST | `/api/system/upgrade/cancel` | Session connectée uniquement | Annuler la mise à niveau en ligne |
| POST | `/api/system/upgrade/upload` | Session connectée uniquement | Mettre à niveau avec un paquet téléversé |
| GET | `/api/system/backup` | Session connectée uniquement | Télécharger la sauvegarde de configuration |
| GET | `/api/system/diagnostics` | Session connectée uniquement | Exporter le paquet de diagnostic anonymisé |
| GET | `/api/audit` | Session connectée uniquement | Consulter le journal d'audit |
| GET | `/api/metrics` | `metrics:read` | Métriques Prometheus globales |
| GET | `/api/history` | Session connectée uniquement | Lister les versions de l'historique de configuration |
| GET | `/api/history/{name}/{id}/diff` | Session connectée uniquement | Comparer une version de l'historique avec la configuration actuelle |
| POST | `/api/history/{name}/{id}/restore` | Session connectée uniquement | Restaurer une configuration à une version de l'historique |
| POST | `/api/system/restore` | Session connectée uniquement | Restaurer la configuration depuis une sauvegarde |
| POST | `/api/system/restart` | Session connectée uniquement | Redémarrer Gately |
| POST | `/api/system/selfcheck` | Session connectée uniquement | Autodiagnostic du réseau et de la configuration |
| GET | `/api/system/onboarding` | Session connectée uniquement | Indique si l'assistant de premier démarrage est en attente |
| POST | `/api/system/onboarding/dismiss` | Session connectée uniquement | Terminer ou ignorer l'assistant de premier démarrage |
| GET | `/api/openapi.json` | Tout jeton valide | Obtenir la description OpenAPI de cette API |

## Import de configuration

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | Importer la configuration Lucky |

## Jeton API

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| GET | `/api/tokens` | Session connectée uniquement | Lister les jetons API |
| GET | `/api/tokens/scopes` | Session connectée uniquement | Lister les domaines de permission attribuables |
| POST | `/api/tokens` | Session connectée uniquement | Créer un jeton API |
| PUT | `/api/tokens/order` | Session connectée uniquement | Réordonner les jetons d'API |
| PUT | `/api/tokens/group` | Session connectée uniquement | Définir le groupe de plusieurs jetons d'API |
| PUT | `/api/tokens/{id}` | Session connectée uniquement | Modifier le nom, le groupe, les permissions, l'expiration et l'état d'un jeton d'API |
| POST | `/api/tokens/{id}/regenerate` | Session connectée uniquement | Régénérer le jeton en clair |
| DELETE | `/api/tokens/{id}` | Session connectée uniquement | Supprimer un jeton API |

## Compte

| Méthode | Chemin | Permission requise | Description |
| --- | --- | --- | --- |
| PUT | `/api/account` | Session connectée uniquement | Modifier le nom d'utilisateur et le mot de passe de l'administrateur |
| PUT | `/api/account/ui-language` | Session connectée uniquement | Enregistrer la langue de la console d'administration |
| GET | `/api/account/passkeys` | Session connectée uniquement | Lister les Passkeys enregistrées |
| POST | `/api/account/passkeys/begin` | Session connectée uniquement | Commencer l'enregistrement d'une Passkey |
| POST | `/api/account/passkeys` | Session connectée uniquement | Terminer l'enregistrement de la Passkey |
| DELETE | `/api/account/passkeys/{id}` | Session connectée uniquement | Supprimer une Passkey |
| GET | `/api/account/users` | Session connectée uniquement | Lister les comptes en lecture seule |
| POST | `/api/account/users` | Session connectée uniquement | Créer un compte en lecture seule |
| PUT | `/api/account/users/{name}` | Session connectée uniquement | Réinitialiser le mot de passe d'un compte en lecture seule |
| DELETE | `/api/account/users/{name}` | Session connectée uniquement | Supprimer un compte en lecture seule |
| GET | `/api/account/totp` | Session connectée uniquement | Consulter l'état de l'authentification à deux facteurs |
| POST | `/api/account/totp/setup` | Session connectée uniquement | Générer une clé d'authentification à deux facteurs |
| POST | `/api/account/totp/enable` | Session connectée uniquement | Activer l'authentification à deux facteurs |
| POST | `/api/account/totp/disable` | Session connectée uniquement | Désactiver l'authentification à deux facteurs |

## Structures de données

### backup.LocalFile

| Champ | Type |
| --- | --- |
| `modTime` | Heure (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| Champ | Type |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| Champ | Type |
| --- | --- |
| `lastRun` | Heure (RFC 3339) |
| `manual` | boolean |
| `nextRun` | Heure (RFC 3339) |
| `results` | Tableau de `backup.Result` |
| `running` | boolean |

### backup.Target

| Champ | Type |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| Champ | Type |
| --- | --- |
| `count` | integer |
| `created` | Heure (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Heure (RFC 3339) |

### cloudflared.Tunnel

| Champ | Type |
| --- | --- |
| `apiToken` | string |
| `edgeIpVersion` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `mode` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `service` | string |
| `token` | string |

### cron.Task

| Champ | Type |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `group` | string |
| `http` | `webhook.Config` |
| `key` | string |
| `name` | string |
| `remark` | string |
| `spec` | string |
| `type` | string |

### crud.enableRequest

| Champ | Type |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| Champ | Type |
| --- | --- |
| `group` | string |
| `keys` | Tableau de string |

### crud.orderRequest

| Champ | Type |
| --- | --- |
| `keys` | Tableau de string |

### ddns.Record

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `line` | string |
| `remark` | string |
| `sub` | string |
| `ttl` | integer |
| `type` | string |
| `value` | string |
| `zone` | string |

### ddns.Task

| Champ | Type |
| --- | --- |
| `credentials` | `dnsprovider.Credentials` |
| `enable` | boolean |
| `forceInterval` | integer |
| `group` | string |
| `httpTimeout` | integer |
| `interval` | integer |
| `ipv4` | `ipsource.Source` |
| `ipv6` | `ipsource.Source` |
| `key` | string |
| `name` | string |
| `provider` | string |
| `records` | Tableau de `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| Champ | Type |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| Champ | Type |
| --- | --- |
| `records` | Tableau de `ddns.recordLayout` |

### dnsprovider.Credentials

| Champ | Type |
| --- | --- |
| `extra` | Table de correspondance de string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| Champ | Type |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| Champ | Type |
| --- | --- |
| `accessUrls` | Tableau de string |
| `adminPassword` | string |
| `adminUser` | string |
| `authMethod` | string |
| `baseUrl` | string |
| `disableWebDav` | boolean |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenIp` | string |
| `locale` | string |
| `name` | string |
| `port` | integer |
| `proxyHeader` | string |
| `remark` | string |
| `sources` | Tableau de `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| Champ | Type |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| Champ | Type |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ClientSettings

| Champ | Type |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| Champ | Type |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| Champ | Type |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | Tableau de `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | Tableau de string |

### frp.Proxy

| Champ | Type |
| --- | --- |
| `client` | string |
| `customDomains` | Tableau de string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `localIp` | string |
| `localPort` | integer |
| `name` | string |
| `remark` | string |
| `remotePort` | integer |
| `secretKey` | string |
| `subdomain` | string |
| `type` | string |

### frp.Server

| Champ | Type |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| Champ | Type |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| Champ | Type |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| Champ | Type |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| Champ | Type |
| --- | --- |
| `buildTime` | Heure (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| Champ | Type |
| --- | --- |
| `count` | integer |
| `created` | Heure (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Heure (RFC 3339) |

### guard.FirewallStatus

| Champ | Type |
| --- | --- |
| `active` | boolean |
| `checkedAt` | Heure (RFC 3339) |
| `dropped` | Table de correspondance de integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | Table de correspondance de integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | Tableau de integer |
| `version` | string |

### guard.Jail

| Champ | Type |
| --- | --- |
| `anyStatus` | boolean |
| `banTime` | integer |
| `enable` | boolean |
| `filter` | string |
| `findTime` | integer |
| `group` | string |
| `ignore` | string |
| `key` | string |
| `maxRetry` | integer |
| `name` | string |
| `preset` | string |
| `remark` | string |
| `source` | string |
| `target` | string |
| `unknownHost` | boolean |

### guard.Platform

| Champ | Type |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| Champ | Type |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `threshold` | integer |

### guard.Settings

| Champ | Type |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | Tableau de string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| Champ | Type |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | Tableau de integer |

### guard.TestResult

| Champ | Type |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| Champ | Type |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| Champ | Type |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| Champ | Type |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| Champ | Type |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | Tableau de string |

### nettools.Request

| Champ | Type |
| --- | --- |
| `cidr` | string |
| `count` | integer |
| `ports` | string |
| `seconds` | integer |
| `server` | string |
| `target` | string |
| `tool` | string |
| `type` | string |
| `url` | string |

### nettools.Settings

| Champ | Type |
| --- | --- |
| `deny` | Tableau de string |

### nettools.Task

| Champ | Type |
| --- | --- |
| `error` | string |
| `finished` | Heure (RFC 3339) |
| `id` | string |
| `lines` | Tableau de string |
| `result` | Tous |
| `started` | Heure (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| Champ | Type |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | Heure (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| Champ | Type |
| --- | --- |
| `channels` | Tableau de string |
| `enable` | boolean |
| `events` | Tableau de string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### portforward.Rule

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | Tableau de string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | Tableau de string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### protoroute.Route

| Champ | Type |
| --- | --- |
| `alpn` | Tableau de string |
| `domains` | Tableau de string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | Tableau de string |
| `targetPort` | integer |

### protoroute.Rule

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | Tableau de string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | Tableau de `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | Tableau de string |
| `targetPorts` | string |

### ssl.ACMEConfig

| Champ | Type |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `domains` | Tableau de string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | Tableau de string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| Champ | Type |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | Tableau de `ssl.Deploy` |
| `enable` | boolean |
| `group` | string |
| `issuedProfile` | string |
| `issuerPem` | string |
| `key` | string |
| `keyFile` | string |
| `keyPem` | string |
| `mapping` | `ssl.Mapping` |
| `remark` | string |
| `source` | string |
| `updatedAt` | Heure (RFC 3339) |

### ssl.Deploy

| Champ | Type |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | Tableau de string |
| `deployedAt` | Heure (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| Champ | Type |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| Champ | Type |
| --- | --- |
| `domain` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `password` | string |
| `path` | string |
| `readOnly` | boolean |
| `remark` | string |
| `skipVerify` | boolean |
| `type` | string |
| `url` | string |
| `username` | string |

### storage.MountInfo

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| Champ | Type |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| Champ | Type |
| --- | --- |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `keepAliveServers` | Tableau de string |
| `key` | string |
| `localPort` | integer |
| `maxConnections` | integer |
| `name` | string |
| `natpmp` | boolean |
| `natpmpGateway` | string |
| `protocol` | string |
| `remark` | string |
| `retryInterval` | integer |
| `script` | string |
| `scriptEnable` | boolean |
| `stunServers` | Tableau de string |
| `stunTimeout` | integer |
| `targetAddresses` | Tableau de string |
| `targetPort` | integer |
| `udpSessionTimeout` | integer |
| `upnp` | boolean |
| `upnpControlUrl` | string |
| `upnpGateway` | string |
| `upnpInternalPort` | integer |
| `upnpLocalHost` | string |
| `useGlobalServers` | boolean |
| `useGlobalWebhook` | boolean |
| `webhook` | `webhook.Config` |
| `webhookOnlyOnChange` | boolean |

### thirdauth.Provider

| Champ | Type |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| Champ | Type |
| --- | --- |
| `bodyLimitAction` | string |
| `bodyLimitMB` | integer |
| `enable` | boolean |
| `group` | string |
| `inboundThreshold` | integer |
| `key` | string |
| `mode` | string |
| `name` | string |
| `outboundThreshold` | integer |
| `paranoia` | integer |
| `remark` | string |
| `ruleSets` | Tableau de `waf.RuleSet` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### webhook.Config

| Champ | Type |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | Tableau de string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `successContent` | Tableau de string |
| `timeout` | integer |
| `url` | string |

### webservice.AuthUser

| Champ | Type |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | Tableau de string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BanInfo

| Champ | Type |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | Heure (RFC 3339) |

### webservice.CacheConfig

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `ttl` | integer |

### webservice.ForwardAuth

| Champ | Type |
| --- | --- |
| `copyHeaders` | Tableau de string |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| Champ | Type |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | Heure (RFC 3339) |
| `url` | string |

### webservice.Listener

| Champ | Type |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | Tableau de `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| Champ | Type |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | Tableau de string |
| `targets` | Tableau de string |

### webservice.OIDCClient

| Champ | Type |
| --- | --- |
| `allow` | Tableau de string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | Tableau de string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| Champ | Type |
| --- | --- |
| `clients` | Tableau de `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.PathRule

| Champ | Type |
| --- | --- |
| `auth` | string |
| `ipList` | Tableau de string |
| `ipMode` | string |
| `ipSets` | Tableau de string |
| `prefix` | string |

### webservice.Point

| Champ | Type |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.RateLimit

| Champ | Type |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.Rewrite

| Champ | Type |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| Champ | Type |
| --- | --- |
| `authSecret` | string |
| `authUsers` | Tableau de `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | Tableau de `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | Tableau de string |

### webservice.ShareLink

| Champ | Type |
| --- | --- |
| `expiresAt` | Heure (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| Champ | Type |
| --- | --- |
| `accessLog` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | Tableau de string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | Tableau de string |
| `enable` | boolean |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `insecureSkipVerify` | boolean |
| `ipList` | Tableau de string |
| `ipMode` | string |
| `ipSets` | Tableau de string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | Tableau de `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | Tableau de string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | Tableau de string |
| `responseHeaders` | Tableau de string |
| `rewrites` | Tableau de `webservice.Rewrite` |
| `root` | string |
| `rules` | Tableau de `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | Tableau de `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | Tableau de string |
| `text` | string |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | Tableau de `webservice.UserRoute` |
| `users` | Tableau de string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteSharedUsers

| Champ | Type |
| --- | --- |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| Champ | Type |
| --- | --- |
| `bytesOut` | integer |
| `series` | Tableau de `webservice.Point` |
| `status` | Table de correspondance de integer |
| `topCountries` | Tableau de `webservice.TopItem` |
| `topIps` | Tableau de `webservice.TopItem` |
| `upstreams` | Tableau de `webservice.UpstreamStatus` |

### webservice.TopItem

| Champ | Type |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.UpstreamStatus

| Champ | Type |
| --- | --- |
| `backup` | boolean |
| `conns` | integer |
| `healthy` | boolean |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| Champ | Type |
| --- | --- |
| `targets` | Tableau de string |
| `users` | Tableau de string |

### webservice.UserSite

| Champ | Type |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | Tableau de string |
| `enable` | boolean |
| `forwardAuth` | boolean |
| `group` | string |
| `listener` | string |
| `listenerEnable` | boolean |
| `listenerName` | string |
| `name` | string |
| `otherAuth` | boolean |
| `port` | integer |
| `sharedUsers` | boolean |
| `site` | string |
| `tls` | boolean |
| `type` | string |
| `webAuth` | boolean |

### webservice.UsersRequest

| Champ | Type |
| --- | --- |
| `authUsers` | Tableau de `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Tableau de `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| Champ | Type |
| --- | --- |
| `authUsers` | Tableau de `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Tableau de `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| Champ | Type |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.geoipDownload

| Champ | Type |
| --- | --- |
| `url` | string |

### webservice.totpRequest

| Champ | Type |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| Champ | Type |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| Champ | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | Tableau de string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| Champ | Type |
| --- | --- |
| `authType` | string |
| `enable` | boolean |
| `key` | string |
| `keyFile` | string |
| `name` | string |
| `passphrase` | string |
| `password` | string |
| `privateKey` | string |
| `remark` | string |
| `username` | string |

### webterminal.LocalConfig

| Champ | Type |
| --- | --- |
| `env` | Table de correspondance de string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| Champ | Type |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| Champ | Type |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
| `keepAlive` | integer |
| `keyFile` | string |
| `passphrase` | string |
| `password` | string |
| `port` | integer |
| `privateKey` | string |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |
| `workDir` | string |

### webterminal.TelnetConfig

| Champ | Type |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.Device

| Champ | Type |
| --- | --- |
| `broadcastIps` | Tableau de string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | Tableau de string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |

