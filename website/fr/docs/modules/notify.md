# Centre de notifications

Le Centre de notifications s'abonne au bus d'événements interne de Gately et, selon vos règles, envoie des événements tels que les changements DDNS, l'expiration des certificats, les blocages WAF et les défaillances de processus vers Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu et e-mail. L'interface d'administration se trouve dans le groupe « Système » de la barre latérale → « Notifications ».

## Fonctionnement

1. Chaque module publie des événements sur le bus d'événements du processus lorsque son état change. La publication ne bloque jamais la logique métier.
2. Lorsque le Centre de notifications reçoit un événement, il examine les règles une par une : la règle est activée, le niveau de l'événement n'est pas inférieur au niveau minimal de la règle, et le type de l'événement correspond à la liste d'événements de la règle.
3. Une fois qu'une règle correspondante a passé la déduplication par silence, le message est remis aux canaux activés sélectionnés par la règle. Si un même événement correspond à plusieurs règles, chaque canal n'est sollicité qu'une seule fois.
4. Le résultat de chaque envoi (succès, ou motif de l'échec) est écrit dans « Envois récents », qui conserve les 200 dernières entrées en mémoire. Les échecs sont aussi écrits dans le journal du module.

Un envoi unique expire après 45 secondes.

## Langue des notifications

Le contenu des notifications et les journaux utilisent la langue du serveur définie dans « Paramètres système ». Lorsque la langue du serveur est « Suivre le système », Gately utilise la langue indiquée par la variable d'environnement `GATELY_LANG`, puis par `LANG`. Les services sous Docker, fnOS et OpenWrt ne définissent généralement aucune de ces deux variables ; la langue suit alors celle de l'interface utilisée par l'administrateur lors de sa dernière connexion ou de son dernier changement dans la barre supérieure, et l'anglais est utilisé à défaut. Les connexions des comptes en lecture seule ne modifient pas cette langue. La « Source » d'une notification est traduite lorsqu'il s'agit d'un nom généré par le programme (comme « Interface d'administration ») ; les noms de tâches et autres noms saisis par l'utilisateur restent tels quels.

## Canaux

Cliquez sur « Ajouter un canal », choisissez un type et renseignez les champs. Chaque canal possède un nom (obligatoire), une remarque et un interrupteur d'activation. Les champs secrets ne sont plus affichés après l'enregistrement ; les laisser vides lors d'une modification conserve la valeur d'origine.

| Type | Champs obligatoires | Champs facultatifs |
| --- | --- | --- |
| Webhook | URL de la requête | Méthode de requête (GET par défaut ; GET, POST, PUT ou PATCH), en-têtes (un `Key: Value` par ligne), corps, contenu de succès attendu, délai (secondes, 20 par défaut), ignorer la vérification du certificat |
| Telegram | Bot Token, Chat ID | URL de l'API (laissez vide pour utiliser `https://api.telegram.org`) |
| Bark | Device Key | URL du serveur (laissez vide pour utiliser `https://api.day.app`) |
| WeCom | URL du bot | Aucun |
| DingTalk | URL du bot | Secret de signature (à renseigner lorsque le bot a la « signature » activée) |
| Feishu | URL du bot | Secret de signature (à renseigner lorsque le bot a la « vérification de signature » activée) |
| E-mail | Serveur, destinataires, et au moins l'un de l'expéditeur ou du nom d'utilisateur | Chiffrement, port, nom d'utilisateur, mot de passe |
| ntfy | Sujet (topic) | URL du serveur (laissez vide pour utiliser `https://ntfy.sh`), jeton d'accès |
| Gotify | URL du serveur, jeton d'application | Aucun |
| Pushover | Jeton d'application, User Key | Aucun |
| Discord | URL du webhook | Aucun |
| Slack | URL du webhook | Aucun |
| ServerChan | SendKey | Aucun |
| PushPlus | Token | Aucun |

Les champs d'URL nécessitent une adresse complète commençant par `http://` ou `https://`.

### Variables de modèle Webhook

Les variables suivantes peuvent être utilisées dans l'URL de la requête et dans le corps, écrites sous la forme du nom de la variable entre accolades (la forme avec préfixe `#` est également acceptée) :

| Variable | Contenu |
| --- | --- |
| `title` | Titre, sous la forme `[Gately] Titre de l'événement` |
| `message` | Description détaillée de l'événement |
| `text` | Corps complet, incluant la description, la source, le niveau et l'heure |
| `type` | Type d'événement, par exemple `ddns.changed` |
| `source` | Source de l'événement |
| `level` | Niveau : `info`, `warn` ou `error` |
| `time` | Heure au format RFC 3339 |

Les champs supplémentaires portés par un événement (comme `ip` et `container`) peuvent aussi servir de variables. Si un nom entre en collision avec le tableau ci-dessus, ce dernier est prioritaire. Les valeurs de variables dans l'URL sont échappées pour les URL.

### E-mail

| Chiffrement | Valeur | Port par défaut |
| --- | --- | --- |
| STARTTLS | `starttls` | 587 |
| SSL/TLS | `tls` | 465 |
| Aucun | `none` | 25 |

Un port à 0 utilise le port par défaut du chiffrement choisi. Vous pouvez saisir plusieurs destinataires séparés par des virgules. Les e-mails sont envoyés en texte brut UTF-8. L'authentification PLAIN est utilisée lorsque le serveur prend en charge AUTH et qu'un nom d'utilisateur est défini ; lorsque l'expéditeur est vide, le nom d'utilisateur est utilisé.

### Signatures et codes de réponse

- DingTalk : une fois un secret de signature défini, les requêtes portent automatiquement `timestamp` et `sign`.
- Feishu : une fois un secret de signature défini, le corps de la requête porte automatiquement `timestamp` et `sign`.
- WeCom, DingTalk, Feishu et Bark vérifient le code de retour métier dans la réponse. Un HTTP 200 accompagné d'un échec métier est également enregistré comme un envoi en échec.

### Proxy {#proxy}

Lorsque le service de notification n'est pas joignable directement et ne l'est que par un proxy (par exemple Telegram, Discord ou Slack), les notifications peuvent passer par un proxy :

- **Proxy global** : dans « Paramètres de notification », en en-tête de la page, on définit le proxy utilisé par défaut par tous les canaux : « Sans proxy », « Utiliser le proxy global de mise à jour » (réutilise le proxy de vérification des mises à jour des [paramètres système](/fr/docs/guide/configuration) ; la modification s'applique immédiatement et, si ce proxy est vide, cela revient à ne pas utiliser de proxy) ou « Proxy personnalisé ».
- **Proxy par canal** : le champ « Proxy » de la fenêtre de modification de chaque canal vaut par défaut « Suivre les paramètres de notification » ; on peut aussi choisir « Direct (sans proxy) », « Utiliser le proxy global de mise à jour » ou un proxy personnalisé, qui remplace le réglage global. Les services auto-hébergés sur le réseau local, comme ntfy ou Gotify, peuvent rester en direct.
- Les adresses de proxy acceptent `http://`, `https://`, `socks5://` et `socks5h://`, avec nom d'utilisateur et mot de passe facultatifs, par exemple `http://user:pass@127.0.0.1:7890` ; les cartes de canal masquent le mot de passe lorsqu'elles affichent le proxy.
- Avec « Sans proxy », les canaux HTTP respectent les variables d'environnement comme `HTTPS_PROXY` et `NO_PROXY` et l'e-mail se connecte directement, comme auparavant ; « Direct » ignore en plus les variables d'environnement.
- L'e-mail accepte aussi un proxy : avec un proxy `http(s)://`, un tunnel est établi par la méthode CONNECT jusqu'au port du serveur de messagerie (le proxy doit autoriser la connexion à ce port ; les proxys courants comme Clash l'autorisent par défaut), et avec un proxy `socks5://`, c'est le proxy qui résout le nom du serveur. STARTTLS / SSL et l'authentification se déroulent ensuite dans le tunnel, si bien que le proxy ne voit pas le contenu des e-mails.
- Si la connexion au proxy échoue, le journal des envois et le message du test affichent « Échec de la connexion via le proxy … », le mot de passe du proxy étant remplacé par `xxxxx`.

## Règles

Cliquez sur « Ajouter une règle » :

| Champ | Description |
| --- | --- |
| Nom | Obligatoire |
| Événements | Au moins un. Choisissez des types d'événements précis, `*` pour tous les événements, ou saisissez un joker comme `ddns.*`. Les jokers utilisent la syntaxe `path.Match` de Go |
| Niveau minimal | `info`, `warn` ou `error` ; vide équivaut à info. Les événements sous ce niveau ne sont pas envoyés |
| Canaux | Au moins un |
| Silence (secondes) | Au sein d'une même règle, les événements du même type provenant de la même source ne sont envoyés qu'une fois pendant cette période, les répétitions sont ignorées. 0 désactive la déduplication |
| Activée | Interrupteur |

::: tip Astuce
Les blocages WAF et les changements d'état des amonts peuvent être fréquents. Pour les règles qui les couvrent, définissez un temps de silence de 300 secondes ou plus.
:::

## Modèles de règles

Lors de la création d'une règle, cliquez sur un modèle pour préremplir les événements, le niveau et le temps de silence, puis choisissez les canaux et enregistrez :

| Modèle | Événements |
| --- | --- |
| Pannes critiques | Échec de mise à jour DDNS, échec de renouvellement de certificat, amont indisponible, arrêt anormal d'un sous-processus, échec de sauvegarde, arrêt de conteneur |
| Expiration des certificats | Certificat bientôt expiré, échec de renouvellement (silence de 24 heures) |
| Changements d'adresse | Mise à jour d'enregistrement DDNS, changement d'adresse STUN |
| Santé des services | Amont indisponible puis rétabli, arrêt de conteneur, arrêt de sous-processus |
| Sécurité et connexions | Échec de connexion, IP bannie, connexion réussie, accès à des contenus sensibles |
| Alertes de performances | Déclenchement et rétablissement des alertes de performances (sans silence) |
| Modifications de configuration | Chaque modification de configuration faite dans l'interface d'administration |
| Mises à jour | Mises à jour de sécurité et nouvelles versions de Gately, nouvelles versions des programmes gérés et des images de conteneurs (sans silence) |

## Types d'événements

| Type d'événement | Nom | Niveau | Déclencheur |
| --- | --- | --- | --- |
| `ddns.changed` | DNS dynamique : enregistrement mis à jour | Info | Une tâche DDNS a mis à jour son enregistrement avec succès |
| `ddns.failed` | DNS dynamique : échec de la mise à jour | Erreur | Une tâche DDNS n'a pas pu mettre à jour son enregistrement |
| `stun.changed` | STUN : adresse de traversée modifiée | Info | L'adresse de traversée d'une règle STUN a changé |
| `ssl.expiring` | Certificat : expire bientôt | Avertissement | Moins de 7 jours de validité restants ; chaque certificat est rappelé au plus une fois toutes les 24 heures |
| `ssl.renew_failed` | Certificat : échec du renouvellement | Erreur | L'émission ou le renouvellement du certificat a échoué |
| `waf.blocked` | WAF : requête bloquée | Avertissement | Le WAF a bloqué une requête, avec les champs `ip`, `host` et `path` |
| `upstream.down` | Proxy inverse : amont hors service | Erreur | Un backend d'un service Web a été jugé indisponible |
| `upstream.up` | Proxy inverse : amont rétabli | Info | Un backend indisponible est de nouveau disponible |
| `docker.exited` | Docker : conteneur arrêté | Avertissement ou Erreur | Un conteneur s'est terminé avec un code de sortie non nul (avertissement), ou a été tué faute de mémoire (erreur) |
| `process.exited` | Processus enfant terminé anormalement | Erreur | Un processus enfant géré par Gately s'est terminé anormalement et sera redémarré automatiquement ; la source est le nom de journal de ce processus |
| `backup.done` | Sauvegarde : terminée | Info | Une sauvegarde planifiée ou manuelle a réussi pour toutes les destinations ; voir [Sauvegarde de la configuration](/fr/docs/modules/backup) |
| `backup.failed` | Sauvegarde : échec | Erreur | L'empaquetage de la sauvegarde a échoué, ou l'écriture vers une destination a échoué |
| `auth.failed` | Authentification : échecs répétés | Avertissement | Une connexion à l'interface d'administration a échoué, avec le champ `ip` |
| `auth.banned` | Authentification : IP bannie | Avertissement | Des échecs d'authentification répétés sur un service Web ont déclenché un bannissement automatique |
| `login.success` | Interface d'administration : connexion réussie | Info | Une connexion à l'interface d'administration a réussi, avec les champs `ip` et `user` |
| `audit.change` | Interface d'administration : modification de configuration | Info | Toute opération de modification dans l'interface d'administration, avec les champs `actor`, `ip`, `method` et `path` ; voir [Audit et historique de configuration](/fr/docs/guide/audit) |
| `audit.access` | Interface d'administration : accès à des contenus sensibles | Avertissement | Téléchargement d'une sauvegarde, d'un paquet de diagnostic ou d'une clé privée de certificat, ouverture d'un terminal, etc. ; mêmes champs que ci-dessus |
| `perf.alert` | Performances : alerte déclenchée | Avertissement ou erreur | La condition d'une règle d'alerte de performances s'est maintenue pendant sa durée ; le niveau vient de la règle, avec les champs `rule`, `metric`, `target`, `value` et `condition` ; voir [Performances](/fr/docs/modules/perf) |
| `perf.recovered` | Performances : alerte rétablie | Information | La condition d'une alerte de performances déclenchée a cessé ; mêmes champs |
| `system.update` | Gately : nouvelle version disponible | Info | Une vérification automatique ou manuelle a trouvé une nouvelle version ; champs `current`, `latest`, `url` ; chaque version n'est notifiée qu'une fois, voir [Vérification automatique des mises à jour](/fr/docs/guide/install#update-check) |
| `system.security` | Gately : mise à jour de sécurité | Erreur | La version actuelle est concernée par un avis de sécurité publié, ou une version plus récente contient des correctifs de sécurité ; champs `current`, `latest`, `url`, `advisories`, `securityReleases` ; nouvelle notification lorsqu'un nouveau problème apparaît |
| `program.update` | Programmes gérés : nouvelle version disponible | Info | La vérification automatique a trouvé une nouvelle version de frp, cloudflared, Tailscale ou File Browser installés ; la source est le nom du programme ; champs `program`, `current`, `latest` |
| `docker.update` | Docker : image plus récente disponible | Info | La vérification planifiée de Docker a trouvé des images plus récentes ; une notification par vérification, une seule fois par nouvelle image ; champ `count` |
| `notify.test` | Notification de test | Info | Produite par le bouton « Tester » du canal ; elle ne participe pas à la correspondance des règles |

## Envoyer un test

Le bouton « Tester » sur la carte d'un canal envoie directement à ce canal un message intitulé « Notification de test ». Il contourne les règles et fonctionne même si le canal n'est pas activé. En cas de succès, « Notification de test envoyée » s'affiche ; en cas d'échec, l'erreur précise s'affiche. Les résultats des tests apparaissent aussi dans « Envois récents », avec « Test » dans la colonne de la règle.

L'API correspondante est `POST /api/notify/channels/{key}/test`, qui peut être appelée avec le scope `notify:write` d'un [jeton d'API](/fr/docs/reference/api).
