# Traversée STUN

Lorsque votre FAI ne fournit pas de port public fixe, la traversée STUN utilise le percement de NAT pour obtenir une adresse et un port publics mappés, afin que les services de votre réseau local soient accessibles depuis internet.

## Fonctions

- Traversée TCP / UDP avec maintien de session ;
- UPnP / NAT-PMP crée automatiquement les mappages de ports sur le routeur ;
- La redirection intégrée envoie directement le port public obtenu par la traversée vers une cible du réseau local ;
- Des Webhooks et des scripts se déclenchent quand l'adresse ou le port public change, ce qui permet de mettre à jour le DNS, de notifier des clients, etc.

## Utilisation avec d'autres modules

Le port public obtenu via STUN change généralement. Associez-le à :

- [DNS dynamique](/fr/docs/modules/ddns) ou à un Webhook / script pour synchroniser la nouvelle adresse avec vos enregistrements DNS ;
- [Services Web](/fr/docs/modules/web) pour fournir un point d'entrée HTTPS.
- Les sites de redirection des Services Web peuvent référencer l'adresse publique actuelle de la règle avec des variables comme `{STUN_NomRègle_PORT}` et suivent automatiquement le nouveau port quand il change ; voir [Variables de redirection](/fr/docs/modules/web#redirect-vars).

## Limites et NAT à plusieurs niveaux

- **NAT de niveau opérateur (CGNAT)** : quand le WAN du routeur reçoit une adresse interne comme `100.64.0.0/10`, la traversée dépend du type de NAT de l'opérateur. Un NAT à cône complet (toute adresse extérieure peut joindre le port mappé) fonctionne ; un NAT à port restreint ou symétrique ne fonctionne pas, utilisez alors [Tailscale, frp ou Cloudflare Tunnel](/fr/docs/modules/tunnels).
- **UPnP derrière plusieurs NAT** : avec UPnP / NAT-PMP, Gately mappe le port du tunnel vers cet hôte sur le routeur. Si l'adresse externe du routeur diffère de l'adresse publique obtenue par STUN, un autre NAT se trouve à l'extérieur et le port public lui appartient : Gately ne garde que le mappage du port du tunnel et n'ajoute plus de mappage pour le port public.
- **Fixer le port local et l'autoriser dans le pare-feu** : un port local vide est choisi au hasard à chaque démarrage. Si l'hôte utilise un pare-feu (par exemple le pare-feu système de fnOS), fixez le port local de la règle et autorisez-le dans le pare-feu (vous pouvez réserver une plage de ports aux règles STUN) ; sinon le pare-feu rejette les connexions entrantes et l'adresse de traversée reste injoignable.
- **TCP et NAT à cône du routeur** : sur certains routeurs, « NAT1 / cône complet » ne s'applique qu'à l'UDP et le port source TCP est réécrit en sortie. Les règles TCP nécessitent UPnP, ou un mappage du port du tunnel vers cet hôte sur le routeur.

## Conseil de déploiement

STUN doit écouter directement sur les ports de l'hôte. Pour un déploiement Docker, `--network host` est recommandé.
