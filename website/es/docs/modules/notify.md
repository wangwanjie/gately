# Centro de notificaciones

El Centro de notificaciones se suscribe al bus de eventos interno de Gately y, según tus reglas, envía eventos como cambios de DDNS, caducidad de certificados, bloqueos del WAF y fallos de procesos a Webhook, Telegram, Bark, ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, WeCom, DingTalk, Feishu y correo electrónico. La interfaz de administración está en el grupo «Sistema» de la barra lateral → «Notificaciones».

## Cómo funciona

1. Cada módulo publica eventos en el bus de eventos del proceso cuando cambia su estado. La publicación nunca bloquea la lógica de negocio.
2. Cuando el Centro de notificaciones recibe un evento, comprueba las reglas una a una: la regla está activada, el nivel del evento no es inferior al nivel mínimo de la regla y el tipo de evento coincide con la lista de eventos de la regla.
3. Cuando una regla coincidente supera la deduplicación por silenciamiento, el mensaje se entrega a los canales activados que la regla haya seleccionado. Si un evento coincide con varias reglas, cada canal recibe el envío una sola vez.
4. El resultado de cada entrega (éxito o motivo del fallo) se escribe en «Envíos recientes», que conserva en memoria las últimas 200 entradas. Los fallos también se escriben en el registro del módulo.

Una entrega individual agota el tiempo a los 45 segundos.

## Idioma de las notificaciones

El contenido de las notificaciones y los registros usan el idioma del servidor definido en «Ajustes del sistema». Cuando el idioma del servidor es «Seguir el sistema», se toma el idioma indicado por la variable de entorno `GATELY_LANG` y después por `LANG`. Los servicios en Docker, fnOS y OpenWrt normalmente no definen ninguna de las dos; en ese caso se sigue el idioma de la interfaz que el administrador usó en su último inicio de sesión o al cambiarlo en la barra superior, y si no hay ninguno se usa el inglés. Los inicios de sesión de cuentas de solo lectura no cambian este idioma. El «Origen» de una notificación se traduce cuando es un nombre generado por el programa (como «Panel de administración»); los nombres de tareas y otros nombres escritos por el usuario se mantienen tal cual.

## Nombre del servidor {#server-name}

Cada notificación incluye el nombre del servidor que la envió: el título tiene la forma `[Gately · fnOS-B760] Certificado a punto de caducar` y el cuerpo incluye la línea «Servidor: fnOS-B760», de modo que las notificaciones de varios servidores que llegan al mismo buzón o grupo se distinguen fácilmente. El nombre se define en **Nombre del servidor** dentro de **Configuración de notificaciones** en la cabecera de la página; si se deja vacío se usa el nombre de host de la máquina (el del host cuando Docker usa la red host), y si no se puede obtener el título sigue siendo `[Gately]`. El nombre admite hasta 64 caracteres y no puede contener caracteres de control como saltos de línea.

## Canales

Pulsa «Añadir canal», elige un tipo y rellena los campos. Todos los canales tienen un nombre (obligatorio), una nota y un interruptor de activación. Los campos secretos no se vuelven a mostrar tras guardar; si los dejas vacíos al editar, se conserva el valor original.

| Tipo | Campos obligatorios | Campos opcionales |
| --- | --- | --- |
| Webhook | URL de la petición | Método de la petición (por defecto GET; GET, POST, PUT o PATCH), cabeceras (un `Clave: Valor` por línea), cuerpo, contenido de coincidencia de éxito, tiempo de espera (segundos, por defecto 20), omitir la verificación del certificado |
| Telegram | Bot Token, Chat ID | URL de la API (déjala vacía para usar `https://api.telegram.org`) |
| Bark | Device Key | URL del servidor (déjala vacía para usar `https://api.day.app`) |
| WeCom | URL del bot | Ninguno |
| DingTalk | URL del bot | Secreto de firma (rellénalo cuando el bot tenga activada la «firma») |
| Feishu | URL del bot | Secreto de firma (rellénalo cuando el bot tenga activada la «verificación de firma») |
| Correo | Servidor, destinatarios y al menos uno entre remitente o usuario | Cifrado, puerto, usuario, contraseña |
| ntfy | Tema | URL del servidor (déjala vacía para usar `https://ntfy.sh`), token de acceso |
| Gotify | URL del servidor, token de la aplicación | Ninguno |
| Pushover | Token de la aplicación, User Key | Ninguno |
| Discord | URL del webhook | Ninguno |
| Slack | URL del webhook | Ninguno |
| ServerChan | SendKey | Ninguno |
| PushPlus | Token | Ninguno |

Los campos de URL requieren una dirección completa que empiece por `http://` o `https://`.

### Variables de plantilla del webhook

Las siguientes variables se pueden usar en la URL de la petición y en el cuerpo, escritas como el nombre de la variable entre llaves (también se acepta la forma con el prefijo `#`):

| Variable | Contenido |
| --- | --- |
| `title` | Título, con la forma `[Gately · nombre del servidor] Título del evento` |
| `server` | Nombre del servidor, consulte [Nombre del servidor](#server-name) |
| `message` | Descripción detallada del evento |
| `text` | Cuerpo completo, con descripción, origen, nivel y hora |
| `type` | Tipo de evento, como `ddns.changed` |
| `source` | Origen del evento |
| `level` | Nivel: `info`, `warn` o `error` |
| `time` | Hora en formato RFC 3339 |

Los campos adicionales que lleva un evento (como `ip` y `container`) también se pueden usar como variables. Si un nombre coincide con los de la tabla anterior, prevalece la tabla anterior. Los valores de las variables en la URL se escapan para URL.

### Correo

| Cifrado | Valor | Puerto por defecto |
| --- | --- | --- |
| STARTTLS | `starttls` | 587 |
| SSL/TLS | `tls` | 465 |
| Ninguno | `none` | 25 |

Un puerto 0 usa el puerto por defecto del cifrado elegido. Puedes introducir varios destinatarios separados por comas. Los correos se envían como texto plano UTF-8. Se usa la autenticación PLAIN cuando el servidor admite AUTH y se ha definido un usuario; cuando el remitente está vacío, se usa el usuario.

### Firmas y códigos de respuesta

- DingTalk: una vez definido un secreto de firma, las peticiones llevan automáticamente `timestamp` y `sign`.
- Feishu: una vez definido un secreto de firma, el cuerpo de la petición lleva automáticamente `timestamp` y `sign`.
- WeCom, DingTalk, Feishu y Bark comprueban el código de retorno de negocio de la respuesta. Un HTTP 200 con un fallo de negocio también se registra como una entrega fallida.

### Proxy {#proxy}

Cuando el servicio de notificación no es accesible directamente y solo se llega a él mediante un proxy (por ejemplo Telegram, Discord o Slack), las notificaciones pueden salir por un proxy:

- **Proxy global**: en «Ajustes de notificación», en la cabecera de la página, se define el proxy que usan por defecto todos los canales: «Sin proxy», «Usar el proxy global de actualizaciones» (reutiliza el proxy de comprobación de actualizaciones de los [ajustes del sistema](/es/docs/guide/configuration); el cambio se aplica al instante y, si ese proxy está vacío, equivale a no usar proxy) o «Proxy personalizado».
- **Proxy por canal**: el campo «Proxy» de la ventana de edición de cada canal tiene por defecto «Según los ajustes de notificación»; también se puede elegir «Directo (sin proxy)», «Usar el proxy global de actualizaciones» o un proxy personalizado, que sustituye al ajuste global. Los servicios autoalojados en la LAN, como ntfy o Gotify, pueden ir en directo.
- Las direcciones de proxy admiten `http://`, `https://`, `socks5://` y `socks5h://`, con usuario y contraseña opcionales, por ejemplo `http://user:pass@127.0.0.1:7890`; las tarjetas de canal ocultan la contraseña al mostrar el proxy.
- Con «Sin proxy», los canales HTTP respetan variables de entorno como `HTTPS_PROXY` y `NO_PROXY` y el correo se conecta directamente, igual que antes; «Directo» ignora además las variables de entorno.
- El correo también admite proxy: con un proxy `http(s)://` se abre un túnel con el método CONNECT hasta el puerto del servidor de correo (el proxy debe permitir conectar a ese puerto; los proxies habituales como Clash lo permiten por defecto), y con un proxy `socks5://` es el proxy quien resuelve el dominio del servidor. STARTTLS / SSL y la autenticación se completan después dentro del túnel, de modo que el proxy no ve el contenido del correo.
- Si falla la conexión con el proxy, el registro de envíos y el mensaje de la prueba muestran «Falló la conexión a través del proxy …», con la contraseña del proxy sustituida por `xxxxx`.

## Reglas

Pulsa «Añadir regla»:

| Campo | Descripción |
| --- | --- |
| Nombre | Obligatorio |
| Eventos | Al menos uno. Elige tipos de evento concretos, `*` para todos los eventos, o introduce un comodín como `ddns.*`. Los comodines usan la sintaxis `path.Match` de Go |
| Nivel mínimo | `info`, `warn` o `error`; vacío significa info. Los eventos por debajo de este nivel no se envían |
| Canales | Al menos uno |
| Silencio (s) | Dentro de una misma regla, los eventos del mismo tipo y del mismo origen se envían una sola vez en este periodo y las repeticiones se descartan. 0 desactiva la deduplicación |
| Activada | Interruptor |

::: tip
Los bloqueos del WAF y los cambios de estado de los backends pueden ocurrir con mucha frecuencia. En las reglas que los cubran, define un tiempo de silenciamiento de 300 segundos o más.
:::

## Plantillas de reglas

Al crear una regla, puedes pulsar una plantilla para rellenar los eventos, el nivel y el tiempo de silenciamiento, y luego elegir los canales y guardar:

| Plantilla | Eventos |
| --- | --- |
| Fallos críticos | Fallo de actualización de DDNS, fallo de renovación de certificado, backend no disponible, salida anómala de un subproceso, fallo de copia de seguridad, salida de contenedor |
| Caducidad de certificados | Certificado a punto de caducar, fallo de renovación (silenciamiento de 24 horas) |
| Cambios de dirección | Actualización de registro DDNS, cambio de dirección STUN |
| Estado de servicios | Backend no disponible y recuperado, salida de contenedor, salida de subproceso |
| Seguridad e inicios de sesión | Inicio de sesión fallido, IP bloqueada, inicio de sesión correcto, acceso a contenido sensible |
| Alertas de rendimiento | Activación y recuperación de alertas de rendimiento (sin silencio) |
| Cambios de configuración | Cada cambio de configuración hecho en la interfaz de administración |
| Actualizaciones | Actualizaciones de seguridad y nuevas versiones de Gately, nuevas versiones de programas gestionados e imágenes de contenedores (sin silencio) |

## Tipos de evento

| Tipo de evento | Nombre | Nivel | Desencadenante |
| --- | --- | --- | --- |
| `ddns.changed` | DNS dinámico: registro actualizado | Info | Una tarea DDNS actualizó su registro correctamente |
| `ddns.failed` | DNS dinámico: fallo de actualización | Error | Una tarea DDNS no pudo actualizarse |
| `stun.changed` | STUN: dirección de traversal modificada | Info | Cambió la dirección de traversal de una regla STUN |
| `ssl.expiring` | Certificado: caduca pronto | Advertencia | Quedan menos de 7 días de validez; cada certificado se recuerda como máximo una vez cada 24 horas |
| `ssl.renew_failed` | Certificado: fallo de renovación | Error | Falló la emisión o la renovación de un certificado |
| `waf.blocked` | WAF: petición bloqueada | Advertencia | El WAF bloqueó una petición, con los campos `ip`, `host` y `path` |
| `upstream.down` | Proxy inverso: backend caído | Error | Un backend de un servicio web se consideró no disponible |
| `upstream.up` | Proxy inverso: backend recuperado | Info | Un backend no disponible se recuperó |
| `docker.exited` | Docker: contenedor terminado | Advertencia o Error | Un contenedor terminó con un código de salida distinto de cero (advertencia) o fue terminado por quedarse sin memoria (error) |
| `process.exited` | Subproceso terminado de forma anómala | Error | Un subproceso gestionado por Gately terminó de forma anómala y se reiniciará automáticamente; el origen es el nombre de registro de ese proceso |
| `backup.done` | Copia de seguridad: completada | Info | Una copia programada o manual se realizó correctamente en todos los destinos; consulta [Copia de seguridad de la configuración](/es/docs/modules/backup) |
| `backup.failed` | Copia de seguridad: fallida | Error | Falló el empaquetado de la copia o la escritura en algún destino |
| `auth.failed` | Autenticación: fallos repetidos | Advertencia | Falló un inicio de sesión en la interfaz de administración, con el campo `ip` |
| `auth.banned` | Autenticación: IP bloqueada | Advertencia | Los fallos repetidos de autenticación de un servicio web activaron un bloqueo automático |
| `knock.opened` | Port knocking: knock correcto | Info | Un knock superó la autenticación y se permitió la dirección de origen, con los campos `ip`, `rule` y `ports`; consulta [Port Knocking](/es/docs/modules/knock) |
| `knock.failed` | Port knocking: knock rechazado | Advertencia | Un toque fue rechazado: un paquete UDP pasó la autenticación HMAC pero la regla lo rechazó (repetición, marca de tiempo caducada, puerto no permitido, usuario incorrecto, etc.), o un paquete enviado por una conexión TCP del enrutamiento de protocolos no superó la autenticación o las comprobaciones. Incluye los campos `ip`, `reason` y `via` (`udp` o `tcp`); se registra como máximo una vez cada 5 segundos por origen y la prevención de intrusiones puede contarlo para bloquear. Las direcciones de origen UDP se pueden falsificar, así que los paquetes UDP que no superan la autenticación HMAC solo se cuentan y no publican este evento |
| `login.success` | Interfaz de administración: inicio de sesión correcto | Info | Un inicio de sesión en la interfaz de administración se realizó correctamente, con los campos `ip` y `user` |
| `audit.change` | Interfaz de administración: cambio de configuración | Info | Cualquier operación de cambio en la interfaz de administración, con los campos `actor`, `ip`, `method` y `path`; consulta [Auditoría e historial de configuración](/es/docs/guide/audit) |
| `audit.access` | Interfaz de administración: acceso a contenido sensible | Advertencia | Descarga de copias de seguridad, paquetes de diagnóstico o claves privadas de certificados, apertura de terminales, etc.; mismos campos que el anterior |
| `perf.alert` | Rendimiento: alerta activada | Aviso o error | La condición de una regla de alerta de rendimiento se ha mantenido durante su duración; el nivel lo fija la regla, con los campos `rule`, `metric`, `target`, `value` y `condition`; consulta [Rendimiento](/es/docs/modules/perf) |
| `perf.recovered` | Rendimiento: alerta recuperada | Información | La condición de una alerta de rendimiento activa ha dejado de cumplirse; mismos campos |
| `system.update` | Gately: nueva versión disponible | Información | Una comprobación automática o manual encontró una versión nueva; campos `current`, `latest`, `url`; cada versión se notifica una vez, consulta [Comprobación automática de actualizaciones](/es/docs/guide/install#update-check) |
| `system.security` | Gately: actualización de seguridad | Error | La versión actual está afectada por un aviso de seguridad publicado, o una versión más reciente incluye correcciones de seguridad; campos `current`, `latest`, `url`, `advisories`, `securityReleases`; se vuelve a notificar si aparece un problema nuevo |
| `program.update` | Programas gestionados: nueva versión disponible | Información | La comprobación automática encontró una versión nueva de frp, cloudflared, Tailscale, NetBird o File Browser instalados; el origen es el nombre del programa; campos `program`, `current`, `latest` |
| `docker.update` | Docker: imagen más reciente disponible | Información | La comprobación programada de Docker encontró imágenes más recientes; un aviso por comprobación y una sola vez por imagen nueva; campo `count` |
| `notify.test` | Notificación de prueba | Info | Generada por el botón «Probar» del canal; no participa en la coincidencia con las reglas |

## Enviar una prueba

El botón «Probar» de la tarjeta de un canal envía un mensaje titulado «Notificación de prueba» directamente a ese canal. Se salta las reglas y funciona incluso si el canal no está activado. Si se completa, se muestra «Notificación de prueba enviada»; si falla, se muestra el error concreto. Los resultados de las pruebas también aparecen en «Envíos recientes», con «Prueba» en la columna de la regla.

La API correspondiente es `POST /api/notify/channels/{key}/test`, a la que se puede llamar con el alcance `notify:write` de un [token de API](/es/docs/reference/api).
