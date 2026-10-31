# Enrutamiento por protocolo

Varios servicios comparten un mismo puerto público (por ejemplo, un sitio web, una VPN, SSH y Escritorio remoto en el 443). Gately lee los primeros datos que envía el cliente para identificar el protocolo, después examina el dominio (SNI) y el ALPN del handshake TLS o el Host de HTTP, y pasa la conexión completa al backend correspondiente. Aquí no se descifra ni se termina nada, de modo que los certificados TLS permanecen en los backends. Hace el trabajo de las reglas `req_ssl_sni` de haproxy más sslh.

El enrutamiento por dominio (enrutamiento SNI) es uno de sus casos: añade una ruta TLS y rellena sus dominios.

Cada regla tiene:

- Escucha: el protocolo (solo TCP; tcp es doble pila, tcp4 / tcp6 una sola familia de direcciones), la dirección de escucha y los puertos (`443`, `443,8443` o un rango);
- Rutas: cada ruta tiene un protocolo, un nombre, direcciones de destino (si hay varias, se prueban por turno, con conmutación por error), un puerto de destino opcional (vacío significa el puerto de escucha) y un ajuste de protocolo PROXY (desactivado, v1 o v2). El ajuste PROXY pertenece a la ruta porque algunos backends lo exigen y otros lo rechazan;
- Un destino predeterminado: las direcciones de destino, los puertos de destino y el ajuste de protocolo PROXY propios de la regla, que se usa cuando ninguna ruta coincide o el protocolo no se reconoce;
- Un tiempo de detección: cuántos segundos se espera a que el cliente envíe datos reconocibles; 0 equivale al valor predeterminado de 5 segundos.

## Protocolos reconocidos

| Protocolo de la ruta | Se reconoce por | Coincidencia adicional |
| --- | --- | --- |
| TLS | El registro del handshake TLS (ClientHello) | Dominio (SNI), ALPN |
| HTTP | El método y la versión `HTTP/` de la línea de solicitud, incluidos CONNECT, los métodos WebDAV y HTTP/2 con conocimiento previo (h2c) | Dominio (cabecera Host) |
| SSH | La cadena de versión del cliente `SSH-` | — |
| RDP (Escritorio remoto) | La cabecera TPKT y la solicitud de conexión X.224 | — |
| SOCKS4 / SOCKS5 | Las solicitudes SOCKS4 y SOCKS4a y la negociación de métodos de SOCKS5 | — |
| OpenVPN (TCP) | El paquete de reinicio completo (hard reset) del cliente, incluidos tls-auth, tls-crypt y tls-crypt-v2 | — |
| XMPP | Una declaración XML o `<stream:stream` | — |
| MQTT | El paquete CONNECT (3.1, 3.1.1, 5.0) | — |
| PostgreSQL | El mensaje de inicio, SSLRequest y GSSENCRequest | — |
| Minecraft (Java Edition) | El paquete de handshake; el ping de la lista de servidores de la 1.6 y anteriores | Dirección del servidor |
| RTSP | La versión `RTSP/1.0` o `RTSP/2.0` en la línea de solicitud (cámaras IP, NVR) | — |
| SIP | La versión `SIP/2.0` en la línea de solicitud (TCP) | — |
| Redis | Comandos enviados como arrays RESP | — |
| AMQP | La cabecera de protocolo `AMQP` (0-9-1 y 1.0, RabbitMQ y otros) | — |
| SMB | Solicitudes de negociación SMB1, SMB2 y SMB3 dentro de un mensaje de sesión NetBIOS | — |
| Git | La primera pkt-line del protocolo `git://` (git daemon) | — |
| IRC | El comando CAP, PASS, NICK o USER que el cliente envía primero | — |
| ADB | El mensaje CNXN de Android Debug Bridge (adb connect) | — |
| BitTorrent | Un handshake entre pares sin cifrar | — |
| Tiempo de espera | El cliente no envía nada dentro del tiempo de detección | — |

## Coincidencia

Las rutas TLS, HTTP y Minecraft coinciden por dominio (Minecraft usa la dirección del servidor que escribió el cliente, tomada del handshake; se ignoran los datos que añaden Forge, BungeeCord o TCPShield): primero los dominios exactos (`vpn.example.com`), luego el comodín más largo (`*.example.com`, de cualquier profundidad), luego `*` (cualquier conexión que lleve un dominio) y, por último, las rutas sin dominio (todas las conexiones de ese protocolo, incluidas las que no llevan SNI ni Host). Los dominios no distinguen mayúsculas de minúsculas.

Las rutas TLS también pueden indicar valores ALPN: la ruta coincide solo cuando el cliente ofrece alguno de ellos. En un mismo dominio, las rutas con ALPN tienen prioridad, y si el ALPN no coincide la búsqueda continúa con dominios más amplios. Por ejemplo, envía `acme-tls/1` al programa que responde a la validación de certificados TLS-ALPN-01, o `postgresql` a las conexiones TLS directas de PostgreSQL 17. Dentro de un protocolo, un dominio (con un ALPN dado) solo puede pertenecer a una ruta; los demás protocolos admiten una ruta cada uno.

«Tiempo de espera» es para protocolos en los que el servidor envía primero, como VNC, MySQL, FTP, SMTP, POP3 e IMAP: sus clientes se conectan y esperan al servidor, así que Gately no recibe nada y no puede saber de cuál se trata. Por eso cada puerto puede enviar solo uno de ellos a la ruta Tiempo de espera, y esas conexiones se reenvían al agotarse el tiempo de detección. El tiempo de detección admite un decimal: los clientes TLS, SSH, HTTP y similares envían el primer paquete milisegundos después del handshake, así que con una ruta Tiempo de espera bastan 0,3–1 segundos, y eso es todo lo que esperan VNC y las conexiones similares.

Estas conexiones van al destino predeterminado: ninguna ruta coincide; los datos no pertenecen a ninguno de los protocolos anteriores (por ejemplo, protocolos sin firma como Shadowsocks, VMess o BitTorrent cifrado, y protocolos cuyo primer paquete es solo un campo de longitud y se confunde fácilmente con otros, como MongoDB o DNS over TCP); no se puede analizar el ClientHello; o el cliente no envía nada y no hay ruta Tiempo de espera. Sin destino predeterminado, estas conexiones se cierran y se cuentan como «cerradas». Un cliente que se desconecta antes de que termine la detección se descarta.

## Notas

- Solo TCP. El tráfico UDP, como HTTP/3 (QUIC) o WireGuard, queda fuera del enrutamiento; usa una regla aparte para él;
- Las variantes cifradas de SIP, Redis, AMQP, IRC y protocolos similares van sobre TLS; enrútalas con rutas TLS por dominio o ALPN;
- El ClientHello puede estar dividido en varios registros TLS y tener hasta 64 KB; las líneas de solicitud de HTTP, RTSP y SIP pueden ocupar hasta 8 KB, y en HTTP se leen hasta 32 KB de cabeceras de solicitud para encontrar el Host;
- Cada conexión se detecta una sola vez, así que las solicitudes posteriores en la misma conexión HTTP keep-alive van al backend elegido la primera vez;
- La tarjeta de la regla muestra cuántas conexiones ha recibido cada ruta y el destino predeterminado; las entradas de depuración del registro de ejecución anotan el protocolo, el SNI, el ALPN o el Host detectado en cada conexión;
- Los cambios en las rutas, el destino predeterminado y el tiempo de detección se aplican de inmediato sin detener la escucha, y las conexiones abiertas se mantienen.

## Ejemplo: sustituir haproxy

Una configuración de haproxy como esta:

```text
frontend https
    bind 192.0.2.10:443
    mode tcp
    tcp-request inspect-delay 5s
    tcp-request content accept if { req_ssl_hello_type 1 }
    use_backend ocserv if { req_ssl_sni -i tz.example.com }
    default_backend web
backend ocserv
    server ocserv 127.0.0.1:443 send-proxy-v2
backend web
    server web 127.0.0.1:8443 send-proxy-v2
```

se convierte en una única regla de Gately:

| Ajuste | Valor |
| --- | --- |
| Protocolo | TCP |
| Dirección / puerto de escucha | `192.0.2.10` / `443` |
| Ruta `ocserv` | protocolo TLS, dominio `tz.example.com`, destino `127.0.0.1`, puerto `443`, PROXY v2 |
| Destino predeterminado | `127.0.0.1`, puerto de destino `8443`, PROXY v2 |

Define la dirección de escucha como la dirección pública del servidor para evitar choques con los backends que escuchan en `127.0.0.1:443`. Para servir sitios web con el propio Gately, añade un listener de [Servicios web](/es/docs/modules/web) en `127.0.0.1:8443` con «Aceptar protocolo PROXY» activado y `127.0.0.0/8` en los orígenes PROXY de confianza. Detén haproxy antes de activar la regla, porque solo un programa puede escuchar en el puerto.

## Ejemplo: sustituir sslh

Estos argumentos de sslh:

```text
sslh --listen 0.0.0.0:443 --timeout 2 \
     --ssh 127.0.0.1:22 --openvpn 127.0.0.1:1194 --tls 127.0.0.1:8443
```

equivalen a una única regla de Gately:

| Ajuste | Valor |
| --- | --- |
| Protocolo | TCP |
| Dirección / puerto de escucha | vacío / `443` |
| Tiempo de detección | `2` |
| Ruta `ssh` | protocolo SSH, destino `127.0.0.1`, puerto `22` |
| Ruta `vpn` | protocolo OpenVPN, destino `127.0.0.1`, puerto `1194` |
| Ruta `web` | protocolo TLS, sin dominios, destino `127.0.0.1`, puerto `8443` |
| Ruta `vnc` (opcional) | protocolo Tiempo de espera, destino `127.0.0.1`, puerto `5900` |

Por defecto, sslh envía las conexiones a SSH al agotarse el tiempo de espera. Gately reconoce SSH al instante por su cadena de versión `SSH-`, así que el tiempo de espera puede llevar a un servicio en el que el servidor envía primero, como VNC. Puedes seguir añadiendo rutas RDP, SOCKS y HTTP, o repartir TLS entre varios backends por dominio.

## Actualizar desde el reenvío de puertos

El enrutamiento por protocolo era antes el modo «Enrutamiento SNI» de las reglas de reenvío de puertos. En el primer arranque tras la actualización, las reglas de enrutamiento SNI del reenvío de puertos pasan automáticamente al módulo Enrutamiento por protocolo: cada regla conserva su clave, su nombre, su estado de activación, su grupo y todas sus rutas, y las rutas antiguas se convierten en rutas TLS. Si el módulo de reenvío de puertos estaba desactivado, el enrutamiento por protocolo también queda desactivado, y los tokens de API con acceso al reenvío de puertos reciben el mismo nivel de acceso al enrutamiento por protocolo.

Tras la migración, gestiona estas reglas a través de `/api/protoroute/rules`: el campo de rutas cambia de nombre de `sniRoutes` a `routes` y el campo `mode` deja de usarse; las reglas de enrutamiento enviadas a `/api/portforward/rules` se rechazan. Las métricas pasan a ser `gately_protoroute_*`; consulta [Métricas de monitorización](/es/docs/reference/monitoring).

## Consejo de despliegue

El enrutamiento por protocolo escucha directamente en puertos del host; en Docker se recomienda `--network host`.

## Grupos y orden

Las reglas pueden tener un «Grupo». La agrupación, el orden y la búsqueda funcionan igual que en [Reenvío de puertos](/es/docs/modules/portforward#grupos-y-orden); el cuadro de búsqueda filtra por nombre, grupo, puerto de escucha, destino, dominio o protocolo de ruta.
