# Servicios web

Sirve proxy inverso, redirecciones, archivos estáticos y texto fijo por dominio. Es el punto de entrada web unificado de Gately. Ubicación en el menú: barra lateral «Web → Servicios web».

## Estructura: listeners, sitios y sitio por defecto

La configuración tiene tres niveles:

1. Listener: un puerto de escucha. Sus campos incluyen la red (`tcp` doble pila, `tcp4`, `tcp6`), la dirección de escucha (vacía para todas las direcciones), el puerto, el interruptor de HTTPS, la versión mínima de TLS (1.0 / 1.1 / 1.2 / 1.3) y si acepta el protocolo PROXY.
2. Sitio: un sitio dentro de un listener, que se selecciona por dominio. Los dominios pueden ser un dominio exacto, `*.example.com` (coincide con subdominios de cualquier profundidad) o `*` (coincide con todo). La prioridad de coincidencia es dominio exacto, comodín y después `*`.
3. Sitio por defecto: se usa cuando ningún sitio coincide. Si no está activado, se devuelve un 404. El sitio por defecto también puede ser de tipo «Cerrar conexión», para que las peticiones a dominios desconocidos no reciban ninguna respuesta. Una vez configurado, el sitio por defecto aparece al final de la lista de sitios; si aún no está configurado, haz clic en «Configurar sitio por defecto» en la tarjeta del listener para configurarlo.

Al guardar un cambio en un sitio o en el sitio por defecto, la lista de sitios se sustituye en caliente: el listener sigue funcionando, las conexiones abiertas (incluido WebSocket) se mantienen y cada sitio conserva sus estadísticas. Si la nueva configuración no es válida, los sitios anteriores siguen sirviendo. Cambiar los ajustes propios del listener (red, dirección, puerto, HTTPS, TLS mínimo, HTTP/3, protocolo PROXY) reinicia ese listener.

Un sitio puede tener un «Grupo» (hasta 64 caracteres; el sitio por defecto no se puede agrupar). Bajo cada listener, la lista muestra los grupos como secciones plegables: los grupos aparecen en el orden en que se encuentran por primera vez, los sitios sin grupo van a continuación y el sitio por defecto queda siempre al final. El estado plegado de cada grupo se guarda en el navegador. Pulsa «Ordenar» para entrar en el modo de ordenación, donde puedes subir y bajar sitios, arrastrarlos dentro de su grupo, mover grupos enteros y reordenar los listeners. El orden solo afecta a la lista: la coincidencia de dominios sigue la prioridad anterior sea cual sea el orden de los sitios, y al guardar el nuevo orden la lista de sitios se sustituye en caliente sin cortar conexiones. El cuadro de búsqueda de la parte superior de la página filtra los sitios por dominio, nombre, grupo, backend o nota.

En el modo de orden también puede marcar varios sitios (la casilla del encabezado de un grupo selecciona el grupo entero). Al arrastrar cualquier sitio marcado se mueven todos, y al soltarlos sobre un sitio o el encabezado de otro grupo pasan a ese grupo. La barra de herramientas mueve de una vez los sitios marcados a un grupo, o los saca de su grupo si deja el nombre vacío; los sitios marcados de distintos listeners se guardan por listener.

Pase el ratón por el encabezado de un grupo y pulse el lápiz para renombrarlo: todos sus elementos pasan al nuevo nombre y conservan su posición y orden. Si elige un nombre ya usado, tras confirmar se fusionan los dos grupos.

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

En un sitio, marca «Usar usuarios compartidos» junto a la autenticación web o básica. Los usuarios propios del sitio siguen funcionando también. También puedes marcar sitios en bloque en la lista de sitios de la página Usuarios de autenticación, agrupada por escucha y con filtro: al marcar un sitio sin autenticación también se activa la autenticación web; un sitio que solo se autentica con usuarios compartidos no se puede desmarcar ahí, para que nunca quede público por error (si quieres hacerlo público, desactiva la autenticación en la configuración del sitio); los sitios con autenticación delegada no pueden usar usuarios compartidos. Los cambios de usuarios, OIDC y sitios de la página se aplican juntos al pulsar Guardar abajo; si alguno no pasa la validación no se guarda nada, y salir de la página con cambios sin guardar pide confirmación.

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

Cerrar la conexión, como tipo de sitio o como acción al denegar, solo evita la respuesta HTTP y no oculta que el servicio está activo:

- el handshake TCP ya se completó, así que el cliente sabe que el puerto escucha;
- HTTP/1.x cierra la conexión, mientras que HTTP/2 y HTTP/3 solo reinician la solicitud y mantienen la conexión abierta para reutilizarla;
- en HTTPS, los dominios sin certificado ya fallan durante el handshake TLS, así que las solicitudes que llegan al sitio por defecto ya recibieron el certificado y los dominios que contiene.

Para que los escáneres solo vean tiempos de espera después, bloquea sus direcciones en [Prevención de intrusiones](/es/docs/modules/guard); con la sincronización del cortafuegos activada, su tráfico se descarta en el kernel.

### Listas de IP suscritas

Añade listas en «Ajustes globales → Listas de IP suscritas»: indica la URL de descarga y el intervalo de actualización (24 horas por defecto). Gately las descarga periódicamente y las guarda en caché en el directorio de configuración, de modo que tras un reinicio se pueden usar sin conexión a Internet. El contenido admite una IP, un CIDR o un rango `inicio-fin` por línea; lo que va detrás de `#`, `;` o `//` es un comentario. Por ejemplo, para los rangos de IP de provincias y ciudades chinas puedes usar `https://metowolf.github.io/iplist/data/cncity/440000.txt` (provincia de Guangdong) de [metowolf/iplist](https://github.com/metowolf/iplist).

- Las listas de IP permitidas y bloqueadas de los sitios y de las reglas por ruta pueden hacer referencia a varias listas, que se comprueban junto con los rangos escritos a mano; coincidir con cualquiera cuenta como estar en la lista;
- Si el contenido descargado no se puede analizar (por ejemplo, se recibe una página de error), se conserva la lista anterior, se muestra el motivo en la lista y se reintenta al cabo de una hora;
- Hasta que la lista se descarga correctamente cuenta como vacía: una lista de IP permitidas deniega por tanto el acceso y una lista de IP bloqueadas no bloquea nada adicional;
- Una lista a la que todavía hace referencia algún sitio no se puede eliminar.

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

La lista de bloqueos de «Ajustes globales» muestra las IP bloqueadas actualmente, el motivo (fallos de autenticación repetidos o bloqueo manual) y la hora de expiración, y cada entrada se puede levantar con «Desbloquear». También puedes bloquear manualmente: introduce una IP y un número de horas (de 1 a 720, es decir, 30 días como máximo); la API usa 24 horas por defecto.

La lista de bloqueos se comparte con [Prevención de intrusiones](/es/docs/modules/guard): con el módulo activado, aquí solo se muestra el número de bloqueos y un enlace a su página para gestionarlos. Los bloqueos también afectan al reenvío de puertos, STUN y otros servicios, y se descartan en el cortafuegos del sistema si la sincronización está activada; con «Ignorar direcciones de la LAN» activado, esas direcciones nunca se bloquean automáticamente.

### Filtro de países con GeoIP

El «Filtro de países» de un sitio tiene tres modos: desactivado, permitir solo y denegar. Introduce códigos de país ISO de dos letras separados por comas, como `CN, HK, US`. Cuando está activado se requiere al menos un país.

- El filtrado depende de una base de datos GeoIP, que debes importar antes en «Ajustes globales → Base de datos GeoIP»: sube un archivo `.mmdb` o introduce una URL de descarga. Se admiten MaxMind GeoLite2, DB-IP y otras bases de datos de países o ciudades; el límite de subida es de 129 MB;
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
- Cuando «Pasar nombre de usuario» contiene un nombre de cabecera como `X-Remote-User`, el nombre del usuario con sesión iniciada se envía al backend en esa cabecera. La misma cabecera enviada por los visitantes se elimina siempre, así que no se puede falsificar;
- Con la caché de respuestas en memoria activada, la clave de caché incluye la regla de usuario elegida, y también el nombre de usuario cuando «Pasar nombre de usuario» está activo, de modo que las respuestas de backends distintos o para usuarios distintos se mantienen separadas.

Al guardar se comprueba que el sitio tiene la autenticación activada, que cada regla tiene al menos un usuario y backends válidos, que ningún usuario aparece en dos reglas y que el nombre de cabecera es válido y no es una de las cabeceras que afectan al reenvío o a la autenticación, como `Host`, `Cookie`, `Authorization` o `X-Forwarded-For`. En la lista de sitios, un sitio con reglas por usuario muestra una etiqueta con el número de reglas.

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
- La clave de caché incluye el Host, la URI completa de la petición y `Accept-Encoding`; con el [reparto por usuario](#user-routes), incluye también la regla de usuario elegida, y el nombre de usuario cuando «Pasar nombre de usuario» está activo.

### Reescritura y redirección de rutas

Las reglas se evalúan en orden y la evaluación se detiene en la primera coincidencia.

| Campo | Descripción |
| --- | --- |
| `pattern` | Una expresión regular que se compara con la ruta de la petición, como `^/old/(.*)$` |
| `to` | El resultado de la sustitución; usa `$1` para referirte a los grupos |
| `redirect` | Desactivado para una reescritura interna (la ruta reescrita se envía al backend y el destino debe empezar por `/`); activado para una redirección |
| `code` | Código de estado de la redirección 301 / 302 / 307 / 308, por defecto 302 |

Las redirecciones conservan los parámetros de consulta originales. El destino de una redirección puede contener `{host}`, el dominio que usó el visitante. Las reglas de redirección se ejecutan antes de la autenticación, como `location = / { return 302 ...; }` de nginx, de modo que una ruta como `/` puede enviar a los visitantes a otro sitio sin pedirles que inicien sesión.

## Estadísticas de acceso y monitorización

### Estadísticas del sitio

En un listener en ejecución, pulsa «Estadísticas» en la fila de un sitio para abrir la ventana de estadísticas, donde puedes elegir la última hora, las últimas 6 horas o las últimas 24 horas:

- Estadísticas de acceso por minuto: un punto por minuto, conservadas durante las últimas 24 horas, con una curva de número de peticiones y curvas de errores 4xx / 5xx;
- Distribución de códigos de estado (2xx / 3xx / 4xx / 5xx) y tráfico de respuesta acumulado;
- Las 10 IP de origen y los 10 países principales (las estadísticas por país requieren una base de datos GeoIP importada);
- Los sitios de proxy inverso muestran el estado de salud, el peso, la marca de reserva y el número actual de conexiones de cada backend.

Las estadísticas se guardan en memoria y vuelven a empezar a contarse tras un reinicio o tras cambiar los ajustes propios del listener. Editar un sitio conserva sus contadores. Las clasificaciones siguen como máximo 5000 orígenes distintos.

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
| `gately_upstream_up` | gauge | Si un backend está saludable: 1 saludable, 0 no disponible (con la etiqueta `upstream`) |

Todas las métricas llevan las etiquetas `listener` (nombre del listener#clave del listener) y `site` (nombre del sitio o, si no tiene, el primer dominio).

## Importar configuración de nginx {#import-nginx}

El botón «Importar nginx» de la parte superior de la página convierte sitios de nginx existentes en listeners y sitios de Gately. Se recomienda ejecutar `nginx -T` en el host de nginx y pegar la salida completa: incluye todos los archivos de include, y Gately los expande igual que nginx.

- Primero la vista previa: agrupados por puerto de escucha, se muestran el tipo de cada sitio, su backend o directorio, el archivo de origen y el número de línea, y los puntos a revisar (directivas que usan variables, location con expresiones regulares, dominios sin certificado, etc.); para los sitios que no se pueden convertir se indica el motivo;
- Los sitios de un mismo puerto se añaden al listener existente o, si no hay ninguno, se crea un listener nuevo; los dominios que ya existen se omiten, así que se puede importar varias veces;
- Por defecto se importan desactivados, para revisarlos uno a uno en paralelo con nginx antes de activarlos; con «Activar tras importar» marcado se aplican de inmediato (si nginx sigue ocupando el mismo puerto, el listener nuevo no podrá iniciarse);
- Se convierten: `proxy_pass` (incluidos el peso, la reserva y la política de balanceo de carga de upstream), directorios estáticos `root` / `alias` (`autoindex`, `try_files $uri.html`, `error_page 404`), redirecciones y contenido fijo con `return`, redirecciones `rewrite` de todo el sitio, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, backends por ruta, control de acceso y redirecciones a nivel de location, y `listen ... proxy_protocol` y `quic`;
- `auth_request` se convierte en autenticación delegada: el endpoint de autenticación se toma del `proxy_pass` de la location de la subpetición de autenticación (admite variables con `set`) y la redirección de inicio de sesión de `error_page 401 =302`, por lo que la configuración habitual de Authelia se convierte directamente; si solo está activado en algunas location, se convierte en un «Ámbito de autenticación» limitado a esas rutas;
- `auth_basic` se convierte en autenticación básica con usuarios compartidos; el archivo de usuarios de nginx no se lee, así que nadie podrá pasar hasta que añadas los usuarios;
- El reenvío TCP/UDP de `stream` se configura en [Reenvío de puertos](/es/docs/modules/portforward). Los certificados no se importan: si a un sitio HTTPS le falta el certificado, la vista previa muestra los archivos de certificado que usa nginx, y puedes añadirlos por ruta de archivo en la página de [Certificados](/es/docs/modules/ssl).

## Sustituir haproxy y nginx

El reenvío de puertos y los servicios web juntos pueden sustituir la pareja haproxy + nginx. Una disposición habitual es una regla de [Enrutamiento por protocolo](/es/docs/modules/protoroute) en el puerto 443 que envía la VPN y otros servicios TLS directamente a sus backends y todo lo demás a un listener de servicios web que acepta el protocolo PROXY, más un listener de servicios web en el puerto 80 con un sitio de redirección (`https://{host}`).

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
| `add_header` | cabeceras de respuesta |
| `return 444` | un sitio Cerrar conexión |
| `error_page 497 https://$host:$server_port$request_uri` en un server HTTPS | «HTTP en el mismo puerto» de un listener nuevo en «Redirigir a HTTPS»; al fusionarse con un listener existente solo se muestra un aviso y su configuración no cambia |

## Consejo de despliegue

Los servicios web necesitan escuchar en puertos del host como el 80 / 443. En despliegues con Docker se recomienda `--network host`.
