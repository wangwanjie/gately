# Tailscale / NetBird / EasyTier / frp / cloudflared

Estos cinco módulos integran los programas oficiales como procesos gestionados: reinicio automático, registros recogidos en la interfaz y configuración desde la propia interfaz. Gately no incluye estos programas; se pueden instalar, actualizar y revertir en línea desde la página.

## Instalación, actualización y reversión en línea {#install}

Las páginas de frp, Cloudflare Tunnel, Tailscale, NetBird, EasyTier y [File Browser](/es/docs/modules/tools#filebrowser) tienen al final una tarjeta «Instalación en línea»:

- **Descargar e instalar / Actualizar a la última**: descarga del proyecto original la última versión estable, la verifica con el SHA-256 que publica el proyecto, comprueba que se ejecuta en esta máquina y la instala en `bin/` dentro del directorio de configuración; al terminar reinicia los procesos que usan el programa. Si ya está en la última versión, no vuelve a descargarla;
- **Buscar actualizaciones**: solo consulta la última versión estable del proyecto original, sin descargar; si hay una nueva, aparece «Nueva versión» junto al título de la tarjeta. Con la [comprobación automática de actualizaciones](/es/docs/guide/install#update-check) activada, Gately revisa los programas instalados cada 12 horas y publica una notificación `program.update` cuando hay una versión nueva;
- **Instalar una versión**: indica un número de versión (por ejemplo `v0.65.0` para frp, `2025.8.0` para cloudflared, `1.90.2` para Tailscale); sirve también para volver a una versión anterior;
- **Proxy de descarga**: se configura por programa, admite http, https y socks5 y se guarda al iniciar la instalación; vacío, se usa el proxy de actualización de «Ajustes»;
- **Versiones anteriores y reversión**: las versiones sustituidas por una actualización o reversión se guardan en `bin/.versions/`, hasta 3. Si la nueva versión falla, pulsa «Revertir» en la lista; la versión actual también se guarda, así que después puedes volver a ella;
- **Desinstalar**: elimina el programa instalado en `bin/` y todas las versiones anteriores; los ajustes del módulo se conservan. Los procesos que usan el programa se reinician y pasan a usar un programa con el mismo nombre en `PATH`; si no lo hay, no pueden ejecutarse.

| Programa | Origen | Archivos instalados |
| --- | --- | --- |
| frp | GitHub fatedier/frp | `frpc`, `frps` |
| cloudflared | GitHub cloudflare/cloudflared | `cloudflared` |
| Tailscale | Paquetes estáticos de pkgs.tailscale.com, solo Linux | `tailscale`, `tailscaled` |
| NetBird | GitHub netbirdio/netbird, Linux, macOS y Windows | `netbird` |
| EasyTier | GitHub EasyTier/EasyTier, Linux (amd64, arm64, armv7, armhf, arm, mips, mipsel, riscv64, loongarch64), macOS, Windows y FreeBSD | `easytier-core`, `easytier-cli` (el zip de la versión publicada; el SHA-256 de verificación es el resumen que registra GitHub) |
| FileBrowser Quantum | GitHub gtsteffaniak/filebrowser | `filebrowser` |

Tailscale no publica paquetes listos para ejecutar en macOS ni en Windows: usa el instalador oficial e indica la ruta en los ajustes. La tarjeta avisa cuando la plataforma actual no está soportada.

## Restaurar valores predeterminados {#reset}

El botón «Restaurar valores predeterminados» de la cabecera es independiente de la desinstalación: detiene todos los procesos del módulo, devuelve los ajustes (incluidos la ruta del programa y el proxy de descarga) a sus valores predeterminados y elimina los archivos que Gately generó para él. Los programas instalados se conservan.

| Módulo | Qué se elimina |
| --- | --- |
| frp | Todas las instancias de frpc / frps, los proxies y los ajustes de los programas, y los archivos de configuración generados en `frp/` |
| Cloudflare Tunnel | Todos los túneles y los ajustes del programa |
| Tailscale | Todas las instancias y la configuración de programas, y el estado de inicio de sesión de los directorios de estado predeterminados; un tailscaled gestionado por Gately en ejecución que usa el directorio de estado predeterminado cierra sesión primero. Los directorios de estado personalizados y un tailscaled del sistema existente no se tocan |
| NetBird | Todas las instancias y los ajustes del programa, y el estado de sesión y las claves de los directorios de estado predeterminados; si un demonio gestionado por Gately que usa el directorio de estado predeterminado está en ejecución, primero cierra la sesión y esta máquina se elimina de la red. Los directorios de estado personalizados y el servicio NetBird existente del sistema no se tocan |
| EasyTier | Todas las instancias y los ajustes del programa, incluidas las claves de red; EasyTier no guarda otro estado en el directorio de configuración |
| File Browser | Todas las instancias y los ajustes del programa, con los usuarios, enlaces compartidos y cachés de cada instancia; los archivos de los directorios compartidos quedan intactos |

La configuración anterior queda guardada en el [historial de configuración](/es/docs/guide/audit) y se puede recuperar tras un error. Para eliminarlo todo, desinstala primero el programa y después restaura los valores predeterminados. Las API son `DELETE /api/{module}/install` (desinstalar) y `POST /api/{module}/reset` (restaurar valores predeterminados).

## Ubicación de los programas

Los ejecutables se buscan en este orden:

1. La ruta definida en los ajustes del módulo;
2. El subdirectorio `bin/` del directorio de configuración (donde se hace la instalación en línea);
3. `PATH`.

Si se indica una ruta, la versión instalada en línea no se usa y la tarjeta lo avisa. También puedes colocar el binario tú mismo en el directorio de configuración, en Docker por ejemplo `./gately-data/bin/frpc`. Tailscale necesita tanto `tailscale` como `tailscaled`; si usas el tailscaled que ya tiene el sistema, conviene que el comando `tailscale` instalado tenga la misma versión que el demonio.

Los tokens y las claves se guardan con permisos 0600. El token de cloudflared se pasa mediante una variable de entorno, por lo que no aparece en la lista de procesos.

## Tailscale

Se admiten varias instancias: cada una ejecuta su propio tailscaled (directorio de estado, socket de control, puerto WireGuard y proceso separados), de modo que un mismo equipo puede unirse a tailnets distintas, por ejemplo una instancia TUN del kernel para un Headscale remoto y una instancia en espacio de usuario para uno local.

- El directorio de estado de una instancia nueva es por defecto `tailscale/<clave de instancia>` dentro del directorio de configuración; también puedes indicar un directorio de estado existente (por ejemplo, el que montaba un contenedor de tailscale anterior) para conservar la identidad del nodo sin volver a registrarlo;
- Como máximo puede haber una instancia en modo kernel (gestionada y sin red en espacio de usuario) habilitada a la vez: tailscaled usa una tabla de rutas y una marca de cortafuegos fijas, así que dos instancias de kernel se sobrescribirían. Tampoco pueden compartir dos instancias el socket de control, el directorio de estado ni un puerto WireGuard fijo; al guardar o habilitar se avisa del conflicto;
- Editar, habilitar o deshabilitar una instancia reinicia solo su propio tailscaled, y el origen de los registros lleva el nombre de la instancia (como `tailscaled:casa`);
- Al actualizar desde la 0.6.4 o anteriores, la configuración única anterior se migra a una instancia llamada `tailscale`, con el mismo directorio de estado, socket de control y línea de comandos;
- Las rutas de los programas (tailscaled, tailscale) y la instalación en línea son comunes a todas las instancias.

Cada instancia admite:

- Gestiona `tailscaled` (red en espacio de usuario, apta para contenedores) o se conecta a un demonio del sistema existente;
- Clave de autenticación o enlace de inicio de sesión, con compatibilidad con Headscale;
- Rutas de subred y nodos de salida;
- Lista de nodos y proxy SOCKS5. El proxy no tiene autenticación, así que la interfaz avisa cuando su dirección de escucha es accesible desde fuera de esta máquina (por ejemplo `0.0.0.0` o una IP de la LAN); se recomienda escuchar solo en `127.0.0.1`;
- Puerto UDP de WireGuard fijo, que facilita permitir conexiones directas a través de un cortafuegos;
- Argumentos adicionales y variables de entorno de tailscaled. Si el iptables del host usa el backend nf_tables, añade `TS_DEBUG_FIREWALL_MODE=nftables` a las variables de entorno; de lo contrario, el reenvío de subredes no funcionará.

## NetBird

Se admiten varias instancias: cada instancia ejecuta su propio demonio de NetBird (directorio de estado, dirección del demonio, puerto de WireGuard y proceso independientes), así que una misma máquina puede unirse a distintas redes NetBird, por ejemplo una instancia en modo kernel para NetBird Cloud y una instancia en espacio de usuario para un servidor de gestión propio.

- El directorio de estado de una instancia nueva es por defecto `netbird/<clave de la instancia>` dentro del directorio de configuración, y su dirección del demonio `netbird.sock` dentro del directorio de estado; también puedes indicar un directorio de estado existente (por ejemplo `/var/lib/netbird` montado por un contenedor de NetBird anterior) para conservar la identidad del par sin volver a registrarlo;
- Solo puede haber una instancia en modo kernel (gestionada y sin red en espacio de usuario) habilitada a la vez: el modo kernel de NetBird usa una tabla de rutas, una marca de cortafuegos y una tabla de nftables fijas, y el nombre de interfaz predeterminado siempre es `wt0`, así que dos instancias en modo kernel se sobrescribirían. Dos instancias no pueden compartir directorio de estado ni dirección del demonio; las instancias gestionadas no pueden usar el mismo puerto de WireGuard (0 cuenta como el predeterminado 51820, y NetBird cambia en silencio a un puerto aleatorio si el puerto está ocupado); las instancias gestionadas en espacio de usuario no pueden usar la misma dirección SOCKS5 (vacía cuenta como `127.0.0.1:1080`). Al guardar, habilitar, habilitar en lote y copiar se avisa del conflicto, y en una configuración editada a mano con conflicto, la instancia conflictiva posterior no se inicia y muestra el motivo;
- El mismo límite se aplica a un servicio NetBird instalado aparte en el sistema: antes de habilitar o iniciar una instancia gestionada en modo kernel, Gately consulta las instancias externas habilitadas y el socket de control predeterminado del sistema `/var/run/netbird.sock` (si existe), y comprueba si ya hay una interfaz NetBird llamada `wt` más un número que no pertenezca a una instancia gestionada. Si encuentra otro NetBird conectado en modo kernel, al guardar o habilitar se informa del conflicto y la instancia no se inicia y muestra el motivo; detenga el otro servicio o active la red en espacio de usuario para esta instancia y reiníciela. Una interfaz residual se elimina con `ip link delete wt0`. Un servicio externo que use otro nombre de interfaz con el socket de control no visible (por ejemplo, no montado en Docker) no se puede detectar y esa combinación no es compatible;
- Editar, habilitar o deshabilitar una instancia solo reinicia su propio demonio, y los orígenes de registro llevan el nombre de la instancia (como `daemon:casa` y `netbird:casa`); el enlace de inicio de sesión, el código de dispositivo, la lista de pares y el cierre de sesión también se muestran por instancia;
- Al eliminar una instancia gestionada que usa el directorio de estado predeterminado, primero cierra la sesión y después se elimina ese directorio; los directorios de estado personalizados y el directorio antiguo migrado se conservan para reutilizarlos en una instancia nueva;
- Al actualizar desde 0.6.4 o anterior, la configuración única anterior se migra a una instancia llamada `netbird`, con el mismo directorio de estado (`netbird` dentro del directorio de configuración), dirección del demonio, línea de comandos y variables de entorno;
- La ruta del programa y la instalación en línea son comunes a todas las instancias.

Cada instancia admite:

- Unirse a una red NetBird en NetBird Cloud o en tu propio servidor de gestión (indicando su URL);
- Autenticación con clave de configuración o enlace de inicio de sesión. En Linux el enlace usa inicio de sesión con código de dispositivo y se puede abrir en cualquier dispositivo; en macOS y Windows el inicio de sesión normalmente debe completarse en un navegador de la máquina donde corre Gately, así que usa una clave de configuración para gestionarla en remoto. El enlace se renueva automáticamente al caducar;
- Demonio `netbird` gestionado (red en espacio de usuario, apta para contenedores) o conexión al servicio NetBird ya instalado en el sistema;
- Con red en espacio de usuario, por defecto otros pares pueden llegar a los servicios de esta máquina (incluidos los que solo escuchan en 127.0.0.1; las políticas de acceso del servidor de gestión siguen aplicándose), y los programas locales llegan a los dispositivos de la red mediante un proxy SOCKS5 (por defecto `127.0.0.1:1080`);
- El proxy SOCKS5 no tiene autenticación, y la interfaz avisa cuando su dirección de escucha es accesible desde fuera de esta máquina. NetBird anterior a 0.75 ignora la dirección configurada en la red en espacio de usuario y siempre ofrece un proxy SOCKS5 sin autenticación en `0.0.0.0`, a través del cual otros dispositivos pueden entrar en la red; cuando Gately gestiona el demonio y detecta una de esas versiones, deja una advertencia en el registro. Actualiza a la 0.75 o posterior;
- Las rutas de red, los nodos de salida y las políticas de acceso se configuran en el panel de NetBird; en Gately eliges si aceptar rutas, actuar como par de enrutamiento, aceptar DNS y bloquear conexiones entrantes;
- Lista de pares con conexión directa o por relé, latencia y rutas de red;
- Puerto WireGuard fijo, argumentos adicionales del demonio y variables de entorno (las opciones avanzadas `NB_` de la documentación de NetBird);
- Con la red en espacio de usuario desactivada se usa WireGuard del kernel: necesita root y un dispositivo TUN, crea la interfaz `wt0` y añade sus propias reglas de cortafuegos (en OpenWrt también permite el tráfico entrante de `wt0` en fw4), y los servicios locales deben escuchar en todas las direcciones o en la dirección de NetBird para que otros pares lleguen a ellos;
- El programa `netbird` ocupa unos 41 MB y usa unos 60 MB de memoria, y la instalación en línea guarda hasta 3 versiones antiguas en `bin/.versions/`. En routers con poca flash vigila el espacio libre: elimina versiones antiguas en la tarjeta de instalación o coloca el programa en un almacenamiento externo e indica su ruta en los ajustes;
- Restaurar valores predeterminados y `POST /api/netbird/instances/{key}/logout` dan de baja esta máquina en el servidor de gestión y la eliminan de la red; para volver a unirse hay que autenticarse de nuevo.

## EasyTier

Admite varias instancias: cada instancia ejecuta su propio proceso `easytier-core`, de modo que una misma máquina puede unirse a redes distintas. Los nodos con el mismo nombre de red y la misma clave de red se comunican entre sí. Se intenta primero el hole punching P2P; si falla, el tráfico se retransmite mediante un nodo público o uno autoalojado. Los protocolos de conexión incluyen tcp / udp / ws / wss / quic / wg.

- **Clave de red**: se pasa al proceso mediante la variable de entorno `ET_NETWORK_SECRET`, por lo que no aparece en la línea de comandos y se enmascara en los registros. La interfaz muestra un campo de contraseña con «mostrar / ocultar», y las listas y la API devuelven un valor enmascarado;
- **Nodos (peers)**: una dirección por línea, por ejemplo `tcp://public.easytier.cn:11010`;
- **Direcciones de escucha (listeners)**: deben escribirse como `protocolo://dirección:puerto`. Si se dejan vacías, la instancia no escucha en ningún puerto y solo se conecta hacia fuera. Si actúa como nodo de retransmisión o público, debes abrir esos puertos en el cortafuegos por tu cuenta;
- **IP virtual**: asignación automática (DHCP) o una IPv4 fija. Sin ninguna de las dos, el nodo solo reenvía tráfico y no crea interfaz de red;
- **Modo de ejecución**: una interfaz TUN requiere root y `/dev/net/tun`; en Docker requiere `NET_ADMIN` y `/dev/net/tun`. Si la creación falla, la instancia muestra el motivo y el proceso no termina. La «red en espacio de usuario» no requiere privilegios y es apta para contenedores; los servicios de la red se alcanzan mediante el «reenvío de puertos»;
- **Sin opción SOCKS5**: el SOCKS5 de EasyTier escucha en todas las direcciones y no tiene autenticación. La dirección de enlace de un reenvío de puertos la indicas tú; se recomienda `127.0.0.1`, y al enlazar con una dirección que no sea de loopback la interfaz muestra una advertencia (la entrada del reenvío no tiene autenticación);
- **Otras opciones**: proxy de subred (`proxyNetworks`, admite asignaciones como `192.168.1.0/24->10.1.0.0/24`), nodos de salida (actuar como salida / usar una salida), prioridad de latencia, proxy KCP / QUIC (reduce la latencia en redes con pérdida de paquetes UDP), nombre de la interfaz y argumentos adicionales y variables de entorno (para las opciones `ET_` consulta la documentación de EasyTier);
- **Puerto de gestión**: Gately asigna a cada instancia un puerto aleatorio en `127.0.0.1` y lo limita al acceso local. El estado (IP virtual local, tipo de NAT, direcciones públicas, direcciones de escucha, nodos directos / retransmitidos, latencia, pérdida de paquetes, tráfico) se lee mediante `easytier-cli`; `easytier-core` y `easytier-cli` se instalan juntos;
- **Comprobación de conflictos**: las instancias habilitadas a la vez no pueden escuchar en el mismo puerto (TCP y UDP se juzgan por separado), enlazar el mismo puerto de reenvío local ni compartir el nombre de interfaz al usar TUN; los nombres de instancia no pueden repetirse. Las comprobaciones se hacen al guardar, habilitar y copiar;
- Modificar una instancia reinicia solo esa instancia; cambiar únicamente el grupo o la nota no la reinicia;
- **OpenWrt / iStoreOS / ImmortalWrt**: la interfaz TUN de EasyTier (nombre predeterminado `tun0`, se puede fijar con el nombre de interfaz) no pertenece a ninguna zona del cortafuegos y la política input de fw4 descarta por defecto, así que los demás nodos no pueden acceder a la IP virtual de este equipo (este equipo sí puede acceder a los demás). Añada la interfaz a la zona lan: `uci add_list firewall.@zone[0].device='tun0'`, `uci commit firewall`, `/etc/init.d/firewall reload` (`@zone[0]` suele ser lan, ajústelo a la zona real; si cambió el nombre de la interfaz, use el nuevo). Verificado en iStoreOS 24.10 e ImmortalWrt 25.12: tras añadirla, los tres nodos se comunican entre sí. Además, el firmware reducido como ImmortalWrt no trae `/dev/net/tun` por defecto; instale antes `kmod-tun` (`apk add kmod-tun` o `opkg install kmod-tun`), de lo contrario `easytier-core` no arranca y se reinicia repetidamente, con el motivo visible en la instancia; también puede usar la «red en espacio de usuario»;
- **Windows**: la instalación en línea instala `easytier-core.exe` y `easytier-cli.exe` junto con las dependencias del mismo directorio, `wintun.dll`, `Packet.dll` y `WinDivert64.sys` (sin estas DLL el programa no arranca); para usar una interfaz TUN hay que ejecutar Gately como administrador (SYSTEM cuando se ejecuta como servicio);
- El tamaño del programa depende de la plataforma: en Linux x86_64 `easytier-core` ocupa unos 7,5 MB y `easytier-cli` unos 3 MB; en macOS arm64, unos 21 MB y 9 MB respectivamente; el paquete de versión completo para Windows x86_64 (con las bibliotecas) ocupa unos 32 MB.

En redes donde el hole punching UDP es difícil (un tipo de NAT estricto o una alta pérdida de paquetes UDP hacia el nodo de retransmisión), compara los resultados de conexión directa de EasyTier y Tailscale; los nodos de retransmisión de EasyTier pueden elegirse libremente, incluidos nodos de China continental.

## frp

- **Cliente frpc**: puedes añadir varias instancias de frpc; cada una se conecta a un frps y se ejecuta, activa, detiene y reinicia por separado. Los proxies pertenecen a una instancia y admiten tcp / udp / http / https / stcp / xtcp; al cambiar un proxy solo se reinicia su instancia;
- **Dirección de acceso de los proxies**: frpc no conoce los puertos HTTP / HTTPS ni el dominio raíz de subdominios de frps; indíquelos en la **Dirección de acceso** de la instancia frpc (con un clic se rellenan desde una instancia frps local; si hay un proxy inverso o una CDN delante de frps, use el esquema y el puerto que usan los visitantes). Así los dominios de los proxies http / https, incluido el nombre completo formado con un subdominio, se abren con un clic y se puede copiar su URL completa; sin ella siguen como texto. La dirección de acceso solo sirve para los enlaces y cambiarla no reinicia frpc. Los dominios personalizados se guardan en minúsculas y punycode como en Servicios web, y se rechazan los valores con puerto o ruta; un subdominio debe ser un solo nivel, como `nas`, tal como exige frps;
- **Servidor frps**: puedes añadir varias instancias de frps en puertos distintos, cada una con lista de puertos permitidos, subdominios y panel de control;
- **Actualizar desde versiones anteriores**: los ajustes únicos de frpc / frps de la 0.5.9 y anteriores se migran automáticamente a instancias llamadas `frpc` / `frps`; los proxies existentes pertenecen a esa instancia de frpc y las rutas de los programas pasan a la tarjeta «Programas» al final de la página;
- **Direcciones de escucha de frps**: «Dirección de escucha» indica dónde escucha el puerto de conexión de frpc y «Dirección de escucha de proxies», dónde escuchan los puertos HTTP / HTTPS y los puertos remotos de los proxies tcp / udp; vacías escuchan en todas las direcciones. Para poner frps detrás de un proxy inverso como Gately, usa `127.0.0.1` como dirección de escucha, crea en Servicio web un sitio que haga proxy inverso a `127.0.0.1:<puerto de enlace>` y elige websocket o wss como protocolo de transporte de frpc apuntando a ese sitio; deja vacía la dirección de escucha de proxies para que los puertos remotos de los proxies tcp sigan accesibles desde fuera. Al importar una configuración de frps sin `proxyBindAddr`, se usa `bindAddr`, igual que hace frps;
- **Control de acceso por IP de origen en frps**: cada instancia de frps puede usar una lista de permitidos o de bloqueados para decidir por IP de origen si frpc puede iniciar sesión (conectarse al puerto de enlace). La lista combina IP / subredes escritas a mano con listas de IP suscritas del Servicio web; coincidir con cualquiera cuenta como estar en la lista. El módulo Servicio web descarga las listas suscritas periódicamente y se aplican en cuanto se actualizan, sin reiniciar frps; hasta la primera descarga correcta cuentan como vacías. La comprobación se hace antes de que frps verifique el token: Gately abre un punto de comprobación en la dirección de loopback y añade `[[httpPlugins]]` (`ops = ["Login"]`) al final de la configuración de frps generada, así que solo afecta al inicio de sesión de frpc; los puertos remotos de los proxies tcp / udp y los puertos HTTP / HTTPS siguen abiertos como antes. Los bloqueos de la Prevención de intrusiones tienen prioridad: una dirección bloqueada no puede iniciar sesión aunque esté en una subred permitida, y las instancias sin control de acceso también respetan la lista de bloqueos, esté o no activada la sincronización con el cortafuegos. Un frpc rechazado recibe un error como `gately: client address ... is not in the allowlist`, y la tarjeta de la instancia muestra cuántos inicios de sesión se rechazaron desde que arrancó Gately. La lista se comprueba cuando frpc inicia sesión y cubre las conexiones de frpc al puerto de enlace de frps: si se restringe la lista o se bloquea una dirección, un frpc que ya inició sesión sigue conectado y se rechaza en su próximo inicio de sesión; para que se aplique de inmediato, reinicia esa instancia de frps. Las conexiones visitor de stcp / xtcp no pasan por la comprobación de inicio de sesión; para limitarlas, combínalo con la sincronización con el cortafuegos de [Prevención de intrusiones](/es/docs/modules/guard). Con frps detrás de un proxy inverso (dirección de escucha `127.0.0.1`), el origen que ve frps es la dirección del proxy; en ese caso haz el control de acceso por IP en el sitio del Servicio web; detrás de Enrutamiento por protocolo o Reenvío de puertos, frps ve una dirección local y la lista no puede distinguir a los visitantes, así que configura el control de acceso por IP de origen en la regla de enrutamiento o de reenvío;
- **Importar configuración existente**: «Importar configuración», en la cabecera de la página, admite `frpc.ini` / `frps.ini` antiguos (con la sección `[common]`) y `toml` de frp 0.52+. Puedes elegir un archivo o pegar el contenido, e importarlo en una instancia existente o en una nueva. Primero se muestra una vista previa del resultado y de los campos ignorados, y luego se confirma la importación. Al importar en una instancia de frpc existente puedes añadir (los proxies con el mismo nombre no cambian) o reemplazar todos los proxies de esa instancia. Las instancias existentes conservan su estado y las nuevas se crean desactivadas. También se puede llamar con la API `POST /api/frp/import`. Al migrar desde Lucky, cada instancia de frpc / frps de Lucky se importa como una instancia de Gately.

## Cloudflare Tunnel

- Túneles con token y túneles temporales de trycloudflare;
- Varios túneles con reinicio automático;
- Muestra la dirección pública.

### Rutas {#cloudflared-ingress}

Los nombres de host públicos y los servicios de origen (rutas) de un túnel con token se guardan en Cloudflare, y cloudflared en ejecución los obtiene automáticamente. Si indicas el **token de API** opcional en la configuración del túnel, el botón Rutas de la tarjeta permite editarlas directamente:

- El token necesita el permiso Cloudflare Tunnel: Edit y, para crear registros DNS automáticamente, DNS: Edit en las zonas correspondientes. La cuenta y el ID del túnel se leen del token del túnel.
- Cada regla tiene un nombre de host, una expresión regular de ruta opcional y un servicio (como `http://localhost:8080`, `ssh://localhost:22` o `http_status:404`). Las reglas se comparan en orden y gana la primera que coincide. Puedes activar Omitir la verificación TLS del origen y definir la cabecera Host hacia el origen; los demás ajustes de origen se conservan. El servicio por defecto atiende las solicitudes que no coinciden con ninguna regla; por defecto es `http_status:404`.
- Los cambios se aplican al guardar, sin reiniciar el túnel. Si mientras tanto las rutas se modificaron en el panel de Cloudflare u otro lugar, al guardar se avisa y se muestra Recargar para conservar esos cambios. Los servicios HTTP no pueden llevar ruta: una `/` final suelta (como `http://nas:5000/`) se elimina al guardar, porque cloudflared rechazaría toda la configuración.
- Si el túnel se gestiona con un archivo de configuración local (el diálogo indica que las rutas remotas no se aplican), al guardar se pide una confirmación más: tras escribir la configuración remota, el túnel pasa a gestión remota y las rutas del archivo de configuración local dejan de aplicarse.
- Con Sincronizar DNS activado, cada nombre de host recibe un registro CNAME con proxy que apunta a `<ID del túnel>.cfargotunnel.com`; los nombres de host que ya tienen otros registros, o un CNAME al túnel sin proxy (el tráfico entonces no entra en el túnel), los conservan y aparecen en el resultado. Las zonas se buscan solo en la cuenta del túnel. Al eliminar una regla, su registro DNS se mantiene; bórralo en Cloudflare si hace falta.

## Cómo elegir

| Escenario | Recomendación |
| --- | --- |
| Montar una red privada entre varios dispositivos | Tailscale |
| Una red privada en la que el cliente y el servidor de gestión son de código abierto y autoalojables | NetBird |
| Una red en la que el hole punching UDP es difícil o en la que quieres elegir tus propios nodos de retransmisión | EasyTier |
| Tienes un servidor público y quieres publicar a través de él los servicios internos | frp |
| Sin IP pública ni servidor, y quieres exponer un servicio web rápidamente | Cloudflare Tunnel |
| Tu operador ofrece un NAT que permite la perforación | [Traversal STUN](/es/docs/modules/stun) |
