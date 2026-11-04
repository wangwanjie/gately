# Diagnóstico de red

Ejecuta Ping, traceroute, sondeo de puertos, consultas DNS, prueba de velocidad y escaneo de la red local desde el navegador. Ubicación en el menú: «Herramientas → Diagnóstico de red» en la barra lateral.

## Tareas en segundo plano

Cada vez que pulsas «Iniciar» se crea una tarea en segundo plano. La API devuelve de inmediato el ID de la tarea y la interfaz consulta periódicamente la salida.

- Se ejecutan como máximo 4 tareas a la vez; por encima de ese límite se muestra el mensaje «inténtalo de nuevo más tarde»;
- Una tarea se ejecuta como máximo 5 minutos y se cancela automáticamente al agotarse el tiempo;
- Se conservan las últimas 50 tareas y las tareas en ejecución nunca se limpian; cada tarea conserva como máximo 500 líneas de salida;
- Una tarea en ejecución se puede cancelar con «Detener». «Tareas recientes», en la parte inferior de la página, lista las tareas anteriores, y al pulsar una se muestra su salida;
- Estados de las tareas: en ejecución, terminada, fallida, cancelada.

## Ping

Envía solicitudes de eco ICMP al destino y muestra cada tiempo de ida y vuelta, el porcentaje de pérdida de paquetes y la latencia mínima / media / máxima.

| Parámetro | Descripción |
| --- | --- |
| Destino | Nombre de host o IP, con compatibilidad con IPv4 e IPv6 |
| Cantidad | Por defecto 4, hasta 20 |

Cada solicitud agota el tiempo a los 2 segundos, con un intervalo de unos 0,7 segundos. Primero se intenta un socket ICMP de tipo UDP sin privilegios y después un socket raw. Dentro de un contenedor Docker, esto requiere la capability `NET_RAW` o el ajuste `net.ipv4.ping_group_range`; si los permisos son insuficientes, usa el sondeo de puertos. Si el destino no responde en absoluto, la tarea termina como fallida.

## Traceroute

Llama a un comando del sistema para trazar la ruta y muestra el resultado original del comando.

- Windows usa `tracert`; los demás sistemas prefieren `traceroute` y después `tracepath`;
- Un máximo de 20 saltos, sin resolución inversa (las IP se muestran directamente);
- Si el sistema no tiene el comando correspondiente, la tarea informa de un error. En las imágenes de Docker puedes instalar el paquete `traceroute`.

## Sondeo de puertos

Realiza intentos de conexión TCP al destino y lista solo los puertos abiertos.

| Parámetro | Descripción |
| --- | --- |
| Destino | Nombre de host o IP |
| Puertos | Por ejemplo `22,80,443,8000-8100`; se admiten listas separadas por comas y rangos, con puertos de 1 a 65535 |

- Un máximo de 1024 puertos por sondeo;
- Concurrencia de 100, con un tiempo de espera de conexión de 1,5 segundos por puerto;
- Los puertos habituales muestran el nombre del servicio (ssh, http, https, mysql, redis, postgres, rdp, etc.) y el tiempo de conexión.

## Consulta DNS

| Parámetro | Descripción |
| --- | --- |
| Dominio / IP | El nombre que se consulta; introduce una IP para el tipo `PTR` |
| Tipo | `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `PTR`; por defecto `A` |
| Servidor DNS | Déjalo vacío para usar el resolutor del sistema; puedes introducir `223.5.5.5`, `223.5.5.5:5353` o una dirección DoH como `https://dns.alidns.com/dns-query` |

Las consultas agotan el tiempo a los 8 segundos y muestran los registros y el tiempo transcurrido.

## Prueba de velocidad

Una prueba de velocidad de descarga con 4 flujos simultáneos. Imprime la velocidad actual (Mbps) cada segundo y, al final, el total descargado y la velocidad media.

| Parámetro | Descripción |
| --- | --- |
| URL de descarga | Una URL `http(s)://` completa. Déjala vacía para usar el endpoint de prueba de velocidad de Cloudflare (`https://speed.cloudflare.com/__down?bytes=50000000`). La lista desplegable ofrece archivos de prueba públicos habituales de Cloudflare, Vultr (Tokio, Singapur, Los Ángeles), Hetzner (Singapur, Alemania) y OVH (Francia), y se puede escribir cualquier otra URL. Cuando un archivo termina de descargarse se vuelve a pedir hasta que acaba la duración |
| Duración | En segundos, de 2 a 30 en la interfaz, por defecto 8 |

Las peticiones de la prueba de velocidad no siguen redirecciones, por lo que no se pueden dirigir a direcciones cuyo sondeo está bloqueado.

## Escaneo de la red local

Escanea una subred en busca de dispositivos conectados y muestra los resultados en una tabla: IP, MAC, nombre de host, puertos abiertos.

| Parámetro | Descripción |
| --- | --- |
| Subred | CIDR, por ejemplo `192.168.1.0/24`. Déjala vacía para usar la subred de la primera interfaz IPv4 privada de esta máquina (el desplegable lista las subredes locales disponibles, y las subredes demasiado grandes se reducen a /24) |

Límites y funcionamiento:

- Solo se admiten subredes privadas IPv4 (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`);
- Un máximo de 1024 direcciones por escaneo, equivalente a una /22; las direcciones de red y de difusión se excluyen automáticamente;
- En cada dirección se sondean los puertos habituales 80, 443, 22, 445, 139, 3389, 8080, 62078, 5000 y 53, y una conexión que se establece o es rechazada cuenta como conectado;
- Las direcciones MAC proceden de la tabla de vecinos del sistema, y un host que tiene una entrada en esa tabla pero ningún puerto habitual abierto también se lista como conectado;
- Los nombres de host se resuelven con DNS inverso (tiempo de espera de 1,5 segundos);
- En los dispositivos con MAC, pulsa «Añadir a Wake-on-LAN» para añadirlos a la lista de dispositivos de [Wake-on-LAN](/es/docs/modules/tools).

## Lista de bloqueo de sondeos

«Lista de bloqueo de sondeos», arriba a la derecha de la página, define los destinos a los que no pueden acceder las herramientas de diagnóstico.

- Las direcciones de metadatos de los proveedores de nube siempre están bloqueadas: `169.254.169.254/32`, `100.100.100.200/32`, `fd00:ec2::254/128`, lo que evita que las herramientas se usen para robar credenciales de instancias;
- Puedes añadir IP o subredes, una por línea, como `10.0.0.0/8`;
- Si cualquiera de las direcciones a las que se resuelve el nombre de host de un destino coincide con la lista, la tarea falla con un error. También se comprueba la URL de descarga de la prueba de velocidad, y los escaneos omiten las direcciones que coinciden.

::: tip
Las herramientas de diagnóstico se ejecutan en el host o contenedor donde se ejecuta Gately, por lo que los resultados reflejan la red de ese entorno. Para limitar lo que se puede sondear, añade a la lista de bloqueo de sondeos las subredes internas sensibles.
:::

## Autodiagnóstico de red

El botón «Autodiagnóstico» de la página «Resumen» (también disponible en la guía del primer arranque) revisa de una vez los problemas de red y configuración más comunes. Los elementos con problemas aparecen primero, con una recomendación para resolverlos:

| Comprobación | Descripción |
| --- | --- |
| Dirección pública | Obtiene la IPv4 / IPv6 de salida mediante las direcciones de consulta de IP pública que usa DDNS |
| NAT | Si el equipo tiene directamente una IPv4 / IPv6 pública; si una interfaz tiene una dirección de 100.64.0.0/10, avisa de que puede estar detrás de un NAT de operador (se excluyen las interfaces de redes superpuestas como Tailscale) |
| Hora del sistema | Estima el desfase del reloj a partir de la hora de respuesta de los servicios de consulta; más de 60 segundos hace fallar los códigos de dos factores y la validación de certificados |
| Puertos 80 / 443 | Si están libres, ya ocupados o sin permiso para escuchar |
| Resolución DDNS | Si el dominio de cada registro con dirección dinámica resuelve ahora a la dirección de salida de este equipo. Primero se consulta el DNS local; si no devuelve la dirección de este equipo, se consulta también DNS público (223.5.5.5, 119.29.29.29, 1.1.1.1, 8.8.8.8) y la comprobación pasa si cualquiera la devuelve, para que un tiempo de espera ocasional o una caché antigua del DNS local no generen avisos falsos. Si ningún DNS responde, se indica una consulta fallida, distinta de un registro inexistente |
| Certificados | Certificados caducados o que caducan en los próximos 14 días |
| Certificados de sitios HTTPS | En los listeners HTTPS activados, qué dominios de sitio no tienen un certificado que los cubra (esos dominios fallan en el handshake TLS) |
| Conflictos de frp | Proxies de frpc con nombres repetidos, y conflictos de nombre de proxy, dominio o puerto remoto que frps haya notificado en los registros de los últimos 30 minutos (habitual cuando varios equipos copian la misma configuración de frpc) |
| Estado de los módulos | Módulos en estado de advertencia o error en el resumen |

El autodiagnóstico consulta los servicios de IP pública configurados, solo se ejecuta al pulsar el botón y no está disponible para cuentas de solo lectura ni tokens de API.
