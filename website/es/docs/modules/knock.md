# Port Knocking

Autorización de paquete único (SPA, Single Packet Authorization) compatible con [fwknop](https://www.cipherdyne.org/fwknop/): los puertos protegidos no aceptan conexiones desde el exterior en condiciones normales. El cliente envía primero un paquete de llamada cifrado, autenticado con HMAC y protegido contra repetición; cuando Gately lo autentica, abre el acceso solo a esa dirección de origen durante un tiempo limitado. Se usa sobre todo para proteger SSH directo: lo que el terminal web no puede hacer (scp, desarrollo remoto con IDE, reenvío de puertos) también se conecta con seguridad desde Internet, y los escáneres y los ataques de fuerza bruta no ven el puerto.

## Capacidades

- **Compatible con el cliente fwknop**: usa directamente el cliente oficial de fwknop (Linux, macOS, WSL); el formato de las claves es el mismo `KEY_BASE64` y `HMAC_KEY_BASE64` que genera `fwknop -k`, y la página de reglas genera con un clic el comando del cliente y la sección de `~/.fwknoprc`;
- **Seguridad**: los paquetes se cifran con AES-256-CBC y se valida primero el HMAC (SHA-256 / 384 / 512) antes de descifrar, por lo que un paquete con clave incorrecta nunca llega al descifrado; cada paquete solo puede usarse una vez (protección contra repetición) y se rechaza si la marca de tiempo supera la vigencia (120 segundos por defecto); se puede limitar el nombre de usuario y exigir que el paquete indique la dirección que se abre;
- **Bloqueo en el cortafuegos**: los puertos protegidos se escriben en el cortafuegos del sistema (nftables, o iptables + ipset si el kernel no tiene nf_tables) y solo se abren a las direcciones que han llamado con éxito, lo que protege servicios como el sshd local que no pasan por Gately;
- **Entradas de Gately abiertas por llamada**: las reglas de Reenvío de puertos, Enrutamiento por protocolo y reenvío integrado de STUN, además de cada ruta de Enrutamiento por protocolo, pueden elegir «Requerir llamada»; lo decide Gately por sí mismo, sin permisos de cortafuegos, y funciona en Windows, macOS y contenedores sin `NET_ADMIN`;
- **Un solo puerto**: el Enrutamiento por protocolo puede reconocer paquetes de llamada enviados por TCP (`fwknop -P tcp`), de modo que el mismo puerto recibe la llamada y, según su resultado, abre rutas como SSH;
- **Permiso tras iniciar sesión web**: un sitio del servicio web puede actuar como entrada de permiso; cuando un visitante supera la autenticación del sitio, su IP de origen recibe un permiso de la regla de Port Knocking, sin instalar ningún cliente; el sitio también puede exigir admisión por llamada. Consulta [Admisión por llamada y permiso tras iniciar sesión en el servicio web](/es/docs/modules/web#knock);
- **Integración con Prevención de intrusiones**: las llamadas rechazadas publican el evento «Llamada de Port Knocking rechazada», y la plantilla integrada «Fallos de Port Knocking» de Prevención de intrusiones puede bloquear el origen por número de intentos; las llamadas de direcciones ya bloqueadas se descartan directamente;
- Lista de autorizaciones actuales (origen de la autorización, paquete de llamada o inicio de sesión web, usuario que inició sesión, tiempo restante, revocables); las llamadas correctas y rechazadas se anotan en el registro de ejecución y se pueden suscribir en el Centro de notificaciones.

## Inicio rápido: proteger el SSH local

1. Abre «Port Knocking» y añade una regla: en puerto escribe `tcp/22`, activa «Bloqueo en el cortafuegos» y deja las claves vacías para que se generen automáticamente;
2. En la tarjeta de la regla pulsa «Cliente», escribe el dominio o la IP pública del servidor y copia la línea de comandos o la sección de `~/.fwknoprc`;
3. Instala fwknop en el cliente: en macOS `brew install fwknop`, en Debian / Ubuntu `apt install fwknop-client`, en Windows instálalo dentro de WSL;
4. Tras llamar, conéctate por SSH en los siguientes 30 segundos:

```bash
fwknop -n gately-ssh && ssh user@example.com
```

Los paquetes de llamada se envían por defecto al puerto UDP 62201. Si Gately está detrás de un router, hay que reenviar el UDP 62201 junto con el puerto protegido al equipo donde se ejecuta Gately; con «Abrir automáticamente en el cortafuegos» activado, el cortafuegos local (OpenWrt fw4, firewalld, ufw, etc.) abre el puerto de llamada y, una vez activo el bloqueo, también el puerto protegido, de modo que solo la llamada decide si se puede conectar.

::: warning Evita quedarte fuera
- «Red interna sin llamada» está activado por defecto: las direcciones privadas, las de enlace local y los rangos IPv6 públicos de las interfaces del equipo (el prefijo de la interfaz si es /64 o más corto; solo el propio prefijo para los /124 o /128 habituales en servidores en la nube) no necesitan llamar; en los ajustes puedes añadir una lista de permitidos sin llamada. Las direcciones de NAT de operador (100.64.0.0/10) no se incluyen, porque otros clientes del mismo operador también las usan; si usas Tailscale, añade 100.64.0.0/10 a la lista;
- Al activar el bloqueo, las conexiones ya establecidas no se ven afectadas, y tampoco cuando caduca la autorización;
- Al detener Gately con normalidad se eliminan las reglas de bloqueo; si se termina a la fuerza (`kill -9`, `docker rm -f`) las reglas permanecen hasta que Gately vuelve a arrancar. Asegúrate de poder acceder a la máquina desde la red interna o por otra vía.
:::

## Reglas

| Campo | Descripción |
| --- | --- |
| Puerto | Puertos cuya apertura puede solicitar el cliente, como `tcp/22` o `udp/51820`, hasta 16. Todos los puertos del parámetro `-A` del cliente deben estar en esta lista; de lo contrario se rechaza toda la llamada. Si el bloqueo en el cortafuegos está desactivado, puede dejarse vacío: la regla solo obtiene autorizaciones mediante «Permitir tras iniciar sesión» de los sitios web, no acepta paquetes de llamada ni hace que se escuche el puerto UDP de llamada, y la tarjeta muestra «Ninguno (solo se permite por inicio de sesión web)» |
| Bloqueo en el cortafuegos | Si se activa, estos puertos se bloquean en el cortafuegos del sistema y solo se abren a las direcciones que han llamado con éxito; si se desactiva, no se modifica el cortafuegos y la regla solo la usan las entradas con «Requerir llamada» |
| Clave de cifrado / clave HMAC | Codificadas en base64; si se dejan vacías se generan automáticamente (32 / 64 bytes, igual que `fwknop -k`), o se pueden escribir las de un servidor fwknop existente. Copiar la regla genera claves nuevas. Al modificar una regla que sigue activa, cambiar los puertos, el bloqueo en el cortafuegos, la duración, el usuario requerido o la exigencia de dirección revoca todas las autorizaciones obtenidas con paquetes de llamada; cambiar solo el nombre, el grupo, la nota, las [identidades](#identities) o «una dirección a la vez» revoca solo las autorizaciones cuyas claves ya no son válidas (una identidad desactivada, eliminada o con claves nuevas, o las claves compartidas cambiadas o desactivadas). Las autorizaciones obtenidas por inicio de sesión web se conservan; desactivar o eliminar la regla revoca todas las autorizaciones |
| Algoritmo HMAC | SHA-256 (predeterminado de fwknop), SHA-384, SHA-512; debe coincidir con `--hmac-digest-type` del cliente |
| Duración del acceso | Tiempo durante el cual se pueden establecer nuevas conexiones tras una llamada correcta; 30 segundos por defecto |
| Duración máxima del cliente | Límite cuando el cliente pide una duración mayor con `-f` (300 segundos por defecto); 0 ignora la duración pedida por el cliente; si la petición es menor que la duración del acceso, siempre se usa la petición |
| Limitar usuario | Solo acepta llamadas cuyo nombre de usuario en el paquete sea este valor (fwknop envía por defecto el nombre de inicio de sesión local; se puede indicar con `-U`) |
| Exigir dirección | Rechaza los paquetes sin dirección (`fwknop -s`); el cliente debe indicar la dirección que se abre con `-R` o `-a` |

Una regla utilizada por Reenvío de puertos, Enrutamiento por protocolo o STUN no se puede eliminar; la tarjeta enumera quién la utiliza.

### Identidades: unas claves por persona {#identities}

Por defecto una regla tiene un juego de claves compartidas y todos llaman con las mismas. Al añadir identidades a la regla, cada persona (o dispositivo) llama con sus propias claves:

- Los registros, los eventos «Llamada a puertos: llamada correcta / rechazada» (campo `identity`) y la lista de autorizaciones actuales muestran qué identidad fue, y la tarjeta muestra el número de llamadas de cada identidad;
- Desactivar o eliminar una identidad, o darle claves nuevas, solo revoca las autorizaciones de esa identidad; los demás siguen igual;
- El diálogo «Cliente» genera el comando de fwknop y la sección de `~/.fwknoprc` para la identidad elegida, así que cada persona recibe solo sus propias claves;
- Las identidades usan los puertos, el algoritmo HMAC, la duración, el usuario requerido y la exigencia de dirección de la regla. El reenvío de puertos, el enrutamiento por protocolo, STUN y los sitios web siguen haciendo referencia a la regla, y una llamada correcta de cualquier identidad abre el acceso según la regla.

Una regla admite hasta 64 identidades, y entre todas las reglas activas se aceptan como máximo 1024 claves de llamada (claves compartidas e identidades). Todas las claves HMAC deben ser distintas (también entre reglas); si no, un paquete solo coincide con la primera. Una regla sin puertos no acepta paquetes de llamada y no puede tener identidades.

- **Desactivar claves compartidas**: solo se aceptan las claves de las identidades y los clientes con las claves compartidas ya no pueden llamar; debe haber al menos una identidad activada.
- **Una dirección a la vez**: cada identidad (las claves compartidas cuentan como una) solo mantiene autorizada una dirección a la vez. Tras una llamada correcta desde una dirección nueva, las demás direcciones que esa identidad abrió llamando se revocan de inmediato (incluidas las entradas del cortafuegos), de modo que varias personas no pueden usar las mismas claves a la vez. No afecta a las conexiones establecidas ni a las autorizaciones por inicio de sesión web. Si dos identidades llaman desde la misma dirección, la autorización queda a nombre de la más reciente.

### Qué dirección se abre

- `-R` (recomendado): el cliente consulta primero su IPv4 pública y la escribe en el paquete; aunque el paquete se intercepte por el camino, no sirve para otra dirección;
- `-s`: el paquete no lleva dirección y se abre la dirección de origen del paquete. Quien intercepte el paquete podría repetirlo antes desde su propia dirección; la regla puede activar «Exigir dirección» para rechazar estos paquetes;
- `-a dirección`: abre la dirección indicada, por ejemplo para llamar en nombre de otro dispositivo.

El cliente fwknop 2.6 solo puede enviar paquetes de llamada por IPv4 y abrir direcciones IPv4, por lo que el servidor debe ser accesible por IPv4; el propio Gately también acepta paquetes de llamada enviados por IPv6 y abre su dirección de origen.

## Ajustes globales

- **Puerto UDP de llamada**: 62201 por defecto (el puerto predeterminado del cliente fwknop); con 0 no se escucha en UDP y solo se reciben llamadas enviadas mediante Enrutamiento por protocolo. No se escucha si no hay reglas activas o si ninguna regla activa tiene puertos (solo se permiten por inicio de sesión web);
- **Abrir automáticamente en el cortafuegos**: véase arriba;
- **Red interna sin llamada / lista sin llamada**: solo afectan al bloqueo en el cortafuegos; las entradas con «Requerir llamada» se controlan con sus propias listas de IP;
- **Vigencia del paquete**: diferencia máxima entre la marca de tiempo del paquete y la hora local, 120 segundos por defecto; los relojes del cliente y del servidor deben ser precisos.

## Bloqueo en el cortafuegos {#firewall}

| Entorno | Bloqueo en el cortafuegos | Entradas de Gately abiertas por llamada |
| --- | --- | --- |
| Linux (root o `CAP_NET_ADMIN`, con nftables) | Disponible | Disponible |
| Docker (red host + `cap_add: NET_ADMIN`) | Disponible, las reglas se escriben en el host | Disponible |
| Linux sin nf_tables, como Synology DSM | Usa iptables (legacy) + ipset, ya incluidos en la imagen Docker oficial | Disponible |
| Windows, macOS, FreeBSD y Linux sin permisos | No disponible; la parte superior de la página indica el motivo | Disponible |

Gately crea en nftables una tabla independiente `inet gately_knock` sin tocar las reglas existentes del sistema: la cadena `prerouting` (prioridad -160, después del seguimiento de conexiones y antes de la traducción de destino) entrega a la cadena `gate` los paquetes dirigidos a los puertos protegidos, que permite en orden el tráfico cuyo destino no es el equipo local (las conexiones reenviadas a otros dispositivos no se ven afectadas), la interfaz de loopback, las conexiones ya establecidas, los paquetes TCP que no inician conexión, y las combinaciones dirección + protocolo + puerto de las redes sin llamada y del conjunto de llamadas aceptadas; el resto se descarta. Como se decide en prerouting, los puertos asignados a contenedores Docker se bloquean según el puerto anterior a la asignación. Las aperturas se escriben como elementos de conjunto con tiempo de expiración, y el kernel los elimina al caducar.

Sin nf_tables se usa la tabla mangle de iptables: `PREROUTING` salta a `GATELY_KNOCK`, con un conjunto ipset por cada puerto protegido (`gkn4_tcp_22`, etc., con tiempo de expiración). Si la tabla o el salto se borran por una recarga del cortafuegos, `nft flush ruleset` u otra causa, se reescriben en 10 segundos junto con las aperturas todavía vigentes. «Ver reglas del cortafuegos» en la página muestra las reglas escritas actualmente.

### Autorizaciones por inicio de sesión web {#web-grant}

Las autorizaciones obtenidas con [Permitir tras iniciar sesión](/es/docs/modules/web#knock) en un sitio web también se aplican a las reglas con el bloqueo en el cortafuegos activado: Gately escribe en el conjunto de aperturas las direcciones que realmente han visitado el sitio de entrada con esa autorización (hasta 8 por autorización; al superarlas se elimina la más antigua) junto con los puertos de la regla, con la misma caducidad que la autorización, y las elimina cuando la autorización se revoca o caduca. Las autorizaciones IPv6 se evalúan por red (/64 por defecto), pero el cortafuegos solo abre las direcciones concretas registradas. Si una dirección tiene además una autorización obtenida con un paquete de llamada, cada una se conserva hasta su propia caducidad.

::: tip SSH que llega por Reenvío de puertos
Las conexiones reenviadas por el Reenvío de puertos de Gately a `127.0.0.1:22` del equipo llegan por la interfaz de loopback y el bloqueo del cortafuegos no se aplica. En ese caso elige «Requerir llamada» en la regla de Reenvío de puertos, o protege el puerto de reenvío expuesto con el bloqueo en el cortafuegos.
:::

::: warning Atención
- **El mapeo de puertos de Docker reescribe la dirección de origen**: si Gately se ejecuta en una red bridge con el puerto de llamada mapeado con `-p 62201:62201/udp` y el userland-proxy de Docker gestiona ese mapeo (por ejemplo, al acceder por IPv6 o por la dirección de loopback, o con la integración de iptables desactivada), Gately ve como origen la dirección de la puerta de enlace de Docker: un paquete `-s` autoriza esa dirección en lugar del cliente real y puede llegar a abrir el puerto a todas las conexiones que pasen por esa puerta de enlace. Ejecuta Gately en la red host (el bloqueo en cortafuegos también lo necesita) o haz que los clientes indiquen la dirección con `-R` / `-a` y activa «Exigir dirección»;
- **El compromiso de bloquear solo SYN**: para no cortar conexiones establecidas antes de activar el bloqueo que el seguimiento de conexiones aún no tiene registradas, en TCP solo se descartan los paquetes SYN que inician una conexión y los paquetes sin SYN pasan. En teoría, un sistema con SYN cookies activadas (`net.ipv4.tcp_syncookies`) acepta, mientras su cola SYN está desbordada, un ACK suelto que adivine una cookie válida y establece la conexión; la cookie es un número de secuencia inicial de 32 bits calculado con un secreto del kernel y la hora, así que adivinarla es prácticamente imposible; si los requisitos de seguridad son muy estrictos puedes desactivar las SYN cookies (`sysctl net.ipv4.tcp_syncookies=0`).
:::

## Entradas de Gately abiertas por llamada {#gately-entries}

En el «Control de acceso por IP» de Reenvío de puertos, Enrutamiento por protocolo y STUN (reenvío integrado) se puede elegir una regla de Port Knocking como «Requerir llamada»: la dirección de origen debe tener una autorización vigente de esa regla para establecer nuevas conexiones (en UDP, al crear una sesión nueva: cuando la autorización caduca, las sesiones existentes como WireGuard se siguen reenviando, igual que las conexiones establecidas con el bloqueo en cortafuegos; las listas de permitidos / de bloqueo se siguen comprobando por paquete) y el resto de conexiones se cierran; si también hay lista de permitidos / de bloqueo, deben cumplirse ambas. La regla de Port Knocking puede tener desactivado el «Bloqueo en el cortafuegos» y servir solo a estas entradas.

El Enrutamiento por protocolo también permite elegir «Requerir llamada» en una ruta concreta; por ejemplo, en el puerto 443 la ruta HTTPS está abierta a todos y la ruta SSH solo a las direcciones que han llamado con éxito.

### Abrir un solo puerto

Añade a una regla de Enrutamiento por protocolo una ruta «Port Knocking» (protocolo `spa`); el cliente envía el paquete de llamada por TCP al mismo puerto:

```bash
fwknop -A tcp/443 -P tcp -p 443 -R -D example.com --use-hmac \
  --key-base64-rijndael <encryption key> --key-base64-hmac <HMAC key>
ssh -p 443 user@example.com
```

- La ruta de llamada entrega el paquete al módulo de Port Knocking, sin reenviarlo ni responder; el puerto de `-A` debe estar en la lista de puertos de la regla de Port Knocking (por ejemplo `tcp/443`);
- Si toda la regla exige llamada, el origen que no ha llamado solo puede usar la ruta de llamada, y tras llamar puede usar las demás rutas y el destino predeterminado;
- Los paquetes de llamada solo se reconocen si la regla tiene una ruta de llamada, sin afectar a la detección de protocolos de otras reglas;
- El puerto UDP de la regla de Port Knocking puede ponerse en 0, de modo que solo quede abierto al exterior el puerto del Enrutamiento por protocolo;
- Los puertos de Enrutamiento por protocolo con ruta de llamada y el puerto UDP de llamada no pueden figurar en una regla con bloqueo en cortafuegos (se comprueba al guardar y al activar, porque bloquearlos impediría que llegaran los propios paquetes de llamada), y siempre se excluyen al generar las reglas del cortafuegos.

## Integración con Prevención de intrusiones

Las llamadas rechazadas publican el evento «Llamada de Port Knocking rechazada» (su campo `via` es `tcp`), como máximo una vez cada 5 segundos por origen. La dirección de origen de un paquete UDP se puede falsificar y un paquete válido se puede capturar y reenviar desde un origen falso, así que, para que nadie use Prevención de intrusiones para bloquear IP arbitrarias, las llamadas UDP rechazadas solo se cuentan y se registran (una a una si superaron el HMAC) y no publican evento; los paquetes que llegan por un puerto TCP de Enrutamiento por protocolo vienen de un handshake completo y no se pueden falsificar, así que ahí un HMAC distinto o un rechazo de la regla publican el evento. Crea en Prevención de intrusiones una regla con la plantilla «Fallos de Port Knocking» (por defecto 5 veces en 10 minutos bloquean 1 día) y se bloquearán los orígenes que sigan enviando paquetes no válidos por TCP; las llamadas de direcciones bloqueadas se descartan. Los datos que no tienen formato de llamada (escaneos de puertos, otros protocolos) solo se cuentan y no publican evento.

Antes de autenticar se limita la tasa por rango de origen (como máximo 10 paquetes por segundo por cada /24 IPv4 y cada /64 IPv6) y después se aplica un presupuesto global medido en trabajo HMAC (cada paquete cuesta el número de claves que aceptan llamadas (claves compartidas e identidades) por una unidad cada 512 bytes; 20000 unidades por segundo, unos 20 000 paquetes típicos por segundo con una sola clave); el resto se descarta, de modo que una avalancha de paquetes falsos no consume toda la CPU y la avalancha de un origen no desplaza las llamadas normales de otros. Los paquetes que no superan el HMAC no se registran uno a uno: cada minuto se resumen en una línea «M paquetes de llamada no válidos de N orígenes».

## Limitaciones

- Solo admite el modo Rijndael (AES) CBC con HMAC de fwknop; no admite GnuPG, el modo IV antiguo ni paquetes sin HMAC;
- No ejecuta mensajes de comando (`fwknop -C`) ni admite solicitudes de acceso NAT (`-N`);
- Las autorizaciones se guardan en memoria; tras reiniciar Gately hay que volver a llamar o volver a abrir la página de permiso web (las conexiones ya establecidas no se ven afectadas). Los registros contra repetición también están solo en memoria, así que tras un reinicio se rechaza cualquier paquete con marca de tiempo anterior al arranque; si el reloj del cliente va por detrás del servidor, una llamada en los primeros segundos tras el reinicio puede rechazarse y basta con volver a llamar.
