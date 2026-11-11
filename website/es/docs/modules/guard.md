# Prevención de intrusiones

Prevención de intrusiones al estilo de fail2ban: Gately detecta ataques de fuerza bruta en los registros, coloca trampas en puertos sin usar, cuenta los escaneos de puertos, añade las IP maliciosas a una lista de bloqueos y la sincroniza con el cortafuegos del sistema (nftables / iptables + ipset), que descarta en el kernel todo su tráfico.

## Funciones

- **Reglas de registros**: leen archivos de registro, el diario de systemd, la salida de un comando (como `logread -f` en OpenWrt) o los registros de un contenedor Docker, buscan fallos con expresiones regulares y bloquean la IP que alcanza el número de fallos en la ventana de tiempo. Incluye plantillas para SSH, Dropbear, Nginx, Vaultwarden, Home Assistant, Proxmox VE y otros, y en el editor puedes pegar líneas de ejemplo para probar los filtros;
- **Eventos de Gately**: los inicios de sesión fallidos en el panel y los bloqueos del WAF y las llamadas rechazadas de [Port Knocking](/es/docs/modules/knock) también pueden alimentar una regla, sin expresiones regulares;
- **Sondeo de rutas web**: cuenta directamente las solicitudes que atiende el servicio web por escucha, ruta y código de estado, sin leer registros; se bloquean los orígenes que piden rutas señuelo como `/.env` o `/.git`, o que escanean una y otra vez dominios desconocidos;
- **Puertos trampa**: trampas en puertos que este equipo no usa (por defecto 23, 135, 1433, 3389, 5900, 6379 y otros). Cualquier origen que se conecte queda bloqueado;
- **Detección de escaneos**: se bloquea el origen que se conecta a puertos cerrados de este equipo más veces por minuto que el umbral;
- **Una sola lista de bloqueos**: aquí se gestionan los bloqueos manuales (incluidas redes CIDR y bloqueos permanentes) y los bloqueos automáticos por fallos de autenticación del servicio web. Los bloqueos manuales no pueden incluir direcciones de loopback, porque todas las conexiones reenviadas a través de esta máquina aparecen con esa dirección. Los bloqueos se guardan y se restauran tras reiniciar;
- **Bloqueos progresivos**: si una dirección vuelve a bloquearse en 30 días, la duración se duplica hasta un máximo;
- **Lista blanca**: por defecto se ignoran las direcciones de la LAN, de loopback, de 100.64.0.0/10 (Tailscale) y las redes IPv6 /64 de las interfaces de este equipo (los bloqueos son por /64, así que los dispositivos con IPv6 pública de la misma LAN no caen junto con el infractor), y puedes añadir IP o redes. Una sola dirección IPv6 de la lista blanca protege todo su /64.

## Dónde se aplican los bloqueos

| Entorno | Efecto de un bloqueo | Puertos trampa | Detección de escaneos |
| --- | --- | --- | --- |
| Linux (root o `CAP_NET_ADMIN`, nftables instalado) | Se descarta en el kernel todo el tráfico, incluido SSH, otros programas y el tráfico reenviado a contenedores Docker y dispositivos de la LAN | Los gestiona el kernel y detectan escaneos que solo envían SYN | Disponible |
| Docker (red host + `cap_add: NET_ADMIN`) | Igual que arriba; las reglas se escriben en el anfitrión | Igual | Disponible |
| Paquete de fnOS, OpenWrt 22.03 y posteriores | Igual que arriba (se ejecuta como root y nftables viene incluido) | Igual | Disponible |
| Synology DSM y otros Linux cuyo kernel no tiene nf_tables (Docker con red host + `NET_ADMIN`; una instalación directa necesita ipset e iptables legacy) | Igual que arriba, con la tabla mangle de iptables e ipset | Igual que arriba | Disponible (umbral máximo de 255 por minuto) |
| Resto de casos (Windows, macOS, FreeBSD, Linux sin nftables ni iptables (legacy) con ipset, o sin permisos) | Solo los servicios de Gately: reenvío de puertos, servicio web, STUN, WebDAV, FTP, SMB, el inicio de sesión de frps y el panel | Escucha Gately; solo detecta conexiones que completan el saludo TCP | No disponible |

La parte superior de la página muestra el estado de la sincronización con el cortafuegos y, si no está disponible, el motivo y cómo solucionarlo.

## Reglas del cortafuegos

Con «Sincronizar con el cortafuegos» activado, Gately crea su propia tabla `inet gately` en nftables y no toca las reglas del sistema (fw4, Docker, ufw, etc.):

- La cadena `prerouting` (prioridad -190, después del seguimiento de conexiones y antes del NAT de destino) descarta los paquetes que llegan de direcciones bloqueadas, tanto los dirigidos a este equipo como los reenviados a contenedores o dispositivos de la LAN. Las respuestas a las conexiones que abre este equipo (DNS, repositorios, etc.) siguen pasando aunque el otro extremo esté bloqueado. En IPv6 se bloquea por /64;
- La cadena `input` solo mira las conexiones nuevas a este equipo, para los puertos trampa y la detección de escaneos. Las direcciones de la lista blanca no participan;
- La tabla completa se elimina al desactivar la sincronización o al detener Gately. Si otro programa la borra, Gately la vuelve a escribir en menos de 10 segundos.

Si el kernel no tiene nf_tables (por ejemplo, Synology DSM), se usan iptables (legacy) e ipset, también sin tocar las reglas existentes: la primera regla de `PREROUTING` e `INPUT` en la tabla mangle salta a las cadenas propias de Gately `GATELY_PRE` y `GATELY_IN`, que hacen lo mismo que las dos cadenas anteriores; las direcciones individuales bloqueadas van a los conjuntos de ipset `gately_ban4` / `gately_ban6` (IPv6 por /64), y los bloqueos de redes y permanentes son reglas de la cadena `GATELY_DENY`. Si un programa como el cortafuegos de Synology pone sus reglas delante, los saltos vuelven al principio en menos de 10 segundos, y las cadenas borradas se escriben de nuevo. La detección de escaneo cuenta con el módulo recent, con un umbral máximo de 255 por minuto.

«Ver reglas» en la página muestra todas las reglas vigentes.

::: tip Docker
Usa `network_mode: host` y concede `NET_ADMIN` para que las reglas se escriban en el anfitrión:

```yaml
    network_mode: host
    cap_add:
      - NET_ADMIN
```

Sin ello, los bloqueos siguen aplicándose a los servicios de Gately. Para leer registros del anfitrión, monta el directorio de registros en el contenedor, por ejemplo `- /var/log:/var/log/host:ro`, e indica `/var/log/host/auth.log` en la regla. El origen «diario de systemd» no está disponible dentro de un contenedor.
:::

## Reglas de registros

| Origen | Qué indicar | Notas |
| --- | --- | --- |
| Archivo de registro | Rutas de archivo, una por línea | Lee las líneas nuevas desde el final del archivo y sigue la rotación (renombrado o truncado) |
| Diario de systemd | Unidades de servicio como `ssh.service` | Se lee con `journalctl -f` |
| Salida de un comando | Un comando como `logread -f` | Se reinicia si termina |
| Contenedor Docker | Nombre del contenedor | Se lee a través de la conexión a Docker del módulo Docker |
| Evento de Gately | Inicio de sesión fallido / bloqueo del WAF / llamada de Port Knocking rechazada | Los eventos incluyen la IP de origen; no hace falta filtro |
| Sondeo de rutas web | Escuchas (opcional) y reglas de ruta | Cuenta las solicitudes del servicio web, ver más abajo |

Los filtros son expresiones regulares de Go (sintaxis RE2), una por línea, y `<HOST>` representa la dirección de origen, por ejemplo:

```
Failed password for (?:invalid user )?\S* ?from <HOST>
```

Las líneas que coinciden con «Excluir» no se cuentan. La ventana de tiempo, el número de fallos y la duración del bloqueo se configuran por regla; la duración puede ser la predeterminada, personalizada o permanente.

## Sondeo de rutas web

Con el origen «Sondeo de rutas web», la regla cuenta directamente las solicitudes que atiende el servicio web, sin leer registros:

- **Escuchas**: elija una o varias; vacío para todas;
- **Rutas**: una por línea; vacío para todas. `*` coincide con cualquier carácter (incluido `/`), se compara la ruta completa sin distinguir mayúsculas. Por ejemplo, `*/.env` coincide con `/.env` y `/api/.env`. Las líneas que empiezan por `~` son expresiones regulares de Go. Variantes como `//.env` o `/a/../.env` también coinciden mediante la ruta normalizada;
- **Rutas excluidas**: misma sintaxis; las solicitudes que coincidan no se cuentan;
- **Qué se cuenta**: por defecto solo 404 y 444 (contenido inexistente, o conexión cerrada por un sitio «Cerrar conexión»); las peticiones a páginas que existen no cuentan. Con «Cualquier código de estado» cuenta toda respuesta, lo que conviene a las rutas señuelo porque las aplicaciones de una sola página devuelven 200 para cualquier ruta. «Cualquier código de estado» requiere reglas de ruta o «Solo dominios desconocidos»; si no, se rechaza al guardar;
- **Denegaciones del control de acceso**: si un sitio tiene Al denegar en Cerrar conexión, las peticiones rechazadas por un bloqueo, el control de acceso por IP o el filtro de países cuentan como 403 en los sitios normales, para no bloquear a visitantes legítimos fuera de la lista, y como 444 en el sitio predeterminado, igual que un sitio predeterminado «Cerrar conexión»; si el sitio predeterminado usa un proxy inverso con lista de permitidos para tus IP, los escaneos de dominios desconocidos se siguen contando;
- **Solo dominios desconocidos**: cuenta solo las solicitudes cuyo dominio no coincide con ningún sitio, es decir, las atendidas por el sitio predeterminado o, si no lo hay, respondidas con 404 por la escucha.

Se incluyen tres plantillas:

| Plantilla | Condición | Notas |
| --- | --- | --- |
| Rutas señuelo web | `/.env`, `/.git`, `wp-login.php`, `phpinfo.php`, `phpmyadmin` y otras, cualquier código de estado, 1 vez | Bloquea 1 día. Si un sitio usa realmente WordPress, phpMyAdmin u otros similares, elimine antes las líneas correspondientes |
| Escaneo de dominios desconocidos | Solo dominios desconocidos, 5 veces en 1 minuto | Bloquea 1 día. Combina con un sitio predeterminado «Cerrar conexión»: el escáner queda bloqueado tras unos pocos 444 y, con la sincronización del cortafuegos, las conexiones posteriores se descartan |
| Sondeo web 404 | Todas las rutas, 10 veces en 1 minuto | Se excluyen las rutas que los navegadores piden por sí solos, como favicon, robots.txt y archivos `.map`. También cuentan los 404 que devuelven los backends del proxy: miniaturas de galería ausentes o una aplicación que consulta un endpoint inexistente pueden activarla; revise el registro de acceso antes de activarla y suba el número o añada rutas excluidas si hace falta |

«Bloqueo de sondeos» en la tarjeta de una escucha del servicio web abre esta página con esa escucha elegida y la plantilla de señuelos aplicada; la regla se guarda y se aplica en la protección contra intrusiones. Al editar una regla puede pegar rutas de solicitud para probar las reglas de ruta.

Cuando un sitio activa «Confiar en usuarios autenticados» (consulte [Servicio web](./web)), las solicitudes de visitantes autenticados por la autenticación de Gately de ese sitio no cuentan en el sondeo de rutas. Las solicitudes a dominios desconocidos que atiende el sitio predeterminado y los visitantes sin sesión se siguen contando; las demás reglas, como escaneos de puertos, puertos señuelo e inicios de sesión fallidos, no se ven afectadas.

Cuando un sitio externo de un Gately en dos niveles activa «Confiar en la marca de autenticación del backend», las solicitudes de usuarios que la instancia interna confirma como autenticados tampoco cuentan en el sondeo de rutas de la instancia externa.

::: warning Alcance del bloqueo
Sea cual sea la escucha que lo active, el bloqueo entra en la misma lista y se aplica a todas las escuchas, sitios y demás servicios de Gately; con la sincronización del cortafuegos activa, el cortafuegos del sistema también descarta todo el tráfico de esa dirección.
:::

La dirección de origen es la misma del registro de acceso y solo se restaura mediante el protocolo PROXY. En sitios detrás de una CDN que pasa la IP real en una cabecera, se bloquearía el nodo de la CDN, así que no use el sondeo de rutas en esas escuchas. Si Gately está detrás de otro proxy inverso en el mismo equipo, el origen es una dirección de bucle local: con «Ignorar direcciones de la LAN» nunca se bloquea, y la regla no tiene efecto.

## Compromisos de la detección de escaneos

La detección cuenta las conexiones nuevas a puertos **en los que este equipo no escucha**. Los puertos TCP en escucha se actualizan cada 10 segundos y se consideran abiertos; los puertos que otras reglas del cortafuegos reenvían no pasan por la cadena `input` y no se cuentan. El umbral predeterminado es 12 por minuto. Un cliente normal reintenta un puerto cerrado unas 5 veces por minuto, así que no conviene bajar de 10.

Los puertos trampa que están en uso se excluyen automáticamente para no bloquear a visitantes legítimos.

## Modo estricto IPv6

IPv6 no tiene NAT, cada dispositivo tiene una dirección pública y, cuando esa dirección aparece en el DNS o en los registros de transparencia de certificados, los escáneres pueden conectarse a ella directamente. La opción «Modo estricto IPv6» de la detección de escaneo de puertos pasa IPv6 a una lista de permitidos: los orígenes IPv6 solo pueden conectarse a puertos abiertos (puertos en escucha más «Puertos abiertos adicionales») y la primera conexión a cualquier otro puerto provoca el bloqueo, sin contar umbral. IPv4 sigue usando el umbral.

- El bloqueo se aplica a todo el /64, así que los demás dispositivos del mismo /64 también quedan bloqueados. Las direcciones de la lista blanca y las cubiertas por «Ignorar direcciones privadas» no se ven afectadas.
- Un puerto que empieza a escuchar se considera abierto en unos 10 segundos, y el modo surte efecto unos 15 segundos después de activarlo o de reiniciar; hasta entonces se aplica el umbral. El umbral también se aplica cuando no se pueden leer los puertos en escucha.
- Un puerto que deja de escuchar sigue contando como abierto durante 30 segundos, para que no se bloquee a los visitantes que reintentan mientras frps se reconecta o se reinicia un programa o una escucha. Si el servicio FTP tiene un rango de puertos pasivos, todo el rango cuenta como abierto; si el sistema asigna los puertos pasivos al azar, no se pueden listar de antemano, las conexiones de datos IPv6 se bloquearían como escaneos y el registro de ejecución lo avisa: configure un rango de puertos pasivos en el servicio FTP.
- Los puertos que otros programas usan mediante reglas del cortafuegos deben añadirse antes a «Puertos abiertos adicionales»; de lo contrario, los orígenes que se conecten a ellos serán bloqueados.
- Solo se cuentan las conexiones TCP nuevas. La sincronización con el cortafuegos debe estar activa y la detección de escaneo de puertos habilitada.

## Relación con el servicio web

Los bloqueos de «Bloquear tras fallos de autenticación» del servicio web van a la misma lista, por lo que con la sincronización activada afectan a todo el equipo y no solo a los sitios web. Con «Ignorar direcciones de la LAN» activado, las direcciones de la LAN nunca se bloquean automáticamente.

El servicio web y el WAF solo conocen la dirección real del cliente mediante el protocolo PROXY. En sitios detrás de una CDN que pasa la IP real en una cabecera, los bloqueos por fallos de autenticación y la regla «Bloqueos del WAF» bloquearían los nodos de la CDN, así que no los actives en esos sitios.

## Notificaciones

Los bloqueos generan el evento `Prevención de intrusiones: IP bloqueada`, al que puedes suscribirte en el [Centro de notificaciones](/es/docs/modules/notify). Las direcciones que los puertos trampa y la detección de escaneos bloquean en una misma comprobación se agrupan en una sola notificación.

## Verificación

Verificado en espacios de nombres de red aislados en fnOS (Debian 12, nftables 1.0.6) e iStoreOS 24.10 (nftables 1.1.1): los puertos trampa y la detección de escaneos (IPv4 e IPv6) bloquean en el kernel y se importan a la lista de bloqueos; los archivos de registro (con rotación), el diario de systemd, los registros de contenedores Docker y los inicios de sesión fallidos en el panel activan bloqueos; los bloqueos manuales de redes y permanentes, el desbloqueo, las duraciones progresivas, la restauración tras reiniciar, la reescritura tras borrar la tabla desde fuera y la limpieza al detener funcionan como se espera; un contenedor Docker sin `NET_ADMIN` se detecta como sin permisos y pasa a usar puertos trampa en espacio de usuario.
