# Tailscale / NetBird / frp / cloudflared

Estos cuatro módulos integran los programas oficiales como procesos gestionados: reinicio automático, registros recogidos en la interfaz y configuración desde la propia interfaz. Gately no incluye estos programas; se pueden instalar, actualizar y revertir en línea desde la página.

## Instalación, actualización y reversión en línea {#install}

Las páginas de frp, Cloudflare Tunnel, Tailscale, NetBird y [File Browser](/es/docs/modules/tools#filebrowser) tienen al final una tarjeta «Instalación en línea»:

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
| FileBrowser Quantum | GitHub gtsteffaniak/filebrowser | `filebrowser` |

Tailscale no publica paquetes listos para ejecutar en macOS ni en Windows: usa el instalador oficial e indica la ruta en los ajustes. La tarjeta avisa cuando la plataforma actual no está soportada.

## Restaurar valores predeterminados {#reset}

El botón «Restaurar valores predeterminados» de la cabecera es independiente de la desinstalación: detiene todos los procesos del módulo, devuelve los ajustes (incluidos la ruta del programa y el proxy de descarga) a sus valores predeterminados y elimina los archivos que Gately generó para él. Los programas instalados se conservan.

| Módulo | Qué se elimina |
| --- | --- |
| frp | Todas las instancias de frpc / frps, los proxies y los ajustes de los programas, y los archivos de configuración generados en `frp/` |
| Cloudflare Tunnel | Todos los túneles y los ajustes del programa |
| Tailscale | Todos los ajustes y el estado de sesión del directorio de estado predeterminado; si el tailscaled gestionado por Gately está en ejecución, primero cierra la sesión. Los directorios de estado personalizados y el tailscaled existente del sistema quedan intactos |
| NetBird | Todos los ajustes y el estado de sesión y las claves del directorio de estado predeterminado; si el demonio gestionado por Gately está en ejecución, primero cierra la sesión y esta máquina se elimina de la red. Los directorios de estado personalizados y el servicio NetBird existente del sistema no se tocan |
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

- Gestiona `tailscaled` (red en espacio de usuario, apta para contenedores) o se conecta a un demonio del sistema existente;
- Clave de autenticación o enlace de inicio de sesión, con compatibilidad con Headscale;
- Rutas de subred y nodos de salida;
- Lista de nodos y proxy SOCKS5;
- Puerto UDP de WireGuard fijo, que facilita permitir conexiones directas a través de un cortafuegos;
- Argumentos adicionales y variables de entorno de tailscaled. Si el iptables del host usa el backend nf_tables, añade `TS_DEBUG_FIREWALL_MODE=nftables` a las variables de entorno; de lo contrario, el reenvío de subredes no funcionará.

## NetBird

- Unirse a una red NetBird en NetBird Cloud o en tu propio servidor de gestión (indicando su URL);
- Autenticación con clave de configuración o enlace de inicio de sesión. En Linux el enlace usa inicio de sesión con código de dispositivo y se puede abrir en cualquier dispositivo; en macOS y Windows el inicio de sesión normalmente debe completarse en un navegador de la máquina donde corre Gately, así que usa una clave de configuración para gestionarla en remoto. El enlace se renueva automáticamente al caducar;
- Demonio `netbird` gestionado (red en espacio de usuario, apta para contenedores) o conexión al servicio NetBird ya instalado en el sistema;
- Con red en espacio de usuario, por defecto otros pares pueden llegar a los servicios de esta máquina (incluidos los que solo escuchan en 127.0.0.1; las políticas de acceso del servidor de gestión siguen aplicándose), y los programas locales llegan a los dispositivos de la red mediante un proxy SOCKS5 (por defecto `127.0.0.1:1080`);
- Las rutas de red, los nodos de salida y las políticas de acceso se configuran en el panel de NetBird; en Gately eliges si aceptar rutas, actuar como par de enrutamiento, aceptar DNS y bloquear conexiones entrantes;
- Lista de pares con conexión directa o por relé, latencia y rutas de red;
- Puerto WireGuard fijo, argumentos adicionales del demonio y variables de entorno (las opciones avanzadas `NB_` de la documentación de NetBird);
- Con la red en espacio de usuario desactivada se usa WireGuard del kernel: necesita root y un dispositivo TUN, crea la interfaz `wt0` y añade sus propias reglas de cortafuegos (en OpenWrt también permite el tráfico entrante de `wt0` en fw4), y los servicios locales deben escuchar en todas las direcciones o en la dirección de NetBird para que otros pares lleguen a ellos;
- El programa `netbird` ocupa unos 41 MB y usa unos 60 MB de memoria, y la instalación en línea guarda hasta 3 versiones antiguas en `bin/.versions/`. En routers con poca flash vigila el espacio libre: elimina versiones antiguas en la tarjeta de instalación o coloca el programa en un almacenamiento externo e indica su ruta en los ajustes;
- Restaurar valores predeterminados y `POST /api/netbird/logout` dan de baja esta máquina en el servidor de gestión y la eliminan de la red; para volver a unirse hay que autenticarse de nuevo.

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
| Tienes un servidor público y quieres publicar a través de él los servicios internos | frp |
| Sin IP pública ni servidor, y quieres exponer un servicio web rápidamente | Cloudflare Tunnel |
| Tu operador ofrece un NAT que permite la perforación | [Traversal STUN](/es/docs/modules/stun) |
