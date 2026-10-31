<p align="center">
  <a href="https://gately.cn/es/"><img src="docs/images/banner.svg" alt="Gately — La pasarela de red todo en uno: Código abierto y lista para usar" width="100%"></a>
</p>

<p align="center">
  <a href="https://github.com/wangwanjie/gately/releases/latest"><img src="https://img.shields.io/github/v/release/wangwanjie/gately?style=flat-square&color=0e7c66&label=release" alt="Release"></a>
  <a href="https://github.com/wangwanjie/gately/pkgs/container/gately"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fwangwanjie%2Fgately-0e7c66?style=flat-square&logo=docker&logoColor=white" alt="Docker"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wangwanjie/gately?style=flat-square&color=0e7c66" alt="License"></a>
  <a href="https://github.com/wangwanjie/gately/stargazers"><img src="https://img.shields.io/github/stars/wangwanjie/gately?style=flat-square&color=0e7c66" alt="Stars"></a>
  <a href="https://gately.cn/es/"><img src="https://img.shields.io/badge/Sitio%20web-gately.cn-0e7c66?style=flat-square&logo=googlechrome&logoColor=white" alt="gately.cn"></a>
  <a href="https://t.me/gately_chat"><img src="https://img.shields.io/badge/Telegram-gately__chat-26A5E4?style=flat-square&logo=telegram&logoColor=white" alt="Telegram"></a>
</p>

<p align="center">
  <a href="https://gately.cn/es/"><b>Sitio web</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/es/docs/guide/quick-start"><b>Inicio rápido</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/es/docs/guide/install"><b>Guía de instalación</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/es/docs/guide/introduction"><b>Documentación</b></a> &nbsp;·&nbsp; <a href="https://github.com/wangwanjie/gately/releases/latest"><b>Descargas</b></a> &nbsp;·&nbsp; <a href="https://gately.cn/es/news/"><b>Registro de cambios</b></a>
</p>

<p align="center">
  <a href="https://gately.cn/es/sponsor"><img src="https://img.shields.io/badge/Inv%C3%ADtame%20a%20un%20caf%C3%A9-Gately-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black" alt="Invítame a un café" height="34"></a>
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.zh-TW.md">繁體中文</a> | <a href="README.ja.md">日本語</a> | <a href="README.ko.md">한국어</a> | <b>Español</b> | <a href="README.fr.md">Français</a> | <a href="README.de.md">Deutsch</a> | <a href="README.ru.md">Русский</a> | <a href="README.pt-BR.md">Português (Brasil)</a>
</p>

DNS y traversal de NAT, proxy inverso y certificados HTTPS, WAF y prevención de intrusiones, inicio de sesión único y archivos compartidos: todo lo que tu red doméstica y tus servidores necesitan en la entrada, en un solo panel. Instálalo en un NAS, un router por software o un servidor en la nube y configúralo todo desde el navegador. Es de código abierto bajo Apache-2.0; tu configuración y tus datos se quedan en tu dispositivo, y no se recopila ni se envía nada.

> **Usuarios de fnOS:** Gately se distribuye como aplicación nativa de fnOS. Se integra en la **pasarela unificada de fnOS**, de modo que se abre desde el icono del escritorio de fnOS y funciona en la misma dirección que usas para fnOS, incluido tu dominio remoto, sin abrir ningún puerto adicional. Si lo activas, **los administradores de fnOS pueden iniciar sesión directamente con su sesión de fnOS**, sin contraseña de Gately ni código de verificación en dos pasos. Consulta [Aplicación nativa para fnOS](https://gately.cn/es/docs/guide/install#fnos).

## Inicio rápido

Tres pasos: arranca el contenedor, abre `http://<IP del dispositivo>:16680` e inicia sesión con `admin` / `gately.cn`. Las demás plataformas están más abajo, y la [guía de instalación](https://gately.cn/es/docs/guide/install) recoge todas las opciones.

### Docker

La imagen es compatible con amd64, arm64 y arm/v7:

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

Abre `http://<IP del dispositivo>:16680`, inicia sesión con el usuario predeterminado `admin` y la contraseña predeterminada `gately.cn`, y después cambia la contraseña y activa la verificación en dos pasos en «Ajustes». Encontrarás un ejemplo completo de Compose en [deploy/docker-compose.yml](deploy/docker-compose.yml).

Si olvidas la contraseña o pierdes el autenticador, ejecuta `gately reset-password` o `gately reset-totp` en el servidor (con Docker, `docker exec -it gately gately reset-password`); el cambio se aplica en cuanto lo confirmas. Consulta [Restablecer la contraseña y la verificación en dos pasos](https://gately.cn/es/docs/guide/install#reset).

El reenvío de puertos, STUN y el servicio web necesitan escuchar puertos del host, por lo que se recomienda `--network host`. Si no necesitas la gestión de Docker, puedes quitar el montaje de `docker.sock`. `--cap-add NET_ADMIN` permite que Prevención de intrusiones escriba los bloqueos en el cortafuegos nftables del host; sin él, los bloqueos siguen aplicándose a los servicios de Gately.

¿Prefieres otra forma de instalarlo? Elige una aquí abajo.

<details>
<summary><b>Binario</b></summary>

Descarga el paquete de tu plataforma desde [Releases](https://github.com/wangwanjie/gately/releases) (Linux amd64/arm64/arm/mips/mipsle/riscv64, macOS, Windows, FreeBSD), descomprímelo y ejecuta:

```bash
./gately -c /etc/gately
```

Parámetros: `-c` directorio de configuración (también se puede usar la variable de entorno `GATELY_CONFIG_DIR`), `-listen` sobrescribe la dirección de escucha de la interfaz de administración, `-apply` aplica en cada arranque los archivos `<nombre>.json` de un directorio al directorio de configuración (`GATELY_APPLY_DIR`, útil para GitOps y Kubernetes; los archivos de cuentas y tokens no se pueden sobrescribir), `-version` muestra la versión. En instalaciones desatendidas, define `GATELY_INITIAL_PASSWORD` para elegir la contraseña inicial del administrador en lugar de la contraseña predeterminada `gately.cn`.

Plantillas de servicio: [systemd](deploy/systemd/gately.service) y [OpenWrt procd](deploy/openwrt/gately.init). Para verificar los archivos e imágenes descargados (firmas cosign, procedencia de compilación, SBOM), consulta [Verificar los artefactos publicados](docs/es/VERIFY.md).

</details>

<details>
<summary><b>Aplicación nativa para fnOS</b></summary>

Cada versión (Release) incluye paquetes `.fpk` sin conexión para fnOS: `gately_<version>_fnos_x86.fpk` (Intel/AMD) y `gately_<version>_fnos_arm.fpk` (ARM). No están en el Centro de aplicaciones oficial, así que debes instalarlos manualmente: abre **Centro de aplicaciones → Instalación manual** (el botón de la esquina inferior izquierda), elige el archivo y completa el asistente.

- **Funciona con la pasarela unificada de fnOS**: haz clic en el icono de Gately en el escritorio de fnOS y la interfaz de administración se abre a través de la pasarela unificada de fnOS en `/app/gately/`, compartiendo el punto de entrada que ya usas para fnOS. Funcionan tanto la dirección de la red interna como tu dominio remoto de fnOS, sin necesidad de abrir ni redirigir el puerto 16680. El acceso a través de la pasarela lo controla fnOS (solo los administradores de fnOS pueden abrirla), por lo que la entrada de seguridad y el acceso solo desde la red interna se aplican únicamente al acceso directo por el puerto 16680.
- **Inicia la sesión de los administradores de fnOS con su sesión de fnOS**: activa **Ajustes → Control de acceso → Iniciar sesión con fnOS** y los administradores de fnOS que tengan la sesión iniciada en fnOS entrarán en Gately desde el icono del escritorio sin contraseña ni código de verificación en dos pasos; los usuarios normales de fnOS siguen iniciando sesión. Tras cerrar la sesión, la página de inicio de sesión muestra el botón «Iniciar sesión como usuario de fnOS &lt;nombre&gt;» y espera a que lo pulses para volver a entrar. Este ajuste está desactivado por defecto y solo aparece en el paquete de fnOS.
- En la red interna, la interfaz de administración también está disponible en `http://<dirección del NAS>:16680`. El usuario predeterminado es `admin` y la contraseña predeterminada, `gately.cn`.
- La aplicación se ejecuta como `root`, de modo que puede escuchar en 80/443, gestionar Docker mediante `docker.sock` y abrir un terminal web real. Instálala solo en un NAS que controles.
- La configuración se guarda en el directorio `var` de la aplicación y se conserva al desinstalar, salvo que elijas «Eliminar todo» en el asistente de desinstalación. Para actualizar, instala manualmente el `.fpk` más reciente.
- El paquete también añade el comando `gately` (`/usr/local/bin/gately`) para `gately ctl` y `gately mcp`.
- El puerto 16680 se comparte con la imagen de Docker. Detén el contenedor `gately` antes de iniciar la aplicación nativa.
- Verifica la descarga como se describe en [Verificar los artefactos publicados](docs/es/VERIFY.md); los archivos `.fpk` figuran en `checksums.txt`.

</details>

<details>
<summary><b>OpenWrt</b></summary>

Cada versión (Release) incluye paquetes para OpenWrt: `gately_<version>_openwrt_<arch>.ipk` para OpenWrt 24.10 y anteriores (incluido iStoreOS), que se instala con `opkg install`, y `.apk` para OpenWrt 25.12 y posteriores, que se instala con `apk add --allow-untrusted`. Hay paquetes para amd64 / arm64 / arm / mipsle / mips / riscv64; elige uno según `DISTRIB_ARCH` en `/etc/openwrt_release` (consulta la tabla de la [guía de instalación](https://gately.cn/es/docs/guide/install#openwrt)).

- El servicio se activa en el arranque y se inicia al instalar; la interfaz de administración está en `http://<dirección del router>:16680`, con el usuario predeterminado `admin` y la contraseña predeterminada `gately.cn`.
- La configuración se guarda en `/etc/gately` y se conserva al actualizar, al desinstalar y al actualizar el firmware (sysupgrade). Para actualizar, instala el paquete más reciente; la actualización en línea de la página Acerca de te remite al gestor de paquetes.
- El binario ocupa unos 32 MB, así que los paquetes son adecuados para routers software x86, equipos ARM64 y otros dispositivos con almacenamiento amplio; el paquete arm requiere ARMv7 con coma flotante por hardware.
- Una aplicación LuCI independiente de la arquitectura, `luci-app-gately_<versión>_openwrt_all.ipk` / `.apk`, añade **Servicios → Gately** a LuCI: estado del servicio, iniciar / detener, inicio automático, puerto de administración y un enlace directo a la interfaz.

</details>

<details>
<summary><b>Monitorización</b></summary>

Crea un token de API con solo el permiso `metrics:read` y apunta Prometheus a `/api/metrics`. Las métricas, los ejemplos de alertas y el panel de Grafana se describen en [Monitorización](docs/es/MONITORING.md).

</details>

<details>
<summary><b>Uso de la API</b></summary>

Crea un token en la página «Tokens de API» del panel de administración (puedes crear varios, conceder solo lectura o lectura y escritura por módulo, definir una fecha de caducidad; el texto del token se muestra una sola vez tras crearlo) y tus scripts podrán llamar en lote a la API de administración:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks
```

Las API de cuenta, verificación en dos pasos y gestión de tokens, así como las de copia de seguridad, restauración, reinicio y ajustes del sistema, solo admiten sesiones iniciadas. Todas las API, los permisos necesarios y las estructuras de datos están en la página «Documentación de la API» del panel de administración; también puedes exportarlos con `make api-docs` a [docs/api](docs/api/es) (`openapi.json` y `API.md`), que el sitio web https://gately.cn referencia directamente al publicarse.

Al actualizar desde una versión anterior, el token de API global original se convierte automáticamente en un «token heredado» (el valor no cambia y conserva todos los permisos de módulo). Se recomienda restringir sus permisos o sustituirlo por un token nuevo en la página de tokens de API.

</details>

<details>
<summary><b>Tailscale, frp, cloudflared</b></summary>

Estos tres módulos gestionan programas externos oficiales (reinicio automático, registros agregados en la interfaz). Gately no los incluye. La tarjeta «Instalación en línea» de cada página de módulo descarga del proyecto original la última versión estable o una versión concreta, verifica su SHA-256, la instala en `bin/` dentro del directorio de configuración y reinicia los procesos que la usan; cada programa puede tener su propio proxy de descarga y, si se deja vacío, se usa el proxy de actualización de los ajustes. Se conservan las 3 últimas versiones sustituidas, para revertir con un clic si la nueva falla, y el programa se puede desinstalar cuando ya no se necesita. El botón «Restaurar valores predeterminados» de la cabecera es independiente de la desinstalación: detiene los procesos, borra los ajustes del módulo y elimina los archivos que Gately generó para él (archivos de configuración de frp, datos de las instancias de File Browser, estado de sesión de Tailscale); la configuración anterior se puede recuperar en el historial de configuración. Tailscale solo publica paquetes listos para ejecutar en Linux; en otros sistemas usa el instalador oficial e indica la ruta. Los programas se buscan en este orden: la ruta indicada en la configuración del módulo, `bin/` en el directorio de configuración y `PATH`. También puedes colocar el binario tú mismo en el directorio de configuración, en Docker por ejemplo `./gately-data/bin/frpc`; Tailscale requiere proporcionar tanto `tailscale` como `tailscaled`. Los tokens y claves de los archivos de configuración se guardan con permisos 0600, y el token de cloudflared se pasa mediante una variable de entorno, por lo que no aparece en la lista de procesos.

El módulo File Browser aloja [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser) (el filebrowser original está archivado y ya no recibe correcciones de seguridad) con el mismo orden de búsqueda y la misma instalación, actualización y reversión en línea; se instala en `bin/filebrowser`. La contraseña del administrador se pasa mediante una variable de entorno y nunca se escribe en el archivo de configuración.

</details>

## Funciones

Un recorrido rápido por la interfaz de administración. La lista completa de módulos está al final de esta sección.

### Todo de un vistazo

El resumen muestra CPU, memoria, disco y tráfico, las interfaces de red con su IPv4 e IPv6 públicas y el estado de cada módulo, para que veas en segundos si algo requiere atención.

<img src="docs/images/overview.webp" alt="Todo de un vistazo" width="100%">

### Servicios web que pueden sustituir a nginx

Proxy inverso, archivos estáticos y redirecciones por dominio, con HTTPS y HTTP/3. Cada sitio tiene su propio WAF, reglas de IP e inicio de sesión, y los cambios se aplican sin cortar conexiones.

<img src="docs/images/web.webp" alt="Servicios web que pueden sustituir a nginx" width="100%">

<p><b>Estadísticas por sitio</b> — solicitudes, errores, estado de los backends y principales IP y países de origen, minuto a minuto.</p>

<img src="docs/images/site-stats.webp" alt="Estadísticas por sitio" width="100%">

### Un solo puerto 443 para todos tus servicios

El reenvío de puertos con enrutamiento SNI permite que varios servicios compartan un mismo puerto según el dominio: el TLS pasa tal cual y el protocolo PROXY conserva la dirección real del cliente.

<img src="docs/images/portforward-sni.webp" alt="Un solo puerto 443 para todos tus servicios" width="100%">

### DNS dinámico con los principales proveedores

Cloudflare, Alibaba Cloud, DNSPod, Route 53 y más. IPv4 e IPv6, direcciones obtenidas de una URL, una interfaz de red o un comando, y un aviso cada vez que algo cambia.

<img src="docs/images/ddns.webp" alt="DNS dinámico con los principales proveedores" width="100%">

### Prevención de intrusiones

Reglas al estilo de fail2ban leen registros de archivos, journald y contenedores, mientras los puertos trampa y la detección de escaneos atrapan los sondeos. Los bloqueos se sincronizan con nftables y el kernel descarta el tráfico.

<img src="docs/images/guard.webp" alt="Prevención de intrusiones" width="100%">

### frp, Tailscale y túneles de Cloudflare

Varias instancias de frpc y frps en paralelo, junto a Tailscale y los túneles de cloudflared. Gately instala, actualiza y revierte estos programas por ti.

<img src="docs/images/frp.webp" alt="frp, Tailscale y túneles de Cloudflare" width="100%">

### Y además

<table>
<tr>
<td width="50%" valign="top">

**Docker**<br>Contenedores, Compose, registros y terminales, con actualización de imágenes en un clic y reversión automática si falla.

<img src="docs/images/docker.webp" alt="Docker" width="100%">

</td>
<td width="50%" valign="top">

**Terminal web**<br>SSH, Telnet y la shell local en el navegador, con SFTP y paneles divididos.

<img src="docs/images/terminal.webp" alt="Terminal web" width="100%">

</td>
</tr>
</table>

<table>
<tr>
<td width="50%" valign="top">

**Historial de configuración**<br>Cada cambio conserva la versión anterior, con comparación de diferencias y reversión en un clic.

<img src="docs/images/config-history.webp" alt="Historial de configuración" width="100%">

</td>
<td width="50%" valign="top">

**API, línea de comandos y MCP**<br>Tokens de API por módulo, documentación OpenAPI integrada, `gately ctl` y un servidor MCP para asistentes de IA.

<img src="docs/images/api-docs.webp" alt="API, línea de comandos y MCP" width="100%">

</td>
</tr>
</table>

<details>
<summary><b>Lista completa de funciones</b></summary>

| Módulo | Capacidades |
| --- | --- |
| DNS dinámico | Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2 (No-IP, Dynu, …), callback personalizado; obtención de IP por URL/interfaz de red/comando; IPv4/IPv6; Webhook; grupos, orden y búsqueda |
| Túnel STUN | TCP/UDP, keep-alive, UPnP/NAT-PMP, reenvío integrado, activación de Webhook y scripts al cambiar la dirección |
| Reenvío de puertos | TCP/UDP, IPv4/IPv6, rangos de puertos, conmutación por error con varios destinos, límite de conexiones, estadísticas de tráfico, protocolo PROXY v1/v2; enrutamiento SNI que permite que varios servicios compartan un mismo puerto como el 443 (TLS de paso según el dominio, un destino predeterminado, protocolo PROXY por ruta); grupos, orden y búsqueda |
| Certificados SSL | Emisión y renovación automáticas con ACME mediante DNS-01 (comodín; zona DNS de subdominios delegados detectada mediante SOA) o HTTP-01 (sin proveedor DNS), certificados de corta duración y para direcciones IP públicas (perfil shortlived de Let's Encrypt), EAB, carga manual, lectura desde archivo, mapeo a un directorio y ejecución de scripts; tras actualizar un certificado, envío por SSH a otros hosts con ejecución de comandos, o reinicio de los contenedores Docker que lo usan |
| Tailscale | Gestiona tailscaled (red en espacio de usuario, apta para contenedores) o se conecta a un demonio existente del sistema; clave de autenticación/enlace de inicio de sesión, Headscale, rutas de subred, nodo de salida, lista de nodos y proxy SOCKS5, puerto fijo, argumentos adicionales y variables de entorno de tailscaled; instalación, actualización, reversión y desinstalación en línea en Linux; restauración de valores predeterminados |
| frp | Varias instancias de frpc y frps, cada una por separado: Cliente frpc (proxies tcp/udp/http/https/stcp/xtcp, los cambios se aplican al añadir o eliminar) y servidor frps (lista blanca de puertos, subdominios, panel de control); importación con un clic de configuraciones ini antiguas y toml nuevas; instalación, actualización, reversión y desinstalación en línea; restauración de valores predeterminados |
| Túnel Cloudflare | Túnel con token de cloudflared y túnel temporal de trycloudflare, varios túneles, reinicio automático, visualización de la dirección pública; instalación, actualización, reversión y desinstalación en línea; restauración de valores predeterminados; edición de las rutas de los túneles con token mediante la API de Cloudflare y creación de sus registros DNS |
| Servicio web | Proxy inverso/redirección/archivos estáticos/texto fijo/cerrar conexión (como return 444 de nginx) por dominio; HTTPS y HTTP/3 (QUIC), HTTP y HTTPS en el mismo puerto (redirección o servicio directo); WebSocket; envío y recepción del protocolo PROXY; autenticación básica, autenticación web, inicio de sesión con GitHub/Google/QQ/OIDC, usuarios compartidos con verificación en dos pasos TOTP e inicio de sesión único entre subdominios, proveedor OIDC integrado para que Gitea, Grafana y otras aplicaciones OpenID Connect inicien sesión con los usuarios compartidos, autenticación delegada (forward auth) a un servicio externo como Authelia, un Gately público puede reenviar sus cookies de inicio de sesión a un Gately de la red local; listas blancas y negras de IP y listas de IP suscritas con actualización periódica (como rangos de IP por provincia o ciudad), reglas por ruta, enlaces temporales para compartir, bloqueo automático tras fallos de autenticación, filtrado de países con GeoIP; backends por ruta, backends por usuario con sesión iniciada (como map $remote_user de nginx), backends HTTPS que reciben como SNI el dominio visitado, pesos de backend/respaldo/balanceo de carga, comprobaciones de salud activas y pasivas, compresión, limitación de tasa, caché de respuestas, reescritura de rutas; sitios estáticos con URL limpias y una página 404 personalizada; los cambios en los sitios se aplican sin cortar las conexiones abiertas; grupos, ordenación y búsqueda de sitios; estadísticas de acceso por minuto y métricas de Prometheus; los usuarios compartidos pueden cambiar su propia contraseña; los sitios pueden cerrar la conexión en vez de responder 403 al denegar el acceso |
| Cortafuegos WAF | Coraza + OWASP CRS v4, activación de reglas por archivo, SecLang personalizado, bloqueo/solo detección; límite de inspección del cuerpo de la solicitud ajustable, con inspección solo de la primera parte u omisión por ruta al superarlo, para poder proteger con WAF también nubes personales, WebDAV y otros sitios de subida |
| Prevención de intrusiones | Reglas de registros al estilo fail2ban (archivos de registro, diario de systemd, salida de comandos, registros de contenedores Docker, inicios de sesión fallidos en el panel, bloqueos del WAF) con plantillas para SSH, Nginx, Vaultwarden y más; puertos trampa y detección de escaneos; una lista de bloqueos con bloqueos manuales de redes CIDR y permanentes, duraciones progresivas y lista blanca; los bloqueos se sincronizan con el cortafuegos del sistema (nftables) y se descartan en el kernel, o se aplican a los servicios de Gately donde no hay nftables; bloqueo por sondeo de rutas web por escucha: rutas señuelo como /.env y /.git, escaneos de dominios desconocidos y 404 repetidos |
| Gestión de Docker | Creación y edición de contenedores, registros y terminal, Compose, comprobación de actualizaciones de imágenes y actualización con un clic (reversión automática si falla), redes y volúmenes |
| Terminal web | SSH (confianza en la clave del host en el primer uso), Telnet, shell local (desactivado por defecto); proxy, sesiones persistentes, gestión de archivos SFTP, 2FA y confirmación de protocolo, comandos rápidos, paneles divididos |
| Diagnóstico de red | Ping, traceroute, sondeo de puertos, consultas DNS, prueba de velocidad, escaneo de la red local |
| Centro de notificaciones | Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu, correo electrónico; niveles, comodines y reglas de silencio y deduplicación; plantillas de reglas integradas (fallos críticos, vencimiento de certificados, cambios de dirección, inicios de sesión, cambios de configuración) |
| Copia de seguridad de la configuración | Copias cifradas (scrypt + AES-GCM), política de retención, copias programadas a un directorio local o a un punto de montaje de almacenamiento |
| Línea de comandos y MCP | `gately ctl` invoca una instancia en ejecución; `gately mcp` ofrece un servicio MCP por stdio (solo lectura por defecto) |
| Wake-on-LAN | Difusión de paquetes mágicos, detección de estado en línea |
| WebDAV | Multiusuario, directorio independiente por usuario, solo lectura; usuarios compartidos comunes a WebDAV, FTP y SMB |
| FTP | Multiusuario, directorio independiente por usuario, solo lectura, compatible con FTPS y rango de puertos pasivos; usuarios compartidos comunes a WebDAV, FTP y SMB |
| SMB | Multiusuario, recursos compartidos por usuario con uno o varios directorios, solo lectura, compatible con SMB 2/3, cifrado y descubrimiento Bonjour; usuarios compartidos comunes a WebDAV, FTP y SMB |
| File Browser | Aloja el gestor de archivos web FileBrowser Quantum: varias instancias, varios directorios compartidos con solo lectura, autenticación por contraseña o por proxy inverso; instalación, actualización, reversión y desinstalación en línea; restauración de valores predeterminados |
| Montajes de almacenamiento | Directorios locales, montajes WebDAV remotos y recursos compartidos SMB, gestión de archivos en línea |
| Tareas programadas | Expresiones cron, comandos o solicitudes HTTP, historial de ejecuciones |
| Gestión de listas | Reglas, instancias, certificados, registros DNS, conexiones de terminal, tokens de API y otras listas comparten grupos, orden manual (arrastrar o flechas, con selección múltiple) y búsqueda, y la mayoría alterna entre vista de tarjetas y compacta; reordenar o agrupar no reinicia ningún servicio y funciona en el móvil |
| Seguridad y ajustes | Inicio de sesión con passkey (WebAuthn), verificación en dos pasos (TOTP), cuentas de solo lectura, limitación de intentos de inicio de sesión, entrada de seguridad, acceso solo desde la red interna, HTTPS para la interfaz de administración, varios tokens de API (permisos de lectura/escritura por módulo), contraseñas y claves guardadas visibles bajo demanda (requiere permiso de escritura), DNS personalizado (compatible con DoH), zona horaria, copia de seguridad y restauración de la configuración, dirección real del cliente desde proxies inversos locales de confianza, activación o desactivación de módulos de funciones (los módulos desactivados no inician servicios ni ofrecen API) |
| Aplicación nativa para fnOS | Paquetes `.fpk` sin conexión (x86 / ARM); integrada en la pasarela unificada de fnOS, se abre desde el icono del escritorio de fnOS y funciona a través de tu dominio remoto de fnOS; los administradores de fnOS pueden iniciar sesión con su sesión de fnOS (desactivado por defecto); se ejecuta como root para escuchar en 80/443 y gestionar Docker |
| Auditoría e historial | Registro de auditoría de inicios de sesión, cambios de configuración y accesos a contenido sensible (nunca registra cuerpos de petición ni secretos); cada archivo de configuración sobrescrito conserva sus versiones anteriores con comparación de diferencias y reversión con un clic; directorio de configuración declarativa (`-apply`) para GitOps |
| Monitorización y autodiagnóstico | Endpoint global de Prometheus `/api/metrics` (certificados, DDNS, STUN, reenvío de puertos, servicios web) con un panel de Grafana listo para usar; autodiagnóstico de red con un clic (IP pública, CGNAT, reloj, puertos 80/443, resolución DDNS, vencimiento de certificados, sitios HTTPS sin certificado, conflictos de proxies frp) |
| Migración | Importación de configuración de nginx (admite la salida de `nginx -T`): proxy inverso y upstream, directorios estáticos, redirecciones, backends por ruta, control de acceso, con `auth_request` convertido en autenticación delegada |

</details>

## Por qué Gately

Gately es totalmente abierto para que usuarios y comunidad puedan revisar directamente el código fuente y la configuración. Un software que se ejecuta en el borde de la red y maneja claves DNS y claves privadas de certificados debe permitir que los usuarios vean lo que hace. Los compromisos de Gately:

- El código fuente es abierto bajo Apache-2.0 y acepta la revisión de la comunidad
- Por defecto no se conecta a ningún servidor del proyecto ni reporta información del dispositivo; el inicio de sesión de terceros usa tu propia aplicación OAuth, sin intermediarios
- Todas las direcciones externas (consulta de IP pública, servidores STUN, etc.) son visibles, editables y se pueden desactivar
- La configuración es JSON en texto plano, que puedes versionar en git para auditoría y reversión

## Rendimiento

El reenvío de puertos con enrutamiento SNI junto con el servicio web puede sustituir la pareja habitual haproxy (enrutamiento TCP por SNI) + nginx (TLS y proxy inverso). Las cifras siguientes comparan ambas soluciones en la misma máquina con la configuración de un servidor real: 90 dominios enrutados por SNI, protocolo PROXY v2 entre los dos saltos y un certificado ECDSA P-256 con TLS 1.3.

| Escenario | haproxy + nginx | Gately |
| --- | --- | --- |
| Proxy inverso HTTPS, keep-alive, respuestas de 1 KB, 64 conexiones | 14.767 req/s (p99 35,9 ms) | **26.434 req/s** (p99 5,7 ms) |
| Lo mismo sobre HTTP/2 | no probado (la configuración de nginx usaba HTTP/1.1) | 27.795 req/s |
| Una conexión TLS nueva por cada petición, 32 a la vez | 2.557 req/s | **3.448 req/s** |
| Respuestas de 1 MB, 16 conexiones | 1.053 MB/s | **1.585 MB/s** |
| Paso directo SNI de un flujo TLS, 4 conexiones | 2.927 MB/s | **4.962 MB/s** |
| Paso directo SNI, conexiones nuevas por segundo, 64 a la vez | 6.334 /s | 6.266 /s (a la par) |
| Memoria bajo carga | haproxy ≈ 21 MB + nginx ≈ 21 MB | ≈ 110 MB (un único proceso con todos los módulos) |

<details>
<summary><b>Cómo se midió</b></summary>

- Equipo: Intel Core i5-13500T. Cada pila se limitó a 2 núcleos de CPU (haproxy y nginx en Docker con `--cpuset-cpus`, Gately fijado con `taskset`) para equipararse a un servidor en la nube pequeño. Los backends y el generador de carga se ejecutaron en otros núcleos.
- haproxy usó `mode tcp` con reglas `req_ssl_sni` y `send-proxy-v2`. nginx usó `worker_processes 2` y los ajustes de proxy del sitio de producción, que envían `Connection: upgrade` y por tanto abren una conexión nueva al backend por cada petición; un nginx optimizado con keep-alive hacia el upstream reduciría la diferencia en respuestas pequeñas.
- El recorrido de Gately fue la regla SNI, PROXY v2 y después un listener del servicio web, con el mismo número de saltos que haproxy → nginx.
- Cada caso se ejecutó durante 10 segundos, y los casos repetidos coincidieron con una diferencia de hasta el 3 %. En un servidor en la nube real de 2 vCPU, el sitio estático sirvió 5,5k req/s frente a 3,4k req/s. Las conexiones TLS nuevas quedaron a la par (unas 480–550 por segundo en ambos) cuando el cliente usaba X25519; con el intercambio de claves híbrido poscuántico que ofrecen los clientes Go y los navegadores recientes, Gately completó en esa prueba un 13 % menos de handshakes.
- Los resultados dependen del hardware, la configuración y el tráfico. Tómalos como una comprobación de que Gately sigue el ritmo de haproxy y nginx, y mide con tu propia carga de trabajo.

</details>

## Desarrollo

Requisitos: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # compila el frontend (salida en web/dist, incrustado con go:embed)
make build    # compila bin/gately
make test     # go test -race ./...
make lint     # gofmt, go vet, comprobación de tipos del frontend
```

Para desarrollar el frontend, ejecuta `cd web && pnpm dev`; `/api` se redirige al servidor local `127.0.0.1:16680`.

Algunas pruebas se omiten por defecto y requieren condiciones adicionales:

| Prueba | Cómo activarla |
| --- | --- |
| ACME de extremo a extremo (Pebble) | Instala pebble y pebble-challtestsrv y define `GATELY_PEBBLE_BIN` y `GATELY_PEBBLE_SRC` (se ejecuta automáticamente en CI) |
| Servidores STUN públicos reales | `GATELY_NET_TEST=1` |

La estructura de directorios y la selección tecnológica están en [docs/es/ROADMAP.md](docs/es/ROADMAP.md).

El código del sitio web ([gately.cn](https://gately.cn)) está en `website/` y se basa en VitePress: `cd website && pnpm install && pnpm dev`; `pnpm deploy:site` compila y sincroniza con el servidor.

## Normas de contribución

- El código de compatibilidad debe basarse únicamente en el comportamiento, las interfaces y los formatos de configuración observables públicamente. No envíes código copiado de los binarios o del frontend de software de terceros.
- Los módulos nuevos requieren: estructura de configuración y validación, inicio y parada en tiempo de ejecución, interfaz REST y pruebas unitarias; los módulos que quieran mostrar su estado en la página de resumen implementan `module.Summarizer`.
- Los comentarios del código se escriben en chino; las sentencias de control deben usar siempre llaves.

## Comunidad

Las preguntas, comentarios y consejos de uso son bienvenidos en el grupo de Telegram: [t.me/gately_chat](https://t.me/gately_chat), o escanea el código QR con tu teléfono:

<a href="https://t.me/gately_chat"><img src="docs/images/telegram-qr.svg" alt="Escanea para unirte al grupo de Telegram de Gately" width="160"></a>

Los errores se reportan en [GitHub Issues](https://github.com/wangwanjie/gately/issues); adjunta un [paquete de diagnóstico](docs/es/DIAGNOSTICS.md) (Ajustes → Diagnóstico). Se genera localmente y no contiene contraseñas, tokens, dominios ni IP reales.

## Patrocinar

Todas las funciones de Gately son gratuitas. Si te resulta útil, puedes apoyar el desarrollo con USDT, [Afdian](https://ifdian.net/a/gately) o WeChat Pay; las direcciones y los códigos QR están en la [página de patrocinio](https://gately.cn/es/sponsor).

## Evolución de estrellas

[![Star History Chart](https://api.star-history.com/svg?repos=wangwanjie/gately&type=Date)](https://star-history.com/#wangwanjie/gately&Date)

## Licencia

[Apache License 2.0](LICENSE)
