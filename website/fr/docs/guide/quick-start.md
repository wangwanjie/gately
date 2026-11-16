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

## 3. Appliquer un durcissement de base

Dès la connexion, ouvrez « Paramètres » et :

1. Activez l'authentification à deux facteurs (TOTP), ou enregistrez une passkey ;
2. Définissez un chemin d'accès secret, un accès limité au réseau local ou le HTTPS de l'interface d'administration selon vos besoins ;
3. Consultez le score de sécurité en haut de « Paramètres → Compte et sécurité » et traitez les points signalés.

Consultez le [guide de durcissement](/fr/docs/guide/hardening) pour d'autres recommandations.

## 4. Ajouter votre première règle

Choisissez le module qui correspond à votre besoin :

| Besoin | Module |
| --- | --- |
| Votre IP publique domestique change sans cesse et vous voulez un nom de domaine stable | [DNS dynamique](/fr/docs/modules/ddns) |
| Votre FAI ne fournit aucun port public fixe et vous voulez joindre vos services depuis l'extérieur | [Traversée STUN](/fr/docs/modules/stun) |
| Rediriger un port vers un autre appareil de votre réseau local | [Redirection de ports](/fr/docs/modules/portforward) |
| Atteindre plusieurs services du réseau local via un seul domaine en HTTPS | [Certificats SSL](/fr/docs/modules/ssl) + [Services Web](/fr/docs/modules/web) |
| Relier des réseaux situés sur des sites différents | [Tailscale / frp](/fr/docs/modules/tunnels) |
