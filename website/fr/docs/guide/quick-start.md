# Démarrage rapide

Déployez, connectez-vous et créez votre première règle en cinq minutes.

## 1. Démarrer

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  ghcr.io/wangwanjie/gately:latest
```

Pour gérer Docker depuis Gately, ajoutez `-v /var/run/docker.sock:/var/run/docker.sock` à la commande. `--cap-add NET_ADMIN` permet à la prévention d'intrusion d'écrire les bannissements dans le pare-feu de l'hôte. La redirection de ports, STUN et les services Web écoutent sur les ports de l'hôte : le réseau `--network host` est donc recommandé.

Si GHCR est lent chez vous, utilisez `vanjayak/gately:latest` sur Docker Hub à la place : c'est la même image.

::: tip Utilisateurs de fnOS
Installez plutôt l'application native : elle s'ouvre depuis l'icône du bureau via la passerelle unifiée fnOS et peut connecter les administrateurs fnOS avec leur session fnOS. Consultez [Application native fnOS](/fr/docs/guide/install#fnos).
:::

## 2. Se connecter

Ouvrez `http://<ip-de-l-appareil>:16680` dans un navigateur. Le nom d'utilisateur est `admin` et le mot de passe par défaut est `gately.cn`. Ce mot de passe est public : après la connexion, une boîte de dialogue demande de le remplacer par un nouveau mot de passe d'au moins 10 caractères, difficile à deviner, avant de pouvoir continuer.

Sur une installation neuve, après le changement du mot de passe, Gately demande d'abord « À quoi servira Gately ? » : choisissez une ou plusieurs options parmi « Publier des services internes sur un domaine », « Redirection de ports / traversée de NAT », « DNS dynamique », « Réseau entre sites », « Partage de fichiers », « Point d'entrée VPS » ou « Toutes les fonctions ». Seuls les modules des cas choisis restent actifs, les autres sont désactivés (leurs réglages sont conservés) et Gately redémarre une fois ; vous pouvez les activer à tout moment dans « Paramètres → Modules » ou avec « Ajouter d'autres fonctions » en bas de la barre latérale. « Ignorer » garde tout actif. Les instances mises à niveau ne voient pas cette étape. La liste « Premiers pas » en haut de la vue d'ensemble indique les prochaines étapes des cas choisis et les coche d'après la configuration réelle ; elle peut être réduite ou fermée.

## 3. Appliquer un durcissement de base

Dès la connexion, ouvrez « Paramètres » et :

1. Activez l'authentification à deux facteurs (TOTP), ou enregistrez une passkey ;
2. Définissez un chemin d'accès secret, un accès limité au réseau local ou le HTTPS de l'interface d'administration selon vos besoins ;
3. Consultez le score de sécurité en haut de « Paramètres → Compte et sécurité » et traitez les points signalés.

Consultez le [guide de durcissement](/fr/docs/guide/hardening) pour d'autres recommandations.

## 4. Ajouter votre première règle

« Site rapide » sur la page Service Web publie un site en trois étapes : saisissez le domaine, puis l'adresse du backend (avec « Tester », ou préremplie à partir des modèles Home Assistant, Jellyfin, Emby, Vaultwarden, Nextcloud, Immich, Proxmox VE, Synology DSM, fnOS), puis choisissez HTTP ou HTTPS. Une écoute est réutilisée ou créée automatiquement ; voir [Service Web · Site rapide](/fr/docs/modules/web#quick-site).

Choisissez le module qui correspond à votre besoin :

| Besoin | Module |
| --- | --- |
| Votre IP publique domestique change sans cesse et vous voulez un nom de domaine stable | [DNS dynamique](/fr/docs/modules/ddns) |
| Votre FAI ne fournit aucun port public fixe et vous voulez joindre vos services depuis l'extérieur | [Traversée STUN](/fr/docs/modules/stun) |
| Rediriger un port vers un autre appareil de votre réseau local | [Redirection de ports](/fr/docs/modules/portforward) |
| Atteindre plusieurs services du réseau local via un seul domaine en HTTPS | [Certificats SSL](/fr/docs/modules/ssl) + [Services Web](/fr/docs/modules/web) |
| Relier des réseaux situés sur des sites différents | [Tailscale / frp](/fr/docs/modules/tunnels) |
