---
title: "Descargas"
description: "Descarga los últimos binarios y paquetes para fnOS y OpenWrt de Gately, o despliégalo con la imagen de Docker."
---

# Descargar Gately

## Imagen de Docker (recomendada)

La imagen se publica para amd64, arm64 y arm/v7:

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

La misma imagen (con las mismas etiquetas) también está en Docker Hub:

```bash
docker pull vanjayak/gately:latest
```

Consulta [Instalación](/es/docs/guide/install) para ver todas las opciones de despliegue.

## Binarios

Descarga el archivo para tu plataforma, descomprímelo y ejecuta `./gately -c /etc/gately`. Plataformas compatibles: Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows y FreeBSD.

<DownloadList />

::: tip ¿Descargas lentas?
GitHub puede ser lento en algunas redes. Los usuarios de NAS y routers software pueden descargar primero a través de un proxy o usar la imagen de Docker.
:::

## Aplicación nativa para fnOS {#fnos}

Los usuarios de fnOS pueden instalar directamente la aplicación nativa: descarga el `.fpk` del grupo fnOS de la lista anterior (`x86` para Intel / AMD, `arm` para dispositivos ARM) y selecciónalo en fnOS en «Centro de aplicaciones → Instalación manual».

- Funciona con la pasarela unificada de fnOS: se abre desde el icono del escritorio, también a través de tu dominio remoto de fnOS, sin abrir puertos adicionales;
- Inicia la sesión de los administradores de fnOS con su sesión de fnOS: si lo activas, los administradores que ya tengan la sesión iniciada en fnOS no necesitan la contraseña de Gately ni el código de verificación en dos pasos.

Consulta [Instalación · Aplicación nativa para fnOS](/es/docs/guide/install#fnos).

## Paquetes para OpenWrt {#openwrt}

Los usuarios de OpenWrt pueden descargar un paquete del grupo OpenWrt de la lista anterior: elige el `.ipk` para OpenWrt 24.10 y anteriores (incluido iStoreOS) e instálalo con `opkg install`, o el `.apk` para OpenWrt 25.12 y posteriores e instálalo con `apk add --allow-untrusted`. Elige la arquitectura según `DISTRIB_ARCH` en `/etc/openwrt_release`. El servicio se inicia justo después de instalar y guarda su configuración en `/etc/gately`.

`luci-app-gately_<versión>_openwrt_all`, en el mismo grupo, es la aplicación LuCI. No depende de la arquitectura; instálala después de Gately para gestionar el servicio desde **Servicios → Gately** en LuCI.

Consulta [Instalación · OpenWrt](/es/docs/guide/install#openwrt).

## Compilar desde el código fuente

Requiere Go 1.26+, Node.js 24+ y pnpm:

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # compila el frontend
make build    # genera bin/gately
```

## Verificación

Cada versión incluye un archivo de sumas de verificación. Comprueba el SHA-256 tras la descarga y antes de ejecutar.

Desde v0.5.0, el archivo de sumas y las imágenes Docker llevan firmas sin clave de cosign, los archivos incluyen atestación de procedencia y se publica un SBOM. Consulta [Verificar versiones](/es/docs/guide/verify) (en inglés).
