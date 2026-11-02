---
title: "Télécharger"
description: "Téléchargez les derniers binaires et paquets fnOS et OpenWrt de Gately, ou déployez-le avec l'image Docker."
---

# Télécharger Gately

## Image Docker (recommandée)

L'image est publiée pour amd64, arm64 et arm/v7 :

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

La même image (mêmes tags) est aussi disponible sur Docker Hub :

```bash
docker pull vanjayak/gately:latest
```

Consultez [Installation](/fr/docs/guide/install) pour l'ensemble des options de déploiement.

## Binaires

Téléchargez l'archive de votre plateforme, extrayez-la et lancez `./gately -c /etc/gately`. Plateformes prises en charge : Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows et FreeBSD.

<DownloadList />

::: tip Téléchargement lent ?
GitHub peut être lent sur certains réseaux. Les utilisateurs de NAS et de routeurs logiciels peuvent d'abord passer par un proxy, ou utiliser l'image Docker.
:::

## Application native fnOS {#fnos}

Les utilisateurs de fnOS peuvent installer directement l'application native : téléchargez le `.fpk` dans le groupe fnOS de la liste ci-dessus (`x86` pour Intel / AMD, `arm` pour les appareils ARM), puis sélectionnez-le dans « Centre d'applications → Installation manuelle » de fnOS.

- Intégrée à la passerelle unifiée fnOS : ouvrez-la depuis l'icône du bureau, y compris via votre domaine fnOS distant, sans port supplémentaire à ouvrir ;
- Connexion des administrateurs fnOS avec leur session fnOS : une fois l'option activée, les administrateurs déjà connectés à fnOS se passent du mot de passe Gately et du code à deux facteurs.

Voir [Installation · Application native fnOS](/fr/docs/guide/install#fnos).

## Paquets OpenWrt {#openwrt}

Les utilisateurs d'OpenWrt peuvent télécharger un paquet dans le groupe OpenWrt de la liste ci-dessus : le `.ipk` pour OpenWrt 24.10 et antérieur (y compris iStoreOS), à installer avec `opkg install`, ou le `.apk` pour OpenWrt 25.12 et ultérieur, à installer avec `apk add --allow-untrusted`. Choisissez l'architecture selon `DISTRIB_ARCH` dans `/etc/openwrt_release`. Le service démarre dès l'installation et conserve sa configuration dans `/etc/gately`.

`luci-app-gately_<version>_openwrt_all`, dans le même groupe, est l'application LuCI. Elle ne dépend pas de l'architecture ; installée après Gately, elle permet de gérer le service depuis **Services → Gately** dans LuCI.

Voir [Installation · OpenWrt](/fr/docs/guide/install#openwrt).

## Compiler depuis les sources

Nécessite Go 1.26+, Node.js 24+ et pnpm :

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # compile le frontend
make build    # produit bin/gately
```

## Vérification

Chaque version est accompagnée d'un fichier de sommes de contrôle. Vérifiez le SHA-256 après le téléchargement et avant l'exécution.

Depuis la v0.5.0, le fichier de sommes et les images Docker portent une signature cosign sans clé, les archives sont accompagnées d’une attestation de provenance et un SBOM est publié. Voir [Vérifier les versions](/fr/docs/guide/verify) (en anglais).
