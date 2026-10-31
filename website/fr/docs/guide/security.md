# Modèle de sécurité et engagement de confidentialité

Gately fonctionne à la frontière du réseau et manipule des clés de fournisseurs DNS, des clés privées de certificats et d'autres identifiants : la sécurité et la transparence sont donc ses principaux objectifs de conception.

## Engagement de confidentialité

- **Zéro connexion sortante par défaut** : aucune connexion aux serveurs du projet, aucune remontée d'informations sur l'appareil, aucune poussée de configuration à distance.
- **Adresses externes visibles et modifiables** : la recherche de l'IP publique, les serveurs STUN et les adresses similaires sont affichés dans l'interface et peuvent être modifiés ou désactivés.
- **Connexion via des tiers sous votre contrôle** : les connexions GitHub, Google, QQ et OIDC utilisent des applications OAuth que vous enregistrez vous-même, sans relais intermédiaire.
- **Mises à niveau sous votre contrôle** : le programme ne se télécharge ni ne se remplace jamais en arrière-plan. La mise à niveau en ligne des installations binaires Linux ne s’exécute qu’après le clic d’un administrateur, vérifie le SHA-256 et conserve la version précédente ; vous décidez quand mettre à niveau.

## Protection des données

- La configuration est du JSON en clair avec les droits 0600, ce qui permet de l'auditer et de revenir en arrière.
- Les champs sensibles sont masqués dans les réponses de l'API. Le bouton en forme d'œil de l'interface récupère sur demande le texte en clair enregistré et exige la permission d'écriture. Les mots de passe des utilisateurs web stockés sous forme de hash bcrypt, les secrets client OIDC et les clés privées ne sont jamais renvoyés ; les utilisateurs partagés avec vérification en deux étapes peuvent afficher à nouveau leur QR code. Les variables d'environnement, les commandes de démarrage et les lignes de commande des processus des conteneurs Docker sont traitées de la même façon : sans permission d'écriture Docker, elles s'affichent sous la forme `********`.
- Le jeton cloudflared est transmis par variable d'environnement et n'apparaît pas dans la liste des processus ; les jetons et clés frp et Tailscale sont stockés avec les droits 0600.

## Authentification et contrôle d'accès

- Hachage des mots de passe avec bcrypt et jetons de session signés HMAC ;
- Le mot de passe par défaut `gately.cn` est public : tant qu'il est utilisé, un avertissement permanent invite à le changer en haut de l'interface d'administration. En cas d'oubli du mot de passe ou de perte de l'application d'authentification, la réinitialisation se fait uniquement sur le serveur avec `gately reset-password` / `gately reset-totp` (accès en lecture et écriture au répertoire de configuration et confirmation par `yes` requis) ; elle invalide toutes les sessions et est consignée dans le journal d'audit. Consultez [Réinitialiser le mot de passe et l'authentification à deux facteurs](/fr/docs/guide/install#reset) ; tant que le mot de passe par défaut est utilisé, le compte, la vérification en deux étapes, les passkeys, les comptes supplémentaires, les jetons d'API, les paramètres système et les paramètres de sécurité du terminal Web sont consultables mais non modifiables (la restauration d'une sauvegarde et le retour des paramètres système ou du terminal Web à une version de l'historique sont aussi refusés), jusqu'à ce que le mot de passe par défaut soit changé ; le nouveau mot de passe ne peut pas être celui par défaut, et une fois changé il ne peut pas y revenir;
- Connexion par passkey (WebAuthn) : seule la clé publique est stockée, la vérification de l'utilisateur (empreinte, visage ou code PIN) est obligatoire et le compteur de signatures empêche le clonage ; nécessite un accès par nom de domaine en HTTPS (ou localhost) ;
- Authentification à deux facteurs (TOTP, vérifiée avec les vecteurs de test de la RFC 6238) ;
- Comptes en lecture seule : ils voient l'état et la configuration, mais ne peuvent ni modifier, ni télécharger de sauvegarde, ni lire de secrets, ni ouvrir de terminal ; la réinitialisation de leur mot de passe ou leur suppression invalide immédiatement leurs sessions ; lorsque les comptes en lecture seule et les jetons d'API en lecture seule lisent la configuration, les mots de passe des utilisateurs partagés des sites, les jetons des liens de partage, les valeurs des en-têtes de requête et de réponse, les secrets ainsi que les mots de passe et valeurs de requête dans les URL s'affichent sous la forme `********`;
- Limitation des tentatives de connexion ;
- Proxy inverse local : lorsque l'interface d'administration se trouve derrière nginx, haproxy ou un service Web Gately sur la même machine, seuls les en-têtes `X-Real-IP` / `X-Forwarded-For` provenant d'adresses locales sont pris en compte. L'accès limité au réseau local, la limitation des tentatives de connexion et le journal d'audit obtiennent ainsi la source réelle. Ce comportement peut être désactivé, ou complété par des proxys de confiance supplémentaires, dans « Paramètres → Contrôle d'accès » ; lorsque les deux en-têtes divergent, l'entrée de `X-Forwarded-For` ajoutée par le proxy l'emporte, et si l'une est une adresse interne et l'autre publique, l'adresse publique est retenue : une adresse interne falsifiée ne permet donc pas de contourner l'accès réservé au réseau local ; configurez le proxy pour qu'il écrive les deux en-têtes (nginx : `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`);
- Chemin d'accès secret, accès limité au réseau local et HTTPS pour l'interface d'administration ;
- Passerelle unifiée fnOS : lorsque le paquet fnOS est accédé via la passerelle unifiée fnOS, fnOS contrôle l'accès et seuls les administrateurs fnOS peuvent l'ouvrir. « Connexion via fnOS » est désactivé par défaut ; une fois activé, les administrateurs fnOS déjà connectés entrent en tant qu'administrateur Gately (sans vérification en deux étapes), tandis que les utilisateurs fnOS ordinaires doivent toujours se connecter. Consultez [Application native fnOS](/fr/docs/guide/install#fnos) ;
- Activation des modules : les modules désactivés dans « Paramètres → Modules » ne démarrent aucun service et n'exposent aucune API d'administration, ce qui réduit la surface d'attaque ;
- API multi-jetons : chaque jeton n'a un accès en lecture/écriture qu'aux modules choisis à sa création, peut avoir une date d'expiration et enregistre l'IP source de sa dernière utilisation. Les points d'accès du compte, de l'authentification à deux facteurs, de la gestion des jetons, ainsi que de la sauvegarde, de la restauration, du redémarrage et des paramètres système n'acceptent que les sessions de connexion. Consultez [Jetons d'API et OpenAPI](/fr/docs/reference/api) ;
- Contrôle d'accès aux sites : bannissement automatique après échecs d'authentification, filtrage GeoIP par pays et règles de chemins. Consultez [Services Web](/fr/docs/modules/web) ;
- Les sauvegardes de configuration peuvent être chiffrées (scrypt + AES-GCM). Consultez [Sauvegarde de la configuration](/fr/docs/modules/backup).

## Audit et traçabilité

Le journal d'audit enregistre les connexions, les modifications de configuration et les accès sensibles. L'historique de configuration conserve la version de chaque fichier de configuration avant écrasement et permet de la restaurer en un clic. Consultez [Audit et historique de configuration](/fr/docs/guide/audit). Les artefacts publiés sont accompagnés d'une signature cosign, d'une attestation de provenance de build et d'un SBOM ; la méthode de vérification est décrite dans [Vérifier les versions](/fr/docs/guide/verify).

## Valeurs par défaut des fonctions à risque

Le terminal de shell local est désactivé par défaut. Le terminal SSH applique une politique de confiance à la première utilisation (TOFU) pour enregistrer les empreintes des clés d'hôte.

## Signaler un problème de sécurité

Signalez les vulnérabilités en privé via la page Security du dépôt, et évitez d'en divulguer les détails dans les tickets publics : <https://github.com/wangwanjie/gately/security>

Pour d'autres recommandations pratiques, consultez le [guide de durcissement](/fr/docs/guide/hardening).
