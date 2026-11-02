# Instalación

## Docker

Imagen: `ghcr.io/wangwanjie/gately`, disponible para amd64, arm64 y arm/v7.

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

### Docker Compose

```yaml
services:
  gately:
    image: ghcr.io/wangwanjie/gately:latest
    container_name: gately
    restart: unless-stopped
    # El reenvío de puertos, el traversal STUN y los servicios web escuchan directamente en puertos del host
    network_mode: host
    # Prevención de intrusiones escribe los bloqueos en el cortafuegos del anfitrión (elimínalo si no lo necesitas)
    cap_add:
      - NET_ADMIN
    volumes:
      # Directorio de configuración (JSON plano, se puede guardar en git; ssl.json contiene claves privadas de certificados)
      - ./data:/etc/gately
      # Gestión de Docker (quítalo si no lo necesitas)
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Shanghai
```

`--cap-add NET_ADMIN` y `cap_add` permiten que [Prevención de intrusiones](/es/docs/modules/guard) escriba los bloqueos en el cortafuegos nftables del anfitrión. Puedes quitarlos; los bloqueos seguirán aplicándose a los servicios de Gately.

La imagen incluye `sh`, `bash` y `curl`, así que los scripts de notificación habituales funcionan sin cambios.

## Binario

1. Descarga el archivo para tu plataforma desde la [página de descargas](/es/download): Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows o FreeBSD.
2. Descomprímelo y ejecuta:

```bash
./gately -c /etc/gately
```

### systemd

El repositorio incluye un archivo de servicio completo, [deploy/systemd/gately.service](https://github.com/wangwanjie/gately/blob/main/deploy/systemd/gately.service), que ya concede las capacidades necesarias para escuchar en puertos bajos:

```bash
sudo install -m 0755 gately /usr/local/bin/gately
sudo install -m 0644 gately.service /etc/systemd/system/gately.service
sudo systemctl daemon-reload && sudo systemctl enable --now gately
```

### OpenWrt {#openwrt}

Cada versión incluye paquetes para OpenWrt con el binario, el script de arranque de procd y una regla que conserva tu configuración al actualizar el firmware:

- OpenWrt 24.10 y anteriores (incluidos iStoreOS y otros firmwares basados en 24.10) usan el `.ipk`, que se instala con `opkg install`;
- OpenWrt 25.12 y posteriores usan el `.apk`, que se instala con `apk add --allow-untrusted` (los paquetes no están firmados, así que `--allow-untrusted` es obligatorio).

Ejecuta `grep DISTRIB_ARCH /etc/openwrt_release` para conocer la arquitectura del dispositivo y elige el paquete en el grupo OpenWrt de la [página de descargas](/es/download):

| DISTRIB_ARCH | Paquete |
| --- | --- |
| `x86_64` | `gately_<version>_openwrt_amd64` |
| `aarch64_*` | `gately_<version>_openwrt_arm64` |
| `arm_cortex-a*_*` (ARMv7 con VFP / NEON) | `gately_<version>_openwrt_arm` |
| `mipsel_*` | `gately_<version>_openwrt_mipsle` |
| `mips_*` | `gately_<version>_openwrt_mips` |
| `riscv64_*` | `gately_<version>_openwrt_riscv64` |

```bash
cd /tmp
# OpenWrt 24.10 / iStoreOS (ejemplo con x86_64)
opkg install ./gately_<version>_openwrt_amd64.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./gately_<version>_openwrt_amd64.apk
```

- El servicio se activa en el arranque y se inicia justo después de instalar; la interfaz de administración está en `http://<dirección del router>:16680`. La configuración se guarda en `/etc/gately` y se conserva al actualizar el firmware (sysupgrade);
- Actualizar: descarga el paquete más reciente y vuelve a ejecutar `opkg install` o `apk add --allow-untrusted`; tu configuración se conserva. Si desactivaste el inicio del servicio en el arranque, seguirá desactivado tras la actualización;
- Desinstalar: `opkg remove gately` o `apk del gately`. El directorio de configuración se conserva; ejecuta `rm -rf /etc/gately` para eliminarlo también;
- Antes de instalar se comprueba la arquitectura del dispositivo. opkg cancela la instalación si la arquitectura no coincide; el apk de OpenWrt 25.12 instala el paquete igualmente pero deja el servicio detenido, así que ejecuta `apk del gately` e instala el paquete correcto;
- El binario ocupa unos 32 MB y necesita la misma cantidad de espacio libre (compruébalo con `df -h /overlay`), por lo que es adecuado para routers software x86, equipos ARM64 y otros dispositivos con almacenamiento amplio; los routers con 16 MB / 32 MB de flash se quedan cortos;
- El paquete arm requiere ARMv7 con coma flotante por hardware. Los dispositivos ARMv5 / ARMv6 y los núcleos sin FPU como `arm_cortex-a9` todavía no son compatibles.

También puedes desplegarlo manualmente: descarga el archivo para Linux y usa el script de arranque de procd [deploy/openwrt/gately.init](https://github.com/wangwanjie/gately/blob/main/deploy/openwrt/gately.init):

```bash
cp gately /usr/bin/gately && chmod +x /usr/bin/gately
cp gately.init /etc/init.d/gately && chmod +x /etc/init.d/gately
/etc/init.d/gately enable && /etc/init.d/gately start
```

#### Aplicación LuCI {#openwrt-luci}

Cada versión incluye además una aplicación LuCI independiente de la arquitectura, `luci-app-gately_<versión>_openwrt_all.ipk` / `.apk`. Instálala después de Gately:

```bash
# OpenWrt 24.10 / iStoreOS
opkg install ./luci-app-gately_<versión>_openwrt_all.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./luci-app-gately_<versión>_openwrt_all.apk
```

Después abre **Servicios → Gately** en LuCI para:

- ver el estado del servicio, la versión, la memoria usada y si arranca al iniciar;
- iniciar, detener y reiniciar el servicio, y activar o desactivar el inicio automático;
- abrir la interfaz de administración de Gately con un clic, con la entrada segura de la configuración global ya incluida en el enlace;
- definir el puerto de la interfaz de administración. Se guarda en `/etc/config/gately` y, si se indica, sustituye el puerto de la dirección de escucha de la configuración global de Gately (se conserva la parte de host, como una dirección de loopback o de LAN), lo que sirve cuando la interfaz no abre por un conflicto de puertos o un puerto equivocado; déjalo vacío para seguir la configuración global.

Todo lo demás se sigue configurando en la interfaz de administración de Gately. La aplicación sigue el idioma de LuCI y está traducida al chino simplificado y tradicional, inglés, japonés, coreano, alemán, francés, español, ruso y portugués (Brasil). Para desinstalar, quita primero `luci-app-gately` (`opkg remove luci-app-gately` o `apk del luci-app-gately`) y luego gately.

## Aplicación nativa para fnOS {#fnos}

Cada versión incluye paquetes sin conexión para fnOS: `gately_<version>_fnos_x86.fpk` (Intel / AMD) y `gately_<version>_fnos_arm.fpk` (ARM). No figuran en el Centro de aplicaciones oficial, así que debes instalarlos manualmente:

1. Descarga el `.fpk` correspondiente a la arquitectura de tu dispositivo desde la [página de descargas](/es/download#fnos);
2. En fnOS, abre «Centro de aplicaciones → Instalación manual» (el botón de la esquina inferior izquierda), elige el archivo y completa el asistente;
3. Haz clic en el icono de Gately del escritorio de fnOS para abrir la interfaz de administración.

### Funciona con la pasarela unificada de fnOS

El icono del escritorio abre la interfaz de administración a través de la pasarela unificada de fnOS en `/app/gately/` de la dirección que usas para fnOS, compartiendo el mismo punto de entrada. Funcionan tanto la dirección de la red local como tu dominio remoto de fnOS, sin necesidad de abrir ni redirigir el puerto 16680.

El acceso a través de la pasarela lo controla fnOS, y solo los administradores de fnOS pueden abrirla. La ruta de acceso segura y «Acceso solo desde la red local» se aplican únicamente al acceso directo por el puerto 16680. En la red local, `http://<dirección del NAS>:16680` sigue funcionando.

### Iniciar sesión con tu sesión de administrador de fnOS

Activa «Iniciar sesión con fnOS» en «Ajustes → Control de acceso» y los administradores que tengan la sesión iniciada en fnOS entrarán en Gately directamente desde el icono del escritorio, sin contraseña ni código de verificación en dos pasos:

- Solo los administradores de fnOS inician sesión de esta forma, como administrador de Gately; los usuarios normales de fnOS siguen introduciendo usuario y contraseña;
- Solo se aplica al acceso a través de la pasarela unificada; el acceso directo por el puerto 16680 sigue requiriendo un inicio de sesión normal;
- Tras cerrar la sesión, Gately espera a que vuelvas a entrar tú mismo: la página de inicio de sesión muestra el botón «Iniciar sesión como usuario de fnOS &lt;nombre&gt;»;
- El ajuste está desactivado por defecto y solo aparece en el paquete de fnOS.

### Notas

- La aplicación se ejecuta como `root`, de modo que puede escuchar en 80/443, gestionar Docker mediante `docker.sock` y abrir un terminal web real. Instálala solo en un NAS que controles.
- La configuración se guarda en el directorio `var` de la aplicación y se conserva al desinstalar, salvo que elijas «Eliminar todo» en el asistente de desinstalación.
- El paquete también añade el comando `/usr/local/bin/gately` para `gately ctl`, `gately mcp` y `gately reset-password`.
- El puerto 16680 se comparte con la imagen de Docker. Detén el contenedor `gately` antes de iniciar la aplicación nativa.

## Primer inicio de sesión

La interfaz de administración escucha por defecto en el puerto `16680`. El usuario predeterminado es `admin` y la contraseña predeterminada, `gately.cn`. En despliegues automatizados puedes fijar otra contraseña inicial con la variable de entorno `GATELY_INITIAL_PASSWORD`.

La contraseña predeterminada es pública; mientras sigas usándola, la interfaz de administración mostrará en la parte superior un aviso para que la cambies. El primer inicio de sesión muestra una guía de bienvenida que te propone, paso a paso, cambiar la contraseña, activar la autenticación de dos factores o una passkey, ejecutar el autodiagnóstico de red, importar desde Lucky y configurar las notificaciones.

## Restablecer la contraseña y la autenticación de dos factores {#reset}

Si olvidas la contraseña del administrador o pierdes el autenticador (TOTP), restablécelos con un comando en el servidor donde se ejecuta Gately. El comando necesita leer y escribir el directorio de configuración, lo que equivale a tener acceso al servidor, por lo que no se puede usar a través de la red:

- `gately reset-password`: restablece la contraseña del administrador a la predeterminada, `gately.cn`; con `-p <nueva contraseña>` puedes fijar directamente una nueva;
- `gately reset-totp`: desactiva la autenticación de dos factores del administrador; puedes volver a activarla después de iniciar sesión.

Antes de ejecutarse, el comando muestra el nombre del administrador y la operación que va a realizar, y pide escribir `yes` para confirmar y evitar errores; en scripts y otros entornos no interactivos, añade `-y` para omitir la confirmación. Una instancia de Gately en ejecución aplica el cambio en pocos segundos sin reiniciarse; si no está en ejecución, se aplica en el siguiente arranque. Tras el restablecimiento se invalidan todas las sesiones iniciadas y se levantan los bloqueos por intentos fallidos; el nombre de usuario, las passkeys, las cuentas de solo lectura y el resto de la configuración no cambian.

| Tipo de instalación | Comando |
| --- | --- |
| Docker | `docker exec -it gately gately reset-password` |
| systemd | `sudo gately reset-password` |
| OpenWrt / iStoreOS | `gately reset-password` |
| fnOS | `sudo gately reset-password` |
| Otros | `./gately reset-password -c <directorio de configuración>` |

Sin `-c`, el comando busca por orden en `GATELY_CONFIG_DIR`, en `data` dentro del directorio actual, en `/etc/gately` y en el directorio de configuración del paquete de fnOS, `/var/apps/gately/var/config`, y muestra el directorio que usa. Cada restablecimiento queda registrado en el registro del sistema y en el registro de auditoría.

## Actualización

### Comprobación automática de actualizaciones y avisos de seguridad {#update-check}

Por defecto, Gately consulta GitHub cada 12 horas para leer la lista de versiones de Gately y los [avisos de seguridad publicados](https://github.com/wangwanjie/gately/security/advisories), y comprueba la última versión de los programas gestionados instalados en línea (frp, túnel de Cloudflare, Tailscale, File Browser). Todas las solicitudes son GET anónimas y no incluyen información del dispositivo, la configuración ni el uso.

- **Nueva versión**: aparece un aviso en la parte superior de la interfaz; puedes ver las novedades u omitir esa versión;
- **Actualización de seguridad**: si la versión actual está dentro del rango afectado de un aviso, o una versión más reciente incluye en sus notas una sección de correcciones de seguridad, un aviso rojo en la parte superior enumera los avisos y versiones. El aviso de seguridad solo se puede posponer 24 horas y vuelve de inmediato si aparece un problema de seguridad nuevo;
- **Notificaciones**: una versión nueva o un problema de seguridad nuevo publica el evento `system.update` / `system.security`, y una versión nueva de un programa gestionado publica `program.update`. Añade una regla con la plantilla «Actualizaciones» en [Notificaciones](/es/docs/modules/notify) para recibirlas. Cada versión y cada problema de seguridad se notifica una sola vez, también tras reiniciar.

El resultado se guarda en `update-check.json` en el directorio de configuración y, tras actualizar, se vuelve a evaluar para la nueva versión sin otra comprobación. La primera comprobación se hace unos 3 minutos después del arranque (o espera hasta que pasen 12 horas desde la última correcta) y, si falla, se reintenta al cabo de una hora; GitHub se consulta a través del proxy de actualización descrito abajo. Puedes desactivarla en la página Acerca de o en Ajustes → Red y ejecución; entonces GitHub solo se consulta al pulsar «Buscar actualizaciones». La comprobación automática solo avisa: nunca descarga ni instala nada.

### Actualización en línea {#online-upgrade}

Las instalaciones binarias en Linux (systemd, contenedores LXC de PVE, OpenWrt desplegado manualmente, etc.) pueden actualizarse en línea desde la página Acerca de del panel: pulsa «Buscar actualizaciones» y, si hay una versión nueva, «Actualizar ahora». También sirve para los contenedores LXC creados en PVE a partir de la imagen OCI, sin necesidad de borrarlos y recrearlos. Gately hará lo siguiente:

1. Descargar desde GitHub el `gately_<versión>_linux_<arquitectura>.tar.gz` de esta plataforma;
2. Verificar su SHA-256 con el `checksums.txt` de la misma versión;
3. Probar el nuevo binario con `-version` para confirmar que se ejecuta en esta máquina;
4. Reemplazar el ejecutable y reiniciar con los mismos argumentos, conservando la versión anterior como `gately.old` en el mismo directorio.

Todo ocurre en un cuadro de diálogo: primero muestra las notas de la versión y, al empezar, el paso actual, el progreso y la velocidad de descarga. Hasta que se reemplaza el binario (obtención, descarga y verificación) puedes pulsar «Cancelar actualización» en cualquier momento, sin que el binario actual cambie.

Si cuesta acceder a GitHub, configura un **proxy de actualización** en la página Acerca de o en Ajustes → Red y ejecución. Admite `http://`, `https://` y `socks5://`, y tanto la búsqueda como la descarga pasan por él. Si está vacío, Gately conecta directamente y sigue respetando la variable de entorno `HTTPS_PROXY`.

Sin acceso a internet, descarga el tar.gz desde la [página de descargas](/es/download) y pulsa «Subir paquete de actualización» en la página Acerca de. Se acepta cualquier versión, así que también sirve para volver a una anterior.

La actualización en línea solo se ejecuta cuando un administrador la inicia; Gately nunca descarga ni instala versiones por su cuenta. Si la nueva versión no arranca, vuelve atrás manualmente con la copia conservada (ejemplo con systemd):

```bash
mv /usr/local/bin/gately.old /usr/local/bin/gately
systemctl restart gately
```

Si concediste al binario permiso para puertos bajos con `setcap`, vuelve a ejecutar `setcap` tras actualizar.

### Otros despliegues

- Docker: ejecuta `docker pull` con la nueva imagen y recrea el contenedor (mantén montado el directorio de configuración);
- fnOS: instala manualmente el `.fpk` más reciente desde el Centro de aplicaciones; tu configuración se conserva;
- Paquetes de OpenWrt: descarga el `.ipk` / `.apk` más reciente y ejecuta `opkg install` / `apk add --allow-untrusted`; tu configuración se conserva (consulta [OpenWrt](#openwrt));
- Windows, macOS, FreeBSD: sustituye el ejecutable y reinicia el servicio.

Antes de actualizar, exporta una copia de seguridad de la configuración o guarda el directorio de configuración en git. Puedes comprobar la firma de los archivos e imágenes descargados según [Verificar versiones](/es/docs/guide/verify).
