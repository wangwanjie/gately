---
title: "Download"
description: "Baixe os binários e os pacotes para fnOS e OpenWrt mais recentes do Gately ou implante com a imagem Docker."
---

# Baixar o Gately

## Imagem Docker (recomendada)

A imagem é publicada para amd64, arm64 e arm/v7:

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

A mesma imagem (com as mesmas tags) também está no Docker Hub:

```bash
docker pull vanjayak/gately:latest
```

Veja todas as opções de implantação em [Instalação](/pt-BR/docs/guide/install).

## Binários

Baixe o pacote da sua plataforma, extraia e execute `./gately -c /etc/gately`. Plataformas compatíveis: Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows e FreeBSD.

<DownloadList />

::: tip Download lento?
O GitHub pode ficar lento em algumas redes. Usuários de NAS e soft routers podem baixar antes por meio de um proxy ou usar a imagem Docker.
:::

## Aplicativo nativo para fnOS {#fnos}

Usuários do fnOS podem instalar diretamente o aplicativo nativo: baixe o `.fpk` no grupo fnOS da lista acima (`x86` para Intel / AMD, `arm` para dispositivos ARM) e selecione-o em "Central de aplicativos → Instalação manual" no fnOS.

- Integrado ao gateway unificado do fnOS: abre pelo ícone na área de trabalho, também pelo seu domínio remoto do fnOS, sem nenhuma porta extra para abrir;
- Administradores do fnOS entram com a sessão do fnOS: depois de ativado, os administradores que já estão conectados ao fnOS dispensam a senha do Gately e o código de verificação em dois fatores.

Veja [Instalação · Aplicativo nativo para fnOS](/pt-BR/docs/guide/install#fnos).

## Pacotes para OpenWrt {#openwrt}

Usuários do OpenWrt podem baixar um pacote no grupo OpenWrt da lista acima: escolha o `.ipk` para o OpenWrt 24.10 e anteriores (incluindo o iStoreOS) e instale com `opkg install`, ou o `.apk` para o OpenWrt 25.12 e posteriores e instale com `apk add --allow-untrusted`. Escolha a arquitetura pelo `DISTRIB_ARCH` em `/etc/openwrt_release`. O serviço inicia logo após a instalação e guarda a configuração em `/etc/gately`.

`luci-app-gately_<versão>_openwrt_all`, no mesmo grupo, é o aplicativo LuCI. Ele não depende da arquitetura; instale-o depois do Gately para gerenciar o serviço em **Serviços → Gately** no LuCI.

Veja [Instalação · OpenWrt](/pt-BR/docs/guide/install#openwrt).

## Compilar a partir do código-fonte

Requer Go 1.26+, Node.js 24+ e pnpm:

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # compila o frontend
make build    # gera bin/gately
```

## Verificação

Cada versão acompanha um arquivo de checksums. Confira o SHA-256 depois de baixar e antes de executar.

Desde a v0.5.0, o arquivo de checksums e as imagens Docker têm assinatura sem chave do cosign, os arquivos trazem atestado de procedência e um SBOM é publicado. Veja [Verificar versões](/pt-BR/docs/guide/verify) (em inglês).
