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

### Build personalizado {#custom-build}

Módulos opcionais e idiomas da interface podem ser removidos na compilação (veja [Visão geral dos módulos · Builds personalizados](/pt-BR/docs/modules/#custom-build)). O jeito mais simples é marcar as opções em [build.gately.cn](https://build.gately.cn) e gerar online. Para compilar por conta própria:

- O Go usa build tags para remover módulos e idiomas: cada módulo opcional corresponde a `no_<módulo>` (como `no_waf`, `no_smb`), e o nível de idiomas é `lang_zh` (apenas chinês simplificado) ou `lang_cjken` (chinês simplificado, chinês tradicional e inglês); sem tags, sai a edição completa;
- O frontend usa a variável de ambiente `GATELY_LOCALES` para empacotar apenas os idiomas escolhidos (como `zh-CN,zh-TW,en`); vazia, inclui todos;
- O `modules.json` na raiz do repositório lista todos os módulos opcionais, as dependências e a estimativa de tamanho; `go run ./tools/custombuild resolve` converte uma seleção em tags, idiomas e argumentos de build do Docker, e o subcomando `build` gera diretamente o tar.gz ou o pacote fnOS.

```bash
# Mantém só DDNS e frp, com a interface apenas em chinês simplificado
go run ./tools/custombuild resolve -modules ddns,frp -lang zh
go run ./tools/custombuild build -version v0.6.4 -modules ddns,frp -lang zh -targets linux-arm64 -out dist
```

As imagens Docker usam o mesmo Dockerfile: os argumentos de build `GATELY_TAGS` e `GATELY_LOCALES` correspondem às tags e idiomas acima, `WITH_DOCKER_CLI=0` deixa de instalar o docker-cli e o compose (quando o módulo Docker não está incluído) e `WITH_NFTABLES=0` deixa de instalar o nftables (quando Prevenção de intrusão e STUN não estão incluídos); `resolve -targets docker` mostra os argumentos correspondentes.

## Verificação

Cada versão acompanha um arquivo de checksums. Confira o SHA-256 depois de baixar e antes de executar.

Desde a v0.5.0, o arquivo de checksums e as imagens Docker têm assinatura sem chave do cosign, os arquivos trazem atestado de procedência e um SBOM é publicado. Veja [Verificar versões](/pt-BR/docs/guide/verify) (em inglês).
