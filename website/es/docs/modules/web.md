# Servicios web

Sirve proxy inverso, redirecciones, archivos estáticos y texto fijo por dominio. Es el punto de entrada web unificado de Gately. Ubicación en el menú: barra lateral «Web → Servicios web».

## Estructura: listeners, sitios y sitio por defecto

La configuración tiene tres niveles:

1. Listener: un puerto de escucha. Sus campos incluyen la red (`tcp` doble pila, `tcp4`, `tcp6`), la dirección de escucha (vacía para todas las direcciones), el puerto, el interruptor de HTTPS, la versión mínima de TLS (1.0 / 1.1 / 1.2 / 1.3) y si acepta el protocolo PROXY.
2. Sitio: un sitio dentro de un listener, que se selecciona por dominio. Los dominios pueden ser un dominio exacto, `*.example.com` (coincide con subdominios de cualquier profundidad) o `*` (coincide con todo). La prioridad de coincidencia es dominio exacto, comodín y después `*`.
3. Sitio por defecto: se usa cuando ningún sitio coincide. Si no está activado, se devuelve un 404. El sitio por defecto también puede ser de tipo «Cerrar conexión», para que las peticiones a dominios desconocidos no reciban ninguna respuesta. Una vez configurado, el sitio por defecto aparece al final de la lista de sitios; si aún no está configurado, haz clic en «Configurar sitio por defecto» en la tarjeta del listener para configurarlo.

Al guardar un cambio en un sitio o en el sitio por defecto, la lista de sitios se sustituye en caliente: el listener sigue funcionando, las conexiones abiertas (incluido WebSocket) se mantienen y cada sitio conserva sus estadísticas. Si la nueva configuración no es válida, los sitios anteriores siguen sirviendo. Cambiar los ajustes propios del listener (red, dirección, puerto, HTTPS, TLS mínimo, HTTP/3, protocolo PROXY) reinicia ese listener.

Cada dominio exacto de la lista de sitios es una URL completa en la que se puede hacer clic y que se abre en una pestaña nueva, con un botón al lado que copia la URL; los dominios comodín (`*.example.com`, `*`) siguen mostrándose como texto. La URL se deduce del listener: `https` si HTTPS está activado y `http` si no, con el puerto del listener, omitiendo puertos por defecto como 443 y 80, por ejemplo `https://example.com:8443`. Cuando los visitantes llegan a través de Enrutamiento por protocolo, una asignación de puertos del router, frp o una CDN, el esquema o el puerto públicos pueden ser distintos; indíquelos en la **Dirección de acceso** del listener (por ejemplo 443 para un listener en 8443 a la que Enrutamiento por protocolo entrega desde 443, o https y 443 cuando una CDN obtiene el contenido por http en 8080). La dirección de acceso solo sirve para los enlaces y cambiarla no reinicia el listener. Al guardar, los dominios se almacenan tal como los envían los navegadores: en minúsculas, IPv6 sin corchetes y los dominios internacionalizados en punycode (`中文.com` se guarda como `xn--fiq228c.com`); se rechazan los valores que nunca pueden coincidir, como los que llevan puerto, ruta o `@`. Las importaciones desde Lucky o nginx solo omiten los dominios no válidos y lo indican.

Un sitio puede tener un «Grupo» (hasta 64 caracteres; el sitio por defecto no se puede agrupar). Bajo cada listener, la lista muestra los grupos como secciones plegables: los grupos aparecen en el orden en que se encuentran por primera vez, los sitios sin grupo van a continuación y el sitio por defecto queda siempre al final. El estado plegado de cada grupo se guarda en el navegador. Pulsa «Ordenar y lotes» para entrar en el modo de ordenación, donde puedes subir y bajar sitios, arrastrarlos dentro de su grupo, mover grupos enteros y reordenar los listeners. El orden solo afecta a la lista: la coincidencia de dominios sigue la prioridad anterior sea cual sea el orden de los sitios, y al guardar el nuevo orden la lista de sitios se sustituye en caliente sin cortar conexiones. El cuadro de búsqueda de la parte superior de la página filtra los sitios por dominio, nombre, grupo, backend o nota; durante la búsqueda, las tarjetas de listener plegadas se despliegan automáticamente.

En el modo de orden también puede marcar varios sitios (la casilla del encabezado de un grupo selecciona el grupo entero). Al arrastrar cualquier sitio marcado se mueven todos, y al soltarlos sobre un sitio o el encabezado de otro grupo pasan a ese grupo. La barra de herramientas mueve de una vez los sitios marcados a un grupo, o los saca de su grupo si deja el nombre vacío; los sitios marcados de distintos listeners se guardan por listener.

Pase el ratón por el encabezado de un grupo y pulse el lápiz para renombrarlo: todos sus elementos pasan al nuevo nombre y conservan su posición y orden. Si elige un nombre ya usado, tras confirmar se fusionan los dos grupos.

### Mover sitios {#move-sites}

Un sitio se puede mover a otro listener con toda su configuración:

- Al editar un sitio, cambia **Listener**; al guardar se guardan primero los cambios y luego se mueve el sitio. Al añadir un sitio, este campo elige en qué listener se crea.
- En el modo de orden, marca sitios (también de varios listeners), pulsa **Mover a listener** en la barra de herramientas y elige el listener y el grupo de destino (por defecto cada sitio conserva su grupo). Los sitios se colocan al final del listener de destino o al final del grupo elegido; los que ya están en el listener de destino no cambian.
- En el modo de orden, arrastra sitios encima o debajo de un sitio de otro listener para insertarlos ahí y unirlos a su grupo, o sobre el título de un grupo para ponerlos al final de ese grupo. Al arrastrar, los listeners sin sitios normales muestran una zona para soltar. Al soltar se pide confirmación.

Cada movimiento es una sola petición: el listener de origen y el de destino se recargan una vez cada uno y su lista de sitios se reemplaza en el sitio, de modo que las conexiones a otros sitios siguen abiertas. Si el listener de destino ya tiene un sitio con el mismo dominio, el movimiento se rechaza y no se modifica ningún listener; el sitio predeterminado pertenece a su listener y no se puede mover. Si la clave de un sitio ya está ocupada en el listener de destino, recibe una clave nueva, y las reglas de alerta de la monitorización de rendimiento que lo referencian se actualizan. Las sesiones están aisladas por listener y sitio, así que los visitantes con sesión iniciada deberán volver a iniciarla tras el movimiento; los contadores de uso de los enlaces compartidos se trasladan con el sitio, por lo que un enlace limitado no vuelve a ser utilizable; las estadísticas y conexiones en vivo del sitio empiezan de cero. Al mover un sitio con inicio de sesión de un listener HTTPS a uno HTTP sin cifrar se avisa de que las contraseñas y cookies de sesión se enviarán en texto plano.

### Operaciones en lote

Al marcar sitios en el modo de ordenación (también de distintos listeners), la barra de herramientas ofrece las operaciones en lote siguientes. Cada operación se envía de una vez: solo se guarda cuando todos los sitios superan la validación, y si alguno falla no se modifica nada y se indica qué sitio ha fallado. Cada listener afectado se recarga una sola vez y sustituye su lista de sitios en caliente, sin cortar conexiones.

- **Activar / Desactivar**: los sitios que ya están en el estado deseado no cambian.
- **Eliminar**: elimina los sitios marcados tras confirmar; el sitio por defecto no se puede eliminar.
- **Dominios en lote**:
  - Añadir dominios: uno por línea (también separados por comas); los dominios que el sitio ya tiene se omiten y los nuevos se añaden al final;
  - Quitar dominios: se comparan por dominio completo; los dominios que el sitio no tiene se ignoran, y un sitio no puede quedarse sin dominios;
  - Reemplazar dominio principal: cambia el sufijo de dominio por uno nuevo. Se reemplazan el dominio original y sus subdominios (incluido el comodín `*.dominio-original`); por ejemplo, al cambiar `old.com` por `new.com`, `www.old.com` pasa a ser `www.new.com`. Los dominios duplicados tras el reemplazo se fusionan, y los sitios que no coinciden quedan igual.

  Un dominio recién añadido se rechaza si aparece en varios sitios del mismo listener, ya que dentro de un listener solo el primer sitio puede coincidir con él; listeners distintos pueden usar el mismo dominio.
- **Opciones en lote**: activa o desactiva en lote Compresión, Registro de acceso, CORS, Cabeceras reenviadas y Cerrar la conexión al denegar, y asigna o quita el WAF. Los elementos que se dejan en «No cambiar» conservan la configuración propia de cada sitio. Los ajustes que afectan al acceso, como la autenticación, el control de acceso o las direcciones de backend, se cambian sitio por sitio.
- **Mover a listener**: mueve los sitios marcados a otro listener, consulta [Mover sitios](#move-sites).

El endpoint correspondiente es `POST /api/webservice/sites/batch`, con el cuerpo `{"sites": [{"listener": "clave del listener", "site": "clave del sitio"}], "action": "...", ...}`. `action` es `enable`, `disable`, `delete`, `addDomains`, `removeDomains` (con `domains`), `replaceDomain` (con `from` y `to`) o `set` (con `options`, que solo acepta los campos de opciones anteriores). La respuesta indica cuántos sitios se modificaron y cuántos no necesitaban cambios. Una petición admite hasta 500 sitios y 100 dominios. Los listeners también admiten los endpoints genéricos `PUT /api/webservice/listeners-enable` y `POST /api/webservice/listeners-delete`. Con `move`, `move` indica el destino, por ejemplo `{"listener": "clave del listener de destino"}`, y opcionalmente `group` (grupo donde poner los sitios; una cadena vacía significa sin grupo, omitirlo conserva el grupo de cada sitio) o `before` / `after` (clave de un sitio del listener de destino antes o después del cual insertar, uniéndose a su grupo). `moved` en la respuesta enumera la clave de cada sitio antes y después del movimiento.

## Tipos de sitio

| Tipo | Descripción |
| --- | --- |
| Proxy inverso | Reenvía a uno o varios backends. Las direcciones de backend tienen la forma `http(s)://host:port` y se admite WebSocket |
| Redirección | Redirige a una URL completa con código de estado 301 / 302 / 307 / 308 (por defecto 302), conservando opcionalmente la ruta y la cadena de consulta originales. La URL puede contener `{host}`, que se sustituye por el dominio que usó el visitante: `https://{host}` con «Conservar ruta y consulta» envía todas las peticiones HTTP a HTTPS en el mismo dominio. También admite variables como `{path}`, `{1}` y `{STUN_NombreRegla_PORT}`; consulta [Variables de redirección](#redirect-vars). |
| Archivos estáticos | El directorio debe ser una ruta absoluta. Con «Listado de directorios» desactivado, una petición a un directorio solo devuelve respuesta si existe `index.html`. «URL sin extensión» prueba `ruta.html` y `ruta/index.html` cuando falta un archivo (para VitePress y generadores similares, como `try_files $uri $uri.html $uri/index.html` de nginx). «Página 404» es una ruta dentro del directorio, como `404.html`, que se devuelve con estado 404 |
| Texto fijo | Contenido personalizado, `Content-Type` (por defecto `text/plain; charset=utf-8`) y código de estado (100-599, por defecto 200) |
| Cerrar conexión | No envía nada y cierra la conexión, como `return 444` de nginx. Se usa sobre todo como sitio por defecto para que las peticiones que escanean dominios desconocidos no reciban respuesta; el registro de acceso las anota como 444; no pasa por la autenticación, el control de acceso, el WAF ni las reescrituras, así que el resto de ajustes del sitio no tiene efecto |

Los sitios de cualquier tipo pueden definir un registro de acceso, orígenes cruzados (CORS), cabeceras de petición y respuesta personalizadas (un `Clave: Valor` por línea; un Valor vacío elimina esa cabecera) y pueden hacer referencia a una instancia de [Cortafuegos WAF](/es/docs/modules/waf) (si la instancia no existe o está desactivada, las peticiones pasan sin filtrar). Consulta [Cabeceras de petición y de respuesta](#headers) para ver cómo se editan las cabeceras de petición, las de respuesta y el Content-Type.

Las entradas del registro de acceso guardan la IP del cliente, el método, el dominio y la ruta, el código de estado, la duración y el nombre del sitio, seguidos del Referer y el User-Agent en formato nginx (`-` si faltan), de modo que los distintos dispositivos detrás de una misma IP pública se distinguen por el User-Agent. El registro se puede filtrar por sitio, código de estado, IP de origen y más; consulte [ver y filtrar los registros de ejecución](/es/docs/guide/configuration#log-filter).

### Variables de redirección {#redirect-vars}

El destino de un sitio de redirección puede usar las siguientes variables:

| Variable | Significado |
| --- | --- |
| `{host}` | Dominio solicitado por el visitante, sin puerto |
| `{port}` | Puerto por el que llegó la solicitud |
| `{hostAndPort}` | Dominio y puerto, igual que el Host de la solicitud |
| `{path}` / `{PATH}` | Ruta de la solicitud; `{PATH}` sin la `/` inicial |
| `{args}` / `{Args}` | Cadena de consulta; `{args}` incluye el `?` inicial (vacía si no hay consulta), `{Args}` no |
| `{domainPartN}` / `{pathPartN}` | La parte N del dominio (separado por `.`) o de la ruta (separada por `/`), contando desde 1 |
| `{ClientIP}` / `{RemoteIP}` | IP del cliente |
| `{time}` | Hora actual con el formato `2006-01-02 15:04:05`, con el espacio escrito como `%20` (`2006-01-02%2015:04:05`) para que la dirección de destino sea una URL válida |
| `{1}` | La parte que coincide con `*` en un dominio del sitio como `*.example.com`; vacía si coincidió un dominio exacto, el dominio completo si coincidió `*` o el sitio predeterminado |
| `{STUN_NombreRegla_PORT}` / `{STUN_NombreRegla_IP}` / `{STUN_NombreRegla_ADDR}` | Puerto, IP e `IP:puerto` públicos actuales de una regla de [Travesía STUN](/es/docs/modules/stun); el nombre es el que aparece en la lista y puede contener guiones bajos |

- Con «Conservar ruta» activado, si el destino contiene `{path}`, `{PATH}` o `{pathPartN}` ya no se añade la ruta original, y si contiene `{args}`, `{Args}` o su propia cadena de consulta ya no se añade la consulta original; la otra parte se sigue añadiendo;
- Cuando cambia el puerto STUN no hay que editar el sitio: la siguiente solicitud va al puerto nuevo. Si la regla referenciada no existe, está desactivada, aún no ha completado la travesía o está reintentando, el sitio devuelve 503 y el registro de ejecución indica el motivo;
- Si tras sustituir las variables el destino no tiene host (por ejemplo, `{1}` queda vacío porque coincidió un dominio exacto y el destino es solo `https://{1}`), el sitio devuelve 503; si el host del visitante contiene caracteres que nunca aparecen en un dominio, como `&`, `=`, `/` o `@`, devuelve 400 y no lo inserta en el destino;
- Los dominios del sitio admiten un solo `*`, por lo que solo existe `{1}`;
- Los destinos de redirección de las reglas de reescritura de rutas siguen admitiendo solo `{host}`; un `{xxx}` desconocido se conserva tal cual.

Ejemplo: la CDN de Cloudflare envía `*.example.com` al puerto 2053 de este equipo, y ese escucha tiene un sitio de redirección para `*.example.com` con el destino `https://{1}.v4.example.com:{STUN_web_PORT}{path}{args}`. Visitar `zly.example.com/a?b=1` redirige a `https://zly.v4.example.com:<puerto STUN>/a?b=1`; la regla STUN web reenvía a otro escucha, donde se crean sitios de proxy inverso para los dominios bajo `*.v4.example.com`.

### Opciones del proxy inverso {#proxy-options}

Son las «Opciones» de la pestaña «Básico» del editor de sitios. El icono de interrogación junto a ellas muestra una explicación breve en la interfaz y enlaza con esta sección.

| Opción | Qué hace | Cuándo activarla |
| --- | --- | --- |
| Omitir verificación del certificado del backend | No verifica el certificado al conectar con un backend HTTPS | El backend usa un certificado autofirmado o caducado, como el puerto https de un NAS o del panel de un router. Ya no se puede confirmar la identidad del backend, así que úsala solo con backends de la red local |
| Usar la dirección del backend como Host | Cambia la cabecera Host enviada al backend por la dirección del backend; desactivada (por defecto) se conserva el Host de la petición del visitante | El backend solo acepta su propia dirección, p. ej. devuelve 400 / 403, redirige a una dirección errónea o indica que el Host no coincide; suele hacer falta con sitios públicos o CDN. Mantenla desactivada si el backend separa sitios por dominio o genera enlaces con el dominio visitado |
| Desactivar reutilización de conexiones | Abre una conexión nueva con el backend en cada petición (Keep-Alive desactivado) | El backend a veces reinicia conexiones, las peticiones se quedan colgadas o no admite conexiones persistentes. Añade algo de latencia; normalmente mantenla desactivada |
| Añadir X-Forwarded-* | Escribe `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host` / `X-Forwarded-Port` y `X-Real-IP` para indicar al backend la IP real, el protocolo original, el dominio y el puerto del visitante | El backend registra IP reales o necesita saber que fuera se usa HTTPS para generar enlaces correctos (si no, puede volver a http o entrar en un bucle tras iniciar sesión). Recomendada para la mayoría de aplicaciones |
| Enviar PROXY v2 al backend | Antepone a la conexión TCP una cabecera del protocolo PROXY v2 para pasar la IP real del cliente. Al activarla, cada solicitud abre una conexión nueva y no se usa HTTP/2 hacia el backend. Si el backend es Gately, activa la recepción del protocolo PROXY en su escucha y añade este equipo a sus orígenes PROXY de confianza. | El backend no lee cabeceras HTTP pero admite el protocolo PROXY, como otro Gately, haproxy o nginx con `proxy_protocol`. Si el backend no está configurado para aceptarla, todas las peticiones fallan; si dudas, déjala desactivada |
| Usar el dominio visitado como SNI | Solo backends HTTPS: envía en el handshake el dominio que usó el visitante y verifica el certificado con él, como nginx `proxy_ssl_server_name on; proxy_ssl_name $host` | El backend elige certificado o enrutamiento por dominio en un mismo puerto, como frp `vhost_https` o un nginx con varios sitios HTTPS. Cada backend tiene su propio pool de conexiones, que nunca se reutilizan entre dominios |
| Reenviar las cookies de inicio de sesión de Gately | Pasa sin cambios al backend las cookies de inicio de sesión propias de Gately (inicio de sesión único `gately_sso`, sesión del sitio, nonce del inicio de sesión de terceros); desactivado (por defecto) no se envían al backend para no filtrar credenciales | El backend es otro Gately que se encarga de la autenticación y este sitio no tiene ninguna. Por ejemplo, un Gately público que solo hace WAF y control de acceso y hace de proxy hacia un Gately de la LAN, que gestiona la autenticación web y el inicio de sesión único. No disponible mientras este sitio tenga autenticación; úsalo solo si el backend es tu propio Gately |

Combinaciones habituales:

- Proxy hacia una aplicación web de la red local (p. ej. `http://192.168.1.10:5000`): activa solo «Añadir X-Forwarded-*»;
- El backend es una dirección https con certificado autofirmado: activa también «Omitir verificación del certificado del backend»;
- Proxy hacia un sitio público o un servicio de terceros: activa «Usar la dirección del backend como Host»;
- Delante del backend hay un servicio que enruta por SNI (p. ej. frp `vhost_https`): activa «Usar el dominio visitado como SNI».
- Gately público solo para WAF y Gately de la LAN a cargo del inicio de sesión: sin autenticación en el sitio público, activa «Reenviar las cookies de inicio de sesión de Gately» y, si necesitas la IP real, también «Enviar PROXY v2 al backend», con el Gately de la LAN aceptando el protocolo PROXY y confiando en el Gately público.

### Cabeceras de petición y de respuesta {#headers}

Se configuran en la pestaña «Cabeceras» del editor de sitios. Las cabeceras de petición se envían al backend y solo se aplican a sitios de proxy inverso; las de respuesta se devuelven al cliente y se aplican a sitios de cualquier tipo.

- Edición por filas: al hacer clic en el campo del nombre se listan las cabeceras habituales y su uso; al elegir una se rellena un valor habitual, y el campo de valor ofrece otros valores habituales. Ambos campos admiten también texto libre. Un valor vacío elimina la cabecera, y una línea `Key: Value` pegada en el campo del nombre se separa automáticamente;
- «Añadir cabeceras de seguridad comunes» añade `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN` y `Referrer-Policy: strict-origin-when-cross-origin`, sin duplicar las que ya existen;
- El «Modo texto» permite pegar varias líneas `Key: Value` de una vez; el contenido se conserva al volver a la edición por filas;
- Las cabeceras de petición adicionales de cada ruta en «Backends por ruta» se editan igual y se aplican después de las del sitio; con el mismo nombre prevalece la de la ruta.

Cada fila recibe información al instante. El rojo indica que el guardado fallará o la cabecera no surtirá efecto, como un nombre ausente, un nombre con espacios, dos puntos u otros caracteres no válidos, o la falta de dos puntos en el modo texto. El naranja indica que se puede guardar pero el resultado quizá no sea el esperado:

- `Host` definido como cabecera de petición no tiene efecto; para reescribirlo, activa «Usar la dirección del backend como Host»;
- Un valor fijo en `X-Forwarded-For` o `X-Real-IP` sobrescribe la IP real del cliente; para pasarla, activa «Añadir X-Forwarded-*»;
- `Content-Length`, `Connection`, `Transfer-Encoding` y similares los gestiona la conexión; definirlos a mano se ignora o rompe la transferencia;
- Los valores se envían tal cual y no se admiten variables de nginx como `$host`; los marcadores como `<token>` de los valores sugeridos deben sustituirse;
- Las cabeceras con el mismo nombre se aplican en orden y prevalece la última.

En los sitios de «Texto fijo», el `Content-Type` se elige entre los tipos habituales o se escribe a mano. La interfaz comprueba el formato `tipo/subtipo; parámetro=valor`, avisa de que un `text/*` sin `charset` puede mostrar mal el texto no inglés y usa `text/plain; charset=utf-8` si se deja vacío.

## Transporte y protocolos

- HTTPS: usa un certificado del módulo [Certificados SSL](/es/docs/modules/ssl) que cubra el dominio;
- HTTP en el mismo puerto: un listener con HTTPS también puede aceptar HTTP sin cifrar; el protocolo se distingue por el primer byte de cada conexión. *Redirigir a HTTPS* envía `http://host:puerto` a `https://host:puerto` (301 para GET/HEAD, 308 para otros métodos), útil cuando solo hay un puerto disponible y los visitantes pueden escribir el esquema equivocado; *Servir* responde con HTTP sin cifrar. Las validaciones ACME HTTP-01 se siguen respondiendo y cambiar esta opción reinicia el listener;
- HTTP/3 (QUIC): actívalo en un listener con HTTPS para servir también HTTP/3 por UDP en el mismo número de puerto; se anuncia al navegador con la cabecera de respuesta `Alt-Svc`. Hay que permitir ese puerto UDP en el cortafuegos y en el reenvío de puertos;
- WebSocket: el proxy inverso lo admite automáticamente;
- Protocolo PROXY: un listener puede aceptar cabeceras PROXY enviadas por componentes situados antes (como haproxy y frp). Solo confía en las IP o subredes indicadas en «Ajustes globales → Orígenes PROXY de confianza»; si se deja vacío, no confía en ningún origen. El proxy inverso puede enviar cabeceras PROXY al backend. También se acepta `X-Forwarded-Proto: https` en las conexiones de un origen de confianza: las cookies de sesión llevan Secure, los puertos que redirigen HTTP en claro dejan de redirigir y, con «Añadir X-Forwarded-*» activado, se pasa https al backend.

### IP real del visitante (detrás de una CDN / WAF) {#real-ip}

Cuando un sitio está detrás de un proxy HTTP como Cloudflare, Alibaba Cloud CDN o Tencent Cloud CDN, las conexiones llegan desde las direcciones de origen del proxy. Indica los proxies de confianza (las IP o rangos de origen del proxy; se admiten varios proveedores) en **Configuración global → IP real del visitante** y elige los encabezados de IP real. Gately toma entonces la dirección del visitante de los encabezados y la usa en los registros de acceso, listas de IP permitidas/bloqueadas, acceso solo LAN, filtro por país, bloqueos por fallos de autenticación, estadísticas, sondeo de rutas y en los `X-Real-IP` / `X-Forwarded-For` enviados a los backends.

- Los encabezados solo se leen cuando el par original es un proxy de confianza. El par original es la dirección tras procesar el protocolo PROXY: la del encabezado PROXY en conexiones desde un origen PROXY de confianza y, si no, el par TCP. Los encabezados de clientes que se conectan directamente siempre se ignoran;
- Los encabezados se revisan en el orden elegido y gana el primero que da una dirección válida; sin elección se usa `X-Forwarded-For`. Los encabezados de varios valores como `X-Forwarded-For` se leen de derecha a izquierda saltando los proxies de confianza y se usa la primera dirección no confiable, así que nunca se toman valores falsificados a la izquierda por el cliente;
- Elige solo encabezados que tu proxy sobrescribe. Cloudflare sobrescribe `CF-Connecting-IP`, por ejemplo, mientras que otros proxies suelen reenviar tal cual un encabezado del mismo nombre enviado por el cliente; ahí `X-Forwarded-For` es más seguro;
- Con Cloudflare conviene usar `CF-Connecting-IP` como encabezado de IP real: Cloudflare escribe en él directamente la dirección del visitante. `X-Forwarded-For` se lee de derecha a izquierda saltando los proxies de confianza, así que si los proxies de confianza incluyen los rangos completos de Cloudflare, una solicitud enviada a través de la salida propia de Cloudflare (como Workers) puede llevar una dirección falsificada a la izquierda;
- `0.0.0.0/0` y `::/0` se rechazan al guardar;
- Si el Enrutamiento por protocolo entrega conexiones a los Servicios web por loopback, activa el envío del encabezado PROXY en el Enrutamiento por protocolo y su recepción en el listener en lugar de añadir `127.0.0.1` a los proxies de confianza; de lo contrario, quien se conecte directamente al Enrutamiento por protocolo podría falsificar los encabezados. Las rutas de paso del Enrutamiento por protocolo siguen siendo reenvío TCP puro y no se ven afectadas;
- Las solicitudes de un proxy de confianza con `X-Forwarded-Proto: https` se tratan como HTTPS;
- Sin proxies de confianza el comportamiento no cambia. Los bloqueos a nivel de conexión (reglas de cortafuegos de la Prevención de intrusiones) se aplican al origen TCP y no afectan a visitantes detrás de una CDN; los bloqueos propios de los Servicios web se aplican a la dirección real.

## Autenticación

Se configura en la página «Seguridad» del editor del sitio:

- Autenticación web: el navegador muestra una página de inicio de sesión;
- Autenticación básica: HTTP Basic;
- Usuarios: un `usuario:contraseña` por línea, y la contraseña puede ser un hash bcrypt. La autenticación básica necesita al menos un usuario cuando está activada; la autenticación web necesita al menos un usuario o una identidad de inicio de sesión de terceros cuando está activada;
- Inicio de sesión de terceros: se admiten GitHub, Google, QQ y OIDC, con tus propias aplicaciones OAuth. Las identidades permitidas se escriben una por línea con la forma `proveedor:cuenta`, por ejemplo `github:octocat` y `google:me@gmail.com`. Un `#` al final de una línea añade un comentario, y `github:*` significa cualquier cuenta de ese proveedor.

Los proveedores de inicio de sesión de terceros se configuran en «Ajustes globales»: introduce tu propio Client ID / Secret, activa el proveedor y define también una dirección de callback (debe ser atendida por uno de los listeners de Gately, por ejemplo `https://auth.example.com`). La URL de callback que debes registrar en GitHub / Google / QQ es:

```text
https://auth.example.com/__gately_auth/oauth/callback
```

El inicio de sesión de terceros de la propia interfaz de administración se configura por separado en «Ajustes → Cuenta y seguridad».

### Autenticación delegada (Authelia y similares)

Delega la autenticación en un servicio externo, como `auth_request` de nginx o `forwardAuth` de Traefik. Marca «Autenticación delegada» en la página «Seguridad». No se puede combinar con la autenticación básica o web en el mismo sitio.

| Campo | Descripción |
| --- | --- |
| `url` | El endpoint de comprobación, como el de Authelia, `http://127.0.0.1:9091/api/authz/auth-request` |
| `loginUrl` | Adónde enviar a los visitantes cuando el endpoint responde 401, como `https://auth.example.com/?rd={url}&rm={method}`. `{url}` es la dirección original (codificada como URL) y `{method}` el método de la petición. Si está vacío, se devuelve tal cual la respuesta del endpoint |
| `copyHeaders` | Cabeceras de identidad que se copian de la respuesta del endpoint a la petición al backend; por defecto son `Remote-User`, `Remote-Groups`, `Remote-Email` y `Remote-Name` |

Cómo funciona:

- Cada petición va primero a `url` como un GET sin cuerpo, con las cabeceras del visitante (cookies, `Authorization`, etc.) más `X-Original-Method`, `X-Original-URL`, `X-Forwarded-Method`, `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-Uri`, `X-Forwarded-For` y `X-Real-Ip`. El tiempo de espera es de 10 segundos;
- Una respuesta 2xx deja pasar la petición, con las cabeceras de identidad añadidas; un 401 redirige a `loginUrl` (estado 302); otras respuestas, como 403 o una redirección, se devuelven al visitante; un servicio inalcanzable produce 502;
- Las cabeceras de identidad enviadas por los visitantes se eliminan siempre, por lo que no se pueden falsificar;
- Funciona con las reglas por ruta y el «Ámbito de autenticación», de modo que solo algunas rutas (por ejemplo `/admin`) requieran inicio de sesión, y con los enlaces temporales para compartir.

### Usuarios compartidos: TOTP e inicio de sesión único {#shared-users}

Define las cuentas una sola vez y úsalas en varios sitios, con verificación en dos pasos TOTP opcional. Cubre lo que muchas personas buscan al ejecutar Authelia.

En la página «Web → Usuarios de autenticación» de la barra lateral (el diálogo de ajustes globales del servicio web solo muestra un resumen y un enlace):

- Añade usuarios con un nombre de usuario y una contraseña. Las contraseñas se guardan como hashes bcrypt y la API nunca las devuelve. Para añadir TOTP, pulsa «Activar TOTP», escanea el código QR con una app de autenticación (Google Authenticator, 1Password, etc.) y guarda. Regenerar la clave invalida la anterior;
- Opcionalmente, indica un correo, un nombre visible y grupos; el [inicio de sesión único OIDC](/es/docs/modules/oidc) los entrega a las aplicaciones como las notificaciones `email`, `name` y `groups`, y los usuarios permitidos de un cliente OIDC pueden autorizar por grupo con `group:nombre`;
- Dominio de SSO, como `example.com`: tras iniciar sesión en cualquier sitio de ese dominio o de sus subdominios, los demás sitios no piden volver a iniciar sesión. Usa una cookie de dominio padre (`gately_sso`, válida durante 7 días) que nunca se reenvía a los backends. Déjalo vacío para iniciar sesión en cada sitio por separado.

En un sitio, marca «Usar usuarios compartidos» junto a la autenticación web o básica. Los usuarios propios del sitio siguen funcionando también. También puedes marcar sitios en bloque en la lista de sitios de la página Usuarios de autenticación, agrupada por escucha y con filtro: al marcar un sitio sin autenticación también se activa la autenticación web; al desmarcar un sitio que solo se autentica con usuarios compartidos se indica «Sin autenticación al guardar», y Guardar lista esos sitios para confirmarlos; tras confirmar, su autenticación se desactiva y se puede acceder sin iniciar sesión; los sitios con autenticación delegada no pueden usar usuarios compartidos. Los cambios de usuarios, OIDC y sitios de la página se aplican juntos al pulsar Guardar abajo; si alguno no pasa la validación no se guarda nada, y salir de la página con cambios sin guardar pide confirmación.

Comportamiento:

- La página de inicio de sesión muestra un campo de código de verificación cuando algún usuario compartido tiene TOTP. Cada código sirve una sola vez;
- Cambiar la contraseña o la clave TOTP de un usuario cierra las sesiones de ese usuario;
- La autenticación básica solo acepta usuarios compartidos sin TOTP, porque el aviso de contraseña del navegador no tiene dónde introducir un código;
- Los intentos fallidos cuentan para el mismo límite de intentos y bloqueo automático que los demás inicios de sesión.

Para que Gitea, OpenList, Grafana y otras aplicaciones sepan qué usuario ha iniciado sesión, ofrece los usuarios compartidos mediante el [inicio de sesión único OIDC](/es/docs/modules/oidc).

### Cambio de contraseña por el usuario {#self-service-password}

Activa Cambio de contraseña por el usuario en la página Usuarios de autenticación y los usuarios compartidos podrán cambiar su contraseña sin pedírselo al administrador:

- Visita `https://dominio-del-sitio/__gately_auth/password` en un sitio con Usar usuarios compartidos activado; la página de inicio de sesión también muestra el enlace Cambiar contraseña. Los usuarios de [inicio de sesión único OIDC](/es/docs/modules/oidc) pueden usar `/__gately_oidc/password` en el dominio emisor;
- Introduce el nombre de usuario, la contraseña actual y dos veces la nueva (de 8 a 128 caracteres y distinta de la actual); los usuarios con TOTP también indican un código de verificación;
- Después, las sesiones del usuario en todos los sitios terminan de inmediato y debe iniciar sesión con la nueva contraseña; los tokens de actualización OIDC emitidos se revocan y los Access Tokens antiguos ya no pueden leer la información del usuario; los tokens que las aplicaciones validan por su cuenta siguen siendo válidos hasta que caducan, y las sesiones propias de las aplicaciones se mantienen;
- Tras un cambio correcto, la misma cuenta no puede volver a cambiar la contraseña durante 1 minuto, para que los cambios repetidos no hagan que FTP y SMB se recarguen una y otra vez y corten las conexiones de otras personas; si una comprobación del servicio rechaza el guardado, el motivo detallado solo se escribe en el registro del servicio web y la página pide contactar con el administrador;
- Una contraseña actual o un código incorrectos cuentan para el mismo límite de intentos y bloqueo automático que los inicios de sesión fallidos, y se rechazan los formularios enviados desde otros sitios web; el registro del servicio web anota el usuario y la IP de origen;
- La misma página también cambia la contraseña de los [usuarios de archivos compartidos](/es/docs/modules/tools#shared-users). Cada tipo de cuenta tiene su propio interruptor; con ambos activados, la página permite elegir el tipo de cuenta. Las cuentas de ambos tipos con el mismo nombre son independientes.

## Control de acceso

### Listas de IP permitidas y bloqueadas

El «Control de acceso por IP» de la página «Seguridad» del sitio puede estar desactivado, en modo lista de permitidas o en modo lista de bloqueadas. La lista admite una IP o subred por línea (como `192.168.1.0/24`) y además puede hacer referencia a listas de IP suscritas.

### Al denegar

**Al denegar**, en la pestaña Seguridad del sitio, decide qué pasa con las solicitudes rechazadas por un bloqueo, el control de acceso por IP (incluidas las reglas de ruta) o el filtro de países: por defecto **Devolver 403**; **Cerrar conexión** no envía nada y corta la solicitud, registrada como 444, igual que un sitio Cerrar conexión. Con una lista de permitidos deja pasar tus propias IP y corta al resto. La limitación de tasa (429) y los bloqueos del WAF no cambian.

El sitio predeterminado se configura igual: elige el tipo **Proxy inverso** con un backend, una lista de permitidos o de bloqueados en Control de acceso por IP (se pueden usar las listas de IP suscritas descritas más abajo) y pon **Al denegar** en **Cerrar conexión**. Las solicitudes que coinciden con la lista de permitidos, o no coinciden con la de bloqueados, se envían al backend y el resto se corta; una lista suscrita actualizada surte efecto al instante. El tipo de sitio Cerrar conexión no tiene backend y corta todas las solicitudes.

Cerrar la conexión, como tipo de sitio o como acción al denegar, solo evita la respuesta HTTP y no oculta que el servicio está activo:

- el handshake TCP ya se completó, así que el cliente sabe que el puerto escucha;
- HTTP/1.x cierra la conexión, mientras que HTTP/2 y HTTP/3 solo reinician la solicitud y mantienen la conexión abierta para reutilizarla;
- en HTTPS, los dominios sin certificado ya fallan durante el handshake TLS, así que las solicitudes que llegan al sitio por defecto ya recibieron el certificado y los dominios que contiene.

Para que los escáneres solo vean tiempos de espera después, bloquea sus direcciones en [Prevención de intrusiones](/es/docs/modules/guard); con la sincronización del cortafuegos activada, su tráfico se descarta en el kernel.

### Listas de IP suscritas

Añade listas en «Ajustes globales → Listas de IP suscritas»: indica la URL de descarga y el intervalo de actualización (24 horas por defecto); si la URL (por ejemplo, GitHub) no es accesible directamente, elige el proxy global de actualizaciones o indica una dirección de proxy en «Proxy de descarga». Gately las descarga periódicamente y las guarda en caché en el directorio de configuración, de modo que tras un reinicio se pueden usar sin conexión a Internet. El contenido admite una IP, un CIDR o un rango `inicio-fin` por línea; lo que va detrás de `#`, `;` o `//` es un comentario. Por ejemplo, para los rangos de IP de provincias y ciudades chinas puedes usar `https://metowolf.github.io/iplist/data/cncity/440000.txt` (provincia de Guangdong) de [metowolf/iplist](https://github.com/metowolf/iplist).

- Las listas de IP permitidas y bloqueadas de los sitios y de las reglas por ruta pueden hacer referencia a varias listas, que se comprueban junto con los rangos escritos a mano; coincidir con cualquiera cuenta como estar en la lista;
- Si el contenido descargado no se puede analizar (por ejemplo, se recibe una página de error), se conserva la lista anterior, se muestra el motivo en la lista y se reintenta al cabo de una hora;
- Hasta que la lista se descarga correctamente cuenta como vacía: una lista de IP permitidas deniega por tanto el acceso y una lista de IP bloqueadas no bloquea nada adicional;
- Al desactivar una lista que sigue referenciada, las listas de IP permitidas que la usan deniegan todos los orígenes y las listas de IP bloqueadas que la usan ya no bloquean ninguno;
- Se rechaza la eliminación de una lista referenciada.

### Reglas por ruta

En «Reglas por ruta», dentro de la página «Seguridad», sobrescribe la autenticación y la configuración de IP del sitio según el prefijo de ruta:

| Campo | Valores |
| --- | --- |
| `prefix` | Un prefijo de ruta que empieza por `/`, como `/admin` |
| `auth` | Heredar del sitio (vacío), `off` (sin autenticación), `on` (autenticación obligatoria) |
| `ipMode` / `ipList` | Heredar del sitio (vacío), `whitelist` o `blacklist`, con una lista de IP o subredes |

Reglas de coincidencia:

- Gana el prefijo más largo;
- La coincidencia respeta los límites de segmento de ruta: `/admin` coincide con `/admin` y `/admin/x`, pero no con `/administrator`;
- La ruta de la petición se normaliza primero (se eliminan `.`, `..` y las barras duplicadas) y después se compara, así que `/public/../admin` se trata como `/admin`;
- Solo se puede configurar una regla para el mismo prefijo.

La opción «Ámbito de autenticación» (`authOptional`): cuando está marcada, la autenticación solo se aplica a las rutas marcadas como «Obligatoria» en las reglas, y el resto de rutas son públicas. Esta opción requiere activar antes la autenticación básica o web.

### Enlaces temporales para compartir

Cuando la autenticación básica o web está activada, puedes crear enlaces para compartir de un sitio, de modo que los visitantes accedan sin cuenta:

| Campo | Descripción |
| --- | --- |
| Nombre | Una etiqueta, como «Para Alice» |
| Caducidad | Vacío para que no caduque |
| Usos `maxUses` | Número máximo de veces que se puede canjear el enlace; 0 significa sin límite. El contador se reinicia tras un reinicio |
| Límite de ruta `pathPrefix` | Si no está vacío, el enlace solo puede acceder a las rutas bajo este prefijo; debe empezar por `/` |

Un visitante abre `https://dominio/cualquier-ruta?_gs=token` para canjear una sesión. Tras un canje correcto, el servidor establece una cookie de acceso sin inicio de sesión y redirige a la dirección sin el parámetro `_gs`. La sesión dura como máximo 24 horas y nunca más allá de la caducidad del enlace. Un token no válido se trata como una visita normal y no revela el motivo del fallo. Las sesiones ya emitidas dejan de ser válidas de inmediato al eliminar el enlace. El token se genera al guardar (de al menos 16 caracteres) y el enlace completo se puede copiar desde la interfaz.

### Límite de intentos de autenticación y bloqueo automático

- Tras 10 autenticaciones fallidas desde la misma IP de origen en 15 minutos, los intentos posteriores se rechazan durante un tiempo;
- En «Ajustes globales → Bloqueo automático → Bloquear por fallos de autenticación», introduce un número de minutos (de 0 a 43200); la IP de origen se bloquea al llegar a 10 fallos. 0 significa sin bloqueo, solo limitación de intentos;
- Cuando se activa un bloqueo automático se publica un evento `AuthBanned`, que se puede enviar por el [Centro de notificaciones](/es/docs/modules/notify);
- Las direcciones IPv6 se cuentan y se bloquean por subred /64.

### Lista de bloqueos

La lista de bloqueos de «Ajustes globales» muestra las IP bloqueadas actualmente, el motivo (fallos de autenticación repetidos o bloqueo manual) y la hora de expiración, y cada entrada se puede levantar con «Desbloquear». También puedes bloquear manualmente: introduce una IP y un número de horas (de 1 a 720, es decir, 30 días como máximo); la API usa 24 horas por defecto. Las direcciones de loopback y las de los proxies de confianza no se pueden bloquear manualmente, por el motivo indicado más abajo en **Cerrar y bloquear**. Marca **Permanente** para bloquear sin caducidad (seconds -1 en la API).

La lista de bloqueos se comparte con [Prevención de intrusiones](/es/docs/modules/guard): con el módulo activado, aquí solo se muestra el número de bloqueos y un enlace a su página para gestionarlos. Los bloqueos también afectan al reenvío de puertos, STUN y otros servicios, y se descartan en el cortafuegos del sistema si la sincronización está activada; con «Ignorar direcciones de la LAN» activado, esas direcciones nunca se bloquean automáticamente.

### Filtro de países con GeoIP

El «Filtro de países» de un sitio tiene tres modos: desactivado, permitir solo y denegar. Introduce códigos de país ISO de dos letras separados por comas, como `CN, HK, US`. Cuando está activado se requiere al menos un país.

- El filtrado depende de una base de datos GeoIP, que debes importar antes en «Ajustes globales → Base de datos GeoIP»: sube un archivo `.mmdb` o introduce una URL de descarga («Proxy de descarga» usa por defecto el proxy global de actualizaciones; sin proxy de actualizaciones configurado, la conexión es directa). Se admiten MaxMind GeoLite2, DB-IP y otras bases de datos de países o ciudades; el límite de subida es de 129 MB;
- Si no hay ninguna base de datos cargada, el filtrado no tiene efecto (todos los visitantes pasan) y la interfaz muestra un aviso;
- Las direcciones de redes privadas siempre se permiten;
- En el modo «Denegar», se permiten las direcciones cuyo país no se encuentra en la base de datos.

## Backends por ruta

Para un sitio de proxy inverso, envía las peticiones bajo un prefijo de ruta a otros backends, como `location /api/ { proxy_pass ...; }` de nginx. Añádelos en «Backends por ruta», en la página «Básico» del editor del sitio.

| Campo | Descripción |
| --- | --- |
| `prefix` | Un prefijo de ruta como `/api/`. La coincidencia respeta los límites de segmento de ruta: `/api` y `/api/` coinciden ambos con `/api` y `/api/x`, pero no con `/apix` |
| `targets` | Los backends de esta ruta, escritos como los backends del sitio. Si está vacío, se usan los del sitio y solo se añaden las cabeceras siguientes |
| `requestHeaders` | Cabeceras de petición añadidas para esta ruta, una `Clave: Valor` por línea. Se aplican después de las del sitio, y una cabecera con el mismo nombre sustituye a la del sitio |

- Gana el prefijo más largo y la ruta se reenvía sin cambios (el prefijo no se elimina);
- Los demás ajustes del sitio, como la opción de Host, las comprobaciones de certificado, las cabeceras reenviadas, el protocolo PROXY, la política de balanceo de carga y las comprobaciones de estado, se aplican a todos los backends por ruta;
- La compresión, el límite de peticiones, la caché y las reescrituras se aplican a todo el sitio.

## Repartir por usuario {#user-routes}

Para un sitio de proxy inverso, envía las peticiones de cada usuario con sesión iniciada a sus propios backends, como `map $remote_user $backend` de nginx. Primero el sitio necesita autenticación básica, autenticación web o autenticación delegada. Después añade reglas en «Repartir por usuario», en la página «Seguridad» del editor del sitio: cada regla indica un conjunto de usuarios y los backends de esos usuarios, escritos como los backends del sitio.

Los usuarios se reconocen por estos identificadores:

- El nombre de un usuario del sitio o de un [usuario compartido](#shared-users);
- En el inicio de sesión de terceros, `proveedor-key:identificador`, por ejemplo `github:octocat`; los demás identificadores de esa identidad también coinciden;
- En la autenticación delegada, el `Remote-User` copiado de la respuesta del servicio de autenticación.

Reparto:

- Los usuarios no listados, y los visitantes sin sesión en rutas públicas (rutas que no requieren autenticación), usan los backends del sitio. En las rutas públicas, un visitante que ya inició sesión con la autenticación web también se reparte según su identidad, de modo que las páginas de una misma aplicación llegan al mismo backend;
- Los backends por ruta con backends propios (consulta la sección anterior) tienen prioridad; los que solo añaden cabeceras usan los backends del usuario;
- Con la caché de respuestas en memoria activada, la clave de caché incluye la regla de usuario elegida y, si se configura [Pasar identidad](#identity-headers), también la identidad enviada al backend, de modo que no se mezclan respuestas de backends o usuarios distintos.

Al guardar se comprueba que el sitio tiene la autenticación activada, que cada regla tiene al menos un usuario y backends válidos y que ningún usuario aparece en dos reglas. En la lista de sitios, un sitio con reglas por usuario muestra una etiqueta con el número de reglas.

### Pasar identidad {#identity-headers}

«Pasar identidad» envía la identidad del usuario con sesión iniciada al backend en cabeceras de petición, para apps que identifican a los usuarios por cabecera; Firefly III, por ejemplo, identifica las cuentas por `Remote-Email`. Cada fila elige un campo y un nombre de cabecera:

| Campo | Cabecera habitual | Origen |
|---|---|---|
| Nombre de usuario | `Remote-User` | El nombre del usuario del sitio o del usuario compartido; `clave-de-proveedor:id` para el inicio de sesión de terceros; `Remote-User` para la autenticación reenviada |
| Correo | `Remote-Email` | El correo del usuario compartido, o el `Remote-Email` copiado por la autenticación reenviada |
| Nombre visible | `Remote-Name` | El nombre visible del usuario compartido, o el `Remote-Name` copiado por la autenticación reenviada |
| Grupos | `Remote-Groups` | Los grupos del usuario compartido (separados por comas), o el `Remote-Groups` copiado por la autenticación reenviada |

- «Añadir cabeceras Remote-* habituales» añade de una vez las filas de la tabla para los campos que aporta el método de inicio de sesión actual;
- El correo, el nombre visible y los grupos solo se pueden elegir en sitios con [usuarios compartidos](#shared-users) o autenticación reenviada; los usuarios del sitio y el inicio de sesión de terceros solo tienen nombre de usuario. Al cambiar los datos de un usuario compartido, las sesiones abiertas envían los valores nuevos al instante; un usuario del sitio con el mismo nombre que un usuario compartido solo envía el nombre de usuario cuando inicia sesión con la contraseña del usuario del sitio;
- La autenticación reenviada solo confía en las cabeceras que copia: si `Remote-Email` y similares no están en la lista de copia, las mismas cabeceras enviadas por el visitante nunca se toman como identidad; si el servicio de autenticación no devuelve `Remote-User`, tampoco se envía nada más;
- Las mismas cabeceras enviadas por el visitante se eliminan siempre, también las escritas con guion bajo en lugar de guion (como `Remote_Email`), así que no se pueden falsificar; los visitantes sin sesión o que llegan por un enlace compartido no reciben ninguna identidad, y una cabecera no se envía si su valor está vacío;
- Al guardar se comprueba que los nombres de cabecera son válidos y no se repiten (sin distinguir mayúsculas, `-` y `_` cuentan como iguales), que no son cabeceras que afectan al reenvío o a la autenticación, como `Host`, `Cookie`, `Authorization` o `X-Forwarded-For`, y que no coinciden con las cabeceras de petición del sitio o de una regla de ruta, que sobrescribirían el resultado de la autenticación; como máximo 16 filas;
- «Pasar nombre de usuario» de versiones anteriores aparece como una fila de nombre de usuario al editar el sitio y pasa a esta lista al guardar.

## Mejoras del proxy inverso

Estas opciones están en la página «Tráfico» del editor del sitio.

### Backends, peso y reserva

La lista de backends admite una dirección por línea, seguida opcionalmente de parámetros:

```text
http://10.0.0.2:8080 weight=3
http://10.0.0.3:8080 backup
```

- `weight=N`: peso, un entero de 1 a 100, por defecto 1;
- `backup`: un backend de reserva que solo se usa cuando todos los backends que no son de reserva no están disponibles. Cuando todos los backends no están disponibles, se vuelven a probar los que no son de reserva.
- Cuando no se puede conectar con un backend (conexión rechazada, inalcanzable o tiempo de conexión agotado), la misma petición se reintenta al momento en el siguiente backend, primero en otros backends principales y después en los de respaldo, con un máximo de 3 backends. En ese punto la petición aún no se ha enviado, así que reenviar POST y peticiones similares es seguro. Los fallos tras establecer la conexión no se reintentan, y el backend fallido sigue contando para la comprobación pasiva.

### Política de balanceo de carga

| Opción de la interfaz | `lbPolicy` | Descripción |
| --- | --- | --- |
| Round robin (ponderado) | Vacío | Por defecto |
| Aleatorio (ponderado) | `random` | |
| Hash de IP (sesiones persistentes) | `ip_hash` | La misma IP de cliente siempre va al mismo backend |
| Menos conexiones | `least_conn` | Elige el menor valor de conexiones / peso |

### Comprobaciones de estado

El interruptor de comprobación activa está desactivado por defecto. Campos y valores por defecto una vez activado:

| Campo | Por defecto | Rango |
| --- | --- | --- |
| Modo `mode` | `http` | `http` (solicita la ruta de comprobación; un código de estado inferior a 400 cuenta como saludable) o `tcp` (solo prueba la conexión) |
| Ruta `path` | `/` | Empieza por `/`, solo en modo http |
| Intervalo `interval` | 10 segundos | De 1 a 3600 |
| Tiempo de espera `timeout` | 3 segundos | De 1 a 60 |
| Fallos consecutivos `fails` | 3 fallos lo marcan como no disponible | De 1 a 20 |
| Éxitos consecutivos `passes` | 2 éxitos lo recuperan | De 1 a 20 |

Si está activado el envío del protocolo PROXY al backend, las comprobaciones HTTP cambian automáticamente a TCP.

Las comprobaciones pasivas siempre están activas: entre varios backends, uno con 3 fallos de conexión consecutivos se pausa durante 30 segundos antes de volver a sondearlo (un backend único sin comprobación activa nunca se retira). Una vez activadas las comprobaciones activas, la recuperación se decide por completo según sus resultados. Cuando cambia el estado de un backend, se publican los eventos de backend caído y backend recuperado, que se pueden enviar por el [Centro de notificaciones](/es/docs/modules/notify).

### Tiempos de espera del backend {#backend-timeouts}

Tiempos de espera de las conexiones de un sitio de proxy inverso con sus backends, en segundos; 0 usa el valor predeterminado. Los backends por ruta y por usuario usan los mismos ajustes.

| Campo | Predeterminado | Rango | Significado |
| --- | --- | --- | --- |
| Conexión `timeouts.connect` | 10 s | 1 a 300 | Establecer la conexión TCP con el backend |
| Negociación TLS `timeouts.tlsHandshake` | 10 s | 1 a 300 | Completar la negociación TLS con un backend HTTPS |
| Cabeceras de respuesta `timeouts.responseHeader` | Sin límite | 1 a 86400 | Esperar las cabeceras de respuesta del backend una vez enviada la petición con su cuerpo |

- El tiempo de espera de cabeceras solo cubre la espera hasta que llegan; el cuerpo de la respuesta, las descargas y las conexiones largas como WebSocket y SSE no se ven afectados después. En subidas grandes el tiempo empieza a contar cuando el cuerpo de la petición se ha enviado por completo;
- Al agotarse se devuelve 504 (Gateway Timeout); otros errores, como una conexión rechazada, devuelven 502. Los tiempos agotados cuentan para la comprobación pasiva igual que los fallos de conexión;
- Aumenta el tiempo de conexión para backends que arrancan despacio o están detrás de enlaces lentos; fija un tiempo de cabeceras cuando un backend bloqueado deba fallar pronto.

### Tiempos de espera del visitante {#client-timeouts}

Tiempos de inactividad hacia los visitantes, en segundos; 0 usa el valor predeterminado de 60 segundos (igual que los valores predeterminados de `client_body_timeout` y `send_timeout` en nginx). Todos los tipos de sitio salvo Cerrar conexión pueden configurarlos.

| Campo | Predeterminado | Rango | Significado |
| --- | --- | --- | --- |
| Cuerpo de la petición `clientTimeouts.readIdle` | 60 s | 1 a 3600 | Espera máxima de una sola lectura del cuerpo de la petición |
| Respuesta `clientTimeouts.writeIdle` | 60 s | 1 a 3600 | Espera máxima de una sola escritura de la respuesta |

- Solo se mide una lectura o escritura que se queda bloqueada, así que las subidas y descargas grandes que avanzan, el tiempo de espera al backend y las conexiones largas como WebSocket no se ven afectados;
- Corta a los visitantes que envían el cuerpo o leen la respuesta de forma extremadamente lenta (slowloris, slow read) para que no ocupen conexiones durante mucho tiempo;
- Agotar la lectura del cuerpo devuelve 408 y cierra la conexión sin contar para la comprobación pasiva del backend; agotar la escritura cierra la conexión;
- Se aplica a HTTP/1, HTTP/2 y HTTP/3.

### Compresión

La compresión se puede activar en los sitios de proxy inverso, archivos estáticos y texto fijo (no en los de redirección). Según `Accept-Encoding`, se prefiere `br` y después `gzip`.

- Solo se comprime el contenido de tipo texto: `text/*`, JSON, JavaScript, XML, XHTML, RSS / Atom, WebAssembly y SVG; se excluye `text/event-stream`;
- Las respuestas cuyo `Content-Length` declarado es inferior a 256 bytes se dejan tal cual;
- Las respuestas que ya tienen un `Content-Encoding` se dejan tal cual; las peticiones de actualización a WebSocket pasan directamente;
- Tras la compresión, los ETag fuertes se convierten en ETag débiles.

### Límite de peticiones por IP

Limitación de peticiones con token bucket por IP de cliente. Campos:

| Campo | Descripción |
| --- | --- |
| `rate` | Peticiones permitidas por segundo |
| `burst` | Capacidad de ráfaga. Cuando se define `rate` y `burst` es 0, por defecto vale `rate` redondeado hacia abajo más 1 |
| `maxConns` | Máximo de peticiones simultáneas por IP; 0 significa sin límite |

Cuando el límite de peticiones está activado, se debe definir al menos uno de `rate` y `maxConns`.

### Caché de respuestas en memoria

Se aplica solo a los sitios de proxy inverso. La caché vive en memoria y usa expulsión LRU.

| Campo | Por defecto | Rango |
| --- | --- | --- |
| Duración por defecto `ttl` | 0 | De 0 a 30 días (en segundos). Cuando el origen no declara ninguna directiva de caché, las respuestas 200 se almacenan durante este tiempo; 0 significa que solo se almacenan las respuestas que el origen permite explícitamente |
| Capacidad de la caché `maxMb` | 64 | De 1 a 4096 MB |

Reglas de caché:

- Solo se almacenan las peticiones GET, y la petición no debe contener las cabeceras `Authorization`, `Cookie` ni `Range`, ni `Cache-Control: no-cache`;
- Solo se almacenan las respuestas con estado 200; se omite una respuesta si contiene `Set-Cookie`, `Vary: *` o un `Cache-Control` con `no-store` / `private` / `no-cache`;
- Se respeta primero `s-maxage` y después `max-age`;
- Una entrada individual está limitada a 4 MB y también a un máximo de una cuarta parte de la capacidad total;
- Un acierto devuelve las cabeceras `X-Cache: HIT` y `Age`, y un fallo devuelve `X-Cache: MISS`;
- La clave de caché incluye el Host, la URI completa de la petición y `Accept-Encoding`; con el [reparto por usuario](#user-routes), incluye también la regla de usuario elegida, y la identidad enviada al backend cuando se configura [Pasar identidad](#identity-headers).

### Reescritura y redirección de rutas

Las reglas se evalúan en orden y la evaluación se detiene en la primera coincidencia.

| Campo | Descripción |
| --- | --- |
| `pattern` | Una expresión regular que se compara con la ruta de la petición, como `^/old/(.*)$` |
| `to` | El resultado de la sustitución; usa `$1` para referirte a los grupos |
| `redirect` | Desactivado para una reescritura interna (la ruta reescrita se envía al backend y el destino debe empezar por `/`); activado para una redirección |
| `code` | Código de estado de la redirección 301 / 302 / 307 / 308, por defecto 302 |

Las redirecciones conservan los parámetros de consulta originales. El destino de una redirección puede contener `{host}`, el dominio que usó el visitante. Las reglas de redirección se ejecutan antes de la autenticación, como `location = / { return 302 ...; }` de nginx, de modo que una ruta como `/` puede enviar a los visitantes a otro sitio sin pedirles que inicien sesión.

## Páginas de error {#error-pages}

La pestaña «Páginas de error» del editor de sitios define las páginas 404 y 5xx que ven los visitantes. Por defecto se usa el estilo propio de Gately; puedes cambiarlo por uno de los 10 estilos integrados (Minimalista, Terminal, Neón, Cielo estrellado, Periódico, Brutalista, Aurora, Píxel, Plano, Cadena de estado) o usar una página propia. Todos los estilos se adaptan a pantallas de móvil, el texto sigue el idioma del navegador del visitante y el botón con forma de ojo, abajo a la derecha de la tarjeta, permite previsualizar distintos códigos de estado en tamaño de escritorio y de móvil.

- Alcance: las páginas 404 y 5xx que Gately genera para un sitio, por ejemplo el 502 cuando no se alcanza el backend, el 404 cuando no existe un archivo estático, el 503 cuando el destino de una redirección no está disponible temporalmente y el error cuando el servicio de autenticación de reenvío no está disponible. Si un sitio estático tiene una «Página 404», esta sigue teniendo prioridad cuando falta un archivo. El 403 del control de acceso y el 429 de la limitación de tasa no se ven afectados.
- Sitio predeterminado: su configuración se aplica también al 404 que se muestra cuando ningún sitio coincide con el dominio solicitado, incluso si el sitio predeterminado está desactivado.
- Los clientes que no son navegadores (`curl`, llamadas a la API, etc.) siguen recibiendo un error en texto plano, como antes.

### Interceptar errores del backend {#intercept-errors}

Un sitio de proxy inverso puede activar «Interceptar errores del backend» para sustituir los códigos de estado indicados que devuelve el backend por la página seleccionada arriba, como `proxy_intercept_errors on` más `error_page` en nginx. Separa los códigos con comas; se admiten rangos y abreviaturas, por ejemplo `404, 500-599` o `5xx`, y solo se aceptan valores de 400 a 599.

- Solo se sustituyen las peticiones con las que el navegador abre una página (`Accept` contiene `text/html`). Las peticiones de API, imágenes, scripts, etc. conservan la respuesta del backend, así que el 404 en JSON de una aplicación de una sola página nunca se cambia por una página web.
- Se conservan el código de estado y cabeceras como `Set-Cookie` y `Retry-After`; se eliminan las cabeceras que describen el cuerpo original (codificación de contenido, `ETag`, etc.). Las peticiones `HEAD` devuelven solo cabeceras.
- «Opciones en lote» permite cambiar el estilo de la página de error de varios sitios a la vez; la configuración de intercepción de cada sitio no cambia.

### Páginas personalizadas {#custom-error-pages}

Las páginas personalizadas se guardan en una biblioteca de páginas que se gestiona en «Ajustes globales → Páginas de error personalizadas» o con «Gestionar páginas personalizadas» en la pestaña «Páginas de error» de un sitio; varios sitios pueden compartir la misma página. Una página puede tener dos orígenes:

- Subida o pegado: elige un archivo `.html` / `.htm` (UTF-8, hasta 256 KB) o pega el código directamente. El contenido se guarda en la configuración y se puede seguir editando.
- Archivo del servidor: indica la ruta absoluta de un archivo `.html` / `.htm` (en Docker, una ruta dentro del contenedor). En cada uso se comprueba si el archivo ha cambiado. Un enlace simbólico también debe apuntar a un archivo HTML, y se rechazan los archivos cuyo contenido no parezca HTML. Si el archivo se elimina o su contenido deja de ser válido, los visitantes ven el estilo predeterminado y la página aparece como no disponible en la biblioteca.

Las páginas pueden usar marcadores de posición, que se sustituyen por los datos del error actual y se escapan como HTML al generar la salida:

| Marcador | Contenido |
| --- | --- |
| `{code}` | Código de estado, como 502 |
| `{status}` | Frase de estado en inglés, como Bad Gateway |
| `{title}`, `{detail}` | Título y descripción del error, en el idioma del visitante |
| `{host}`, `{path}` | Dominio y ruta solicitados |
| `{time}` | Hora del error (UTC) |

Antes de guardar se comprueban automáticamente la seguridad y la usabilidad, y se muestra una vista previa:

- Una página de error comparte origen con el sitio. Para evitar que se use para robar cookies o falsificar acciones, al guardar se eliminan scripts, atributos de evento como `onclick`, enlaces `javascript:`, iframes, objetos de plugin, formularios, `<base>`, `<meta http-equiv>` (redirecciones automáticas, etc.) y construcciones de estilo peligrosas; el resultado de la comprobación enumera lo que se ha eliminado.
- La salida lleva `Content-Security-Policy: sandbox; default-src 'none'; ...`: la página se ejecuta en un sandbox aislado, por lo que los scripts inyectados no pueden ejecutarse, y solo se cargan los estilos en línea y las imágenes o fuentes incrustadas como `data:`, de modo que la dirección del visitante no se filtra a terceros. Las hojas de estilo, imágenes y fuentes externas y las direcciones relativas del sitio no se cargan, y la comprobación lo advierte; escribe los estilos en `<style>` e incrusta imágenes y fuentes como `data:`.
- Se rechaza guardar si el contenido está vacío, no es UTF-8, supera el tamaño máximo o no es HTML. Si falta `viewport`, la declaración de codificación no es UTF-8 o no se usa ningún marcador, se muestra una sugerencia.

## Estadísticas de acceso y monitorización

### Estadísticas del sitio {#site-stats}

En un listener en ejecución, pulsa «Estadísticas» en la fila de un sitio para abrir la ventana de estadísticas, donde puedes elegir la última hora, las últimas 6 horas o las últimas 24 horas:

- Estadísticas de acceso por minuto: un punto por minuto, conservadas durante las últimas 24 horas, con una curva de número de peticiones y curvas de errores 4xx / 5xx;
- Tiempo hasta el primer byte: P50 / P95 por minuto y el P95 máximo del rango elegido. La latencia va desde la llegada de la petición hasta el envío de las cabeceras de respuesta, sin la transferencia del cuerpo; WebSocket y otras peticiones con cambio de protocolo, así como las peticiones cortadas, no se cuentan, y los percentiles se estiman con el tiempo medio de cada tramo fino. Para alertas por umbral consulta [Rendimiento](/es/docs/modules/perf);
- Distribución de códigos de estado (2xx / 3xx / 4xx / 5xx) y tráfico de respuesta acumulado;
- Las 10 IP de origen y los 10 países principales (las estadísticas por país requieren una base de datos GeoIP importada);
- Los sitios de proxy inverso muestran el estado de salud, el peso, la marca de reserva, el número actual de conexiones y el número de peticiones reenviadas de cada backend, incluidos los backends por ruta y por usuario (marcados con su ruta o sus usuarios);
- Si hubo peticiones bloqueadas, los totales por motivo: bloqueo de IP, lista de IP, filtro de país, límite de tasa y WAF (los inicios de sesión fallidos no cuentan).

Las estadísticas se guardan en memoria y vuelven a empezar a contarse tras un reinicio o tras cambiar los ajustes propios del listener. Editar un sitio conserva sus contadores. Las clasificaciones siguen como máximo 5000 orígenes distintos.

### Conexiones en vivo {#connections}

En un listener en ejecución, cada fila de sitio (incluido el sitio predeterminado) muestra «N activas» tras sus estadísticas: las solicitudes que el sitio atiende en este momento, actualizadas al refrescar la página. Haz clic en **Conexiones** para abrir la ventana de conexiones; se actualiza cada 2 segundos y permite buscar por IP:

- **En vivo**: dirección y puerto de origen, protocolo (h1 / h2 / h3, WebSocket marcado como WS), solicitud (método, host y ruta, sin la cadena de consulta), hora de inicio, duración hasta ahora y tráfico (descarga; también subida si hay cuerpo de solicitud o WebSocket). Ordenadas de mayor a menor duración, hasta 500 filas. Con [proxies de confianza](#real-ip) configurados el origen es la dirección real del visitante y se indica el proxy por el que llegó.
- **Historial**: conexiones terminadas recientemente con hora de inicio y fin, duración, tráfico y resultado (completada / cerrada). Solo se guardan las que duraron al menos 1 segundo o se cerraron manualmente, las 200 más recientes por sitio, y se vacía al reiniciar; consulta el registro de acceso para solicitudes cortas.

La lista cuenta solicitudes en curso: una conexión HTTP/1.1 persistente no pertenece a ningún sitio entre solicitudes, y una conexión HTTP/2 o HTTP/3 puede llevar varias solicitudes a la vez, así que la solicitud es la unidad que corresponde exactamente a un sitio. Un WebSocket actualizado sigue en la lista hasta que se cierra la conexión.

**Cerrar** en una fila termina esa solicitud en cuanto confirmas: en HTTP/1.x se cierra toda la conexión; en HTTP/2 y HTTP/3 solo se reinicia esa solicitud y las demás de la misma conexión continúan; un WebSocket actualizado se cierra junto con su conexión al backend. También surte efecto de inmediato cuando el visitante deja de leer y el envío queda bloqueado. Una solicitud cerrada se registra como 444 en el registro de acceso (o conserva su código si la respuesta ya había empezado) y no cuenta como error del backend ni para la detección de rutas. El visitante puede reconectarse al instante; para impedirlo usa **Cerrar y bloquear**, que añade la dirección de origen (su rango /64 en IPv6) a la [lista de bloqueos](#lista-de-bloqueos) y cierra sus solicitudes en curso en todos los listeners y sitios. No se pueden bloquear las direcciones de loopback (todas las solicitudes que llegan por frp o por un reenvío local sin protocolo PROXY aparecen con ellas) ni las direcciones incluidas en los proxies de confianza (en IPv6 se comprueba por /64), porque bloquearlas cortaría el paso a muchos visitantes ajenos; cerrar sin bloquear no tiene esta restricción. El bloqueo dura de 1 a 720 horas, o marca **Permanente**.

Las cuentas y tokens de solo lectura pueden ver la lista; cerrar y bloquear requieren permiso de escritura y quedan en el registro de auditoría.

### Topología de red {#topology}

**Web → Topología** en el menú lateral es una página propia (`#/webtopology`); el botón **Topología** de la cabecera de Servicios web también la abre. Muestra Internet → Gately → listeners → sitios / acciones del listener → backends / destinos, se actualiza cada 2 segundos, se pausa mientras la página está en segundo plano y el lienzo sigue el modo claro u oscuro del tema actual:

- **Nodos**: los listeners muestran su dirección y su estado de ejecución, en rojo si no pudieron arrancar; los sitios muestran cómo coinciden (exacto, comodín, cualquier dominio, sitio por defecto), las QPS y las peticiones en curso; los sitios de proxy inverso se despliegan en todos sus backends, incluidos los backends por ruta y por usuario (marcados con su ruta o sus usuarios), con los backends no disponibles en rojo; los sitios de redirección, archivos estáticos, texto fijo y cierre de conexión llevan cada uno a un nodo de destino. Un sitio por defecto que cierra conexiones, las peticiones HTTP sin cifrar redirigidas en un puerto HTTPS (301 / 308) y el 404 devuelto cuando ningún sitio coincide y no hay un sitio por defecto activado son comportamientos esperados y se muestran en gris. Los listeners y sitios desactivados también aparecen, marcados como detenidos.
- **Enlaces**: el grosor crece con las QPS y los enlaces con tráfico muestran una animación de flujo. Con más de 120 enlaces solo se anima la ruta resaltada, y no se anima nada cuando el sistema pide reducir el movimiento.
- **Resaltado**: al pasar el ratón por un nodo o hacer clic en él se resalta toda su ruta, hacia arriba hasta Internet y hacia abajo hasta los backends, los enlaces de esa ruta muestran QPS y tráfico y el resto se atenúa; haz clic en una zona vacía para restablecer.
- **Panel de detalles**: para el nodo seleccionado, las QPS (media de los últimos 10 segundos), la media de 60 segundos, el tráfico de bajada, las peticiones en curso, el total de peticiones, los errores de backend, las peticiones bloqueadas por motivo, la distribución de códigos de estado, las curvas por segundo de peticiones y tráfico de los últimos 60 segundos y las IP activas que pasaron por él. El ⓘ junto a cada métrica explica cómo se mide. Para un sitio, el panel también abre sus ventanas de estadísticas, conexiones, registro de acceso y edición; un backend abre el registro de acceso y el editor de su sitio, y un listener se edita directamente.
- **Ver el registro de acceso**: tras seleccionar un nodo de sitio o de backend, **Registro de acceso** abre el registro de acceso solo de ese sitio (con coincidencia exacta del nombre del sitio). Si ya filtraste por una IP en la parte superior, el botón pasa a ser **Registro de acceso de esta IP** y filtra además por esa dirección. El icono de registro a la derecha de cada IP activa del panel muestra el registro de acceso de esa dirección en el sitio seleccionado; si el nodo seleccionado no es de la familia de sitios, lo muestra en todos los sitios. Las condiciones de filtro de la ventana de registro se pueden seguir modificando, y la condición de sitio aparece como una etiqueta «Sitio: …» que se puede quitar. Solo los sitios con el registro de acceso activado tienen un historial completo; si está desactivado, la ventana lo indica en la parte superior. Un sitio sin nombre se filtra por su primer dominio. El botón **Registros** de cada fila de sitio en la página Servicios web abre la misma ventana.
- **IP activas**: direcciones de visitantes con peticiones en los últimos 60 segundos, más las que aún tienen peticiones en curso. El lienzo solo muestra recuentos; las direcciones completas aparecen en el panel de detalles. Escribe o elige una dirección arriba (o haz clic en una del panel) para resaltar solo la ruta que siguió, hasta el backend exacto; **Quitar filtro** lo restablece. Cada sitio, backend y acción del listener recuerda hasta 1024 direcciones recientes y la API devuelve como máximo 500, así que con demasiados orígenes el panel indica que la lista está incompleta. Con [proxies de confianza](#real-ip) configurados son las direcciones reales de los visitantes.
- **Ocultar**: con el interruptor activado, las categorías elegidas se muestran con `*` para compartir capturas; solo afecta a esta página y la búsqueda sigue usando el texto completo. Las categorías son IP de visitantes (lista de IP activas y filtro; la última parte de IPv4, los últimos 64 bits de IPv6), direcciones de backend (IP y nombres de host de backends y destinos de redirección), direcciones de escucha (IP de escucha, mensajes de error del listener y sitios que usan una IP como dominio) y dominios (dominios de los sitios y dominios en sus nombres; se conservan la primera etiqueta y el sufijo, como `nas.***.com`). Las tres primeras están marcadas por defecto y la elección se recuerda.
- **Modo de ocultación**: arriba del menú Ocultar se elige **Asteriscos** o **Alias**. Los asteriscos ocultan parte del valor y conservan la red y los extremos; los alias sustituyen el valor completo por un número constante en esta página: las IP de visitantes pasan a `Visitor-01`, las IP de backends y destinos de redirección a `Backend-01`, las direcciones de escucha a `Listen-01`, y los dominios conservan solo el sufijo con la parte registrable y la parte de host numeradas por separado, de modo que `nas.home.example.com` pasa a `host-01.domain-01.com` y los subdominios de un mismo dominio comparten su número `domain`. Los valores iguales reciben siempre el mismo alias y los distintos, alias distintos, así que se sigue viendo qué nodos son el mismo. Los números se asignan por orden de aparición, solo se guardan en la página abierta y se renumeran al recargar.
- **Filtrar**: **Filtrar**, arriba a la derecha, limita la vista a algunos listeners; **Solo problemas** muestra los listeners que no arrancaron (con todos sus sitios), los sitios habilitados que no están en marcha y los sitios con algún backend no saludable, mientras que los listeners y sitios deshabilitados se consideran esperados; **Con bloqueos** muestra, por motivo (bloqueado, lista de IP, filtro por país, límite de tasa, WAF), solo los sitios que han bloqueado peticiones por ese motivo desde que arrancó Gately. Las condiciones se aplican a la vez y se combinan con la búsqueda de la cabecera; al filtrar por problemas o bloqueos se ocultan las acciones a nivel de listener y los listeners sin sitios restantes. Los filtros duran mientras la página esté abierta.
- **Solo activos**: con muchos servicios, pulsa **Solo activos** arriba a la derecha para ver solo los sitios y acciones de listener con peticiones en los últimos 60 segundos, peticiones en curso o IP activas que pasan por ellos; los sitios proxy conservan solo sus backends activos (un sitio cuyos backends no recibieron peticiones, por ejemplo porque el control de acceso las bloqueó, se muestra como nodo final) y los listeners sin actividad se ocultan. En este modo no se aplican el plegado de listeners ni los nodos de resumen "N más", y el lienzo se ajusta solo para mostrar estos nodos completos (al menos al 50 % en Árbol, Vertical y Sankey), actualizándose cuando los servicios empiezan o dejan de estar activos. El número del botón indica cuántos sitios están activos ahora, sigue al filtro y la búsqueda y se muestra aunque el modo esté desactivado. Funciona junto con el filtro, la búsqueda, el resaltado por IP y todos los diseños, y se recuerda por dispositivo; si nada ha recibido peticiones en los últimos 60 segundos, el lienzo lo indica y **Mostrar todo** sale del modo.
- **Diseño**: el menú de diseño de arriba a la derecha ofrece siete; la elección se recuerda, y al cambiar hay un fundido y el lienzo se reajusta. **Árbol** dispone cinco columnas de izquierda a derecha con el mayor detalle; **Vertical** se despliega de arriba abajo como un organigrama, se abre centrado en Gately y se recorre arrastrando a los lados; **Radial** pone Gately en el centro con listeners, sitios y backends en tres anillos concéntricos y los nombres a lo largo del radio; **Sankey** dibuja cada nodo como una barra y cada enlace como una banda, divide la barra del padre según la parte de QPS de cada hijo, y la altura de las barras y el ancho de las bandas siguen al tráfico mientras los nodos no se mueven; **Sunburst** pone Gately en el centro con listeners, sitios y backends como tres anillos de segmentos; **Treemap** y **Burbujas** anidan Gately, listeners, sitios y backends como bloques o círculos, con la misma área para cada backend (o destino). Sunburst, Treemap y Burbujas no tienen enlaces y son más oscuros cuanto más tráfico, útiles para ver muchos sitios de un vistazo. Sankey, Sunburst, Treemap y Burbujas no tienen botones de plegado en el lienzo; pliega o expande un listener desde su panel de detalles.
- **Nodos sobrantes contraídos**: por defecto cada listener muestra sus primeros 8 sitios y cada sitio sus primeros 6 backends; el resto se agrupa en un nodo «N sitios / backends más» con su QPS total y las solicitudes en curso. Haz clic para expandirlos todos y en el nodo **Contraer** añadido al final para volver a agruparlos. Las IP activas de los nodos contraídos cuentan en este nodo y el resaltado por IP se detiene en él. Los sitios y backends con peticiones en los últimos 60 segundos o en curso nunca se contraen: siguen visibles en su orden original aunque superen el límite, de modo que el nodo de resumen solo agrupa los inactivos y su subtítulo dice "Sin solicitudes en los últimos 60 segundos".
- **Desplazar, zoom y tamaño del lienzo**: arrastra el lienzo para desplazarlo y usa la rueda para moverte arriba y abajo (la página se desplaza al llegar al borde); mantén Ctrl/⌘ al desplazar, o pellizca en el trackpad o la pantalla táctil, para hacer zoom entre 15 % y 250 % alrededor del puntero o del centro del pellizco. Los controles de abajo a la derecha acercan, alejan, vuelven al 100 % y ajustan al lienzo (ver todo, icono de diana); el botón del extremo derecho hace que el lienzo ocupe toda la ventana del navegador, y Esc o el mismo botón lo devuelven. Arrastra la barra bajo el lienzo para cambiar su altura (también en pantallas táctiles); la altura se recuerda por dispositivo, y un doble clic en la barra o **Altura automática** vuelve al ajuste automático.
- El cuadro de búsqueda de la cabecera filtra los sitios en la topología (nombre, dominio, grupo, dirección de backend); los listeners se pueden plegar uno a uno o todos a la vez. **Editar** y **Editar escucha** en el panel de detalles abren en esta página el mismo editor que la página Servicios web; al guardar, la topología se actualiza al instante sin salir de la página. Los diálogos de estadísticas, conexiones y registro de acceso también se abren aquí.

Todos los números salen de los contadores en memoria de esta instancia: se reinician al reiniciar, editar un sitio conserva sus contadores, cambiar ajustes del listener como el puerto reinicia los contadores de ese listener y no se suman otras instancias de Gately. La vista lee `GET /api/webservice/topology`, accesible para cuentas y tokens de solo lectura.

### Métricas de Prometheus

El endpoint global `GET /api/metrics` reúne las métricas de todos los módulos; consulta [Métricas de monitorización](/es/docs/reference/monitoring). El endpoint del módulo que se describe a continuación solo incluye las métricas de los servicios web.

El endpoint de métricas es `GET /api/webservice/metrics`, en formato de texto de Prometheus, y requiere credenciales de inicio de sesión o un [token de API](/en/docs/reference/api-reference).

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/webservice/metrics
```

| Métrica | Tipo | Descripción |
| --- | --- | --- |
| `gately_site_requests_total` | counter | Peticiones acumuladas de un sitio |
| `gately_site_errors_total` | counter | Errores de backend de un sitio |
| `gately_site_active_requests` | gauge | Peticiones en curso |
| `gately_site_response_bytes_total` | counter | Bytes de respuesta acumulados |
| `gately_site_responses_total` | counter | Respuestas contadas por clase de estado (etiqueta `class`: de `2xx` a `5xx`) |
| `gately_site_blocked_total` | counter | Peticiones bloqueadas por motivo (etiqueta `reason`: `ban` bloqueo de IP, `ip` lista de IP, `country` filtro de país, `rateLimit` límite de tasa, `waf`) |
| `gately_upstream_up` | gauge | Si un backend está saludable: 1 saludable, 0 no disponible (con la etiqueta `upstream`) |
| `gately_upstream_requests_total` | counter | Peticiones reenviadas a un backend (con la etiqueta `upstream`) |
| `gately_upstream_response_bytes_total` | counter | Bytes del cuerpo de respuesta devueltos por un backend, sin los datos tras una actualización a WebSocket (con la etiqueta `upstream`) |
| `gately_listener_requests_total` | counter | Peticiones que el listener atiende por sí mismo sin un sitio, etiqueta `action`: `https_redirect` (HTTP sin cifrar redirigido en un puerto HTTPS) o `not_found` (404 cuando ningún sitio coincide); solo lleva la etiqueta `listener` |

Salvo `gately_listener_requests_total`, las métricas llevan las etiquetas `listener` (nombre del listener#clave del listener) y `site` (nombre del sitio o, si no tiene, el primer dominio). Los backends por ruta y por usuario llevan además una etiqueta `route` en `gately_upstream_*`: el prefijo de ruta, o `@` seguido de la lista de usuarios. La etiqueta `upstream` es la dirección del backend, con el usuario, la contraseña y la cadena de consulta eliminados.

## Importar configuración de nginx {#import-nginx}

El botón «Importar nginx» de la parte superior de la página convierte sitios de nginx existentes en listeners y sitios de Gately. Se recomienda ejecutar `nginx -T` en el host de nginx y pegar la salida completa: incluye todos los archivos de include, y Gately los expande igual que nginx.

- Primero la vista previa: agrupados por puerto de escucha, se muestran el tipo de cada sitio, su backend o directorio, el archivo de origen y el número de línea, y los puntos a revisar (directivas que usan variables, location con expresiones regulares, dominios sin certificado, etc.); para los sitios que no se pueden convertir se indica el motivo;
- Los sitios de un mismo puerto se añaden al listener existente o, si no hay ninguno, se crea un listener nuevo; los dominios que ya existen se omiten, así que se puede importar varias veces;
- Por defecto se importan desactivados, para revisarlos uno a uno en paralelo con nginx antes de activarlos; con «Activar tras importar» marcado se aplican de inmediato (si nginx sigue ocupando el mismo puerto, el listener nuevo no podrá iniciarse);
- Se convierten: `proxy_pass` (incluidos el peso, la reserva y la política de balanceo de carga de upstream), directorios estáticos `root` / `alias` (`autoindex`, `try_files $uri.html`, `error_page 404`), redirecciones y contenido fijo con `return`, redirecciones `rewrite` de todo el sitio, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, backends por ruta, control de acceso y redirecciones a nivel de location, y `listen ... proxy_protocol` y `quic`;
- `proxy_connect_timeout` y `proxy_read_timeout` se convierten en los [tiempos de conexión y de cabeceras de respuesta](#backend-timeouts) del sitio; el valor de location / tiene prioridad sobre el nivel server y los valores por encima del límite se importan con el límite. Los valores distintos en otras locations no se importan y la vista previa lo indica;
- `client_body_timeout` y `send_timeout` se convierten en los [tiempos de espera del visitante](#client-timeouts) del sitio, para todos los tipos de sitio;
- `auth_request` se convierte en autenticación delegada: el endpoint de autenticación se toma del `proxy_pass` de la location de la subpetición de autenticación (admite variables con `set`) y la redirección de inicio de sesión de `error_page 401 =302`, por lo que la configuración habitual de Authelia se convierte directamente; si solo está activado en algunas location, se convierte en un «Ámbito de autenticación» limitado a esas rutas;
- `auth_basic` se convierte en autenticación básica con usuarios compartidos; el archivo de usuarios de nginx no se lee, así que nadie podrá pasar hasta que añadas los usuarios;
- El reenvío TCP/UDP de `stream` se configura en [Reenvío de puertos](/es/docs/modules/portforward). Los certificados no se importan: si a un sitio HTTPS le falta el certificado, la vista previa muestra los archivos de certificado que usa nginx, y puedes añadirlos por ruta de archivo en la página de [Certificados](/es/docs/modules/ssl).

## Sustituir haproxy y nginx

El enrutamiento por protocolo y los servicios web juntos pueden sustituir la pareja haproxy + nginx. Una disposición habitual es una regla de [Enrutamiento por protocolo](/es/docs/modules/protoroute) en el puerto 443 que envía la VPN y otros servicios TLS directamente a sus backends y todo lo demás a un listener de servicios web que acepta el protocolo PROXY, más un listener de servicios web en el puerto 80 con un sitio de redirección (`https://{host}`).

Conversión de ajustes de nginx:

| nginx | Gately |
| --- | --- |
| `server` con `server_name` | un sitio con esos dominios |
| `location` con otro `proxy_pass` | backends por ruta |
| `auth_request` (Authelia) | autenticación delegada, o usuarios compartidos con TOTP |
| `proxy_ssl_name $host` | usar el dominio visitado como SNI |
| `return 301 https://$host$request_uri` | un sitio de redirección con `https://{host}` y «Conservar ruta y consulta» |
| `return 302 /admin` en `location = /` | una reescritura con «Redirección» activada y patrón `^/$` |
| `try_files $uri $uri.html $uri/index.html` y `error_page 404` | archivos estáticos con URL sin extensión y página 404 |
| `error_page 502 503 504 /50x.html` y `proxy_intercept_errors on` | la configuración de [páginas de error](#error-pages) del sitio e interceptar errores del backend |
| `add_header` | cabeceras de respuesta |
| `return 444` | un sitio Cerrar conexión |
| `error_page 497 https://$host:$server_port$request_uri` en un server HTTPS | «HTTP en el mismo puerto» de un listener nuevo en «Redirigir a HTTPS»; al fusionarse con un listener existente solo se muestra un aviso y su configuración no cambia |

## Consejo de despliegue

Los servicios web necesitan escuchar en puertos del host como el 80 / 443. En despliegues con Docker se recomienda `--network host`.
