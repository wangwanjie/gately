# Archivos y herramientas de operación

## WebDAV

Se configura en «Servicio WebDAV», dentro del grupo «Servicios de archivos» de la barra lateral.

| Campo | Por defecto | Descripción |
| --- | --- | --- |
| `enable` | Desactivado | Activa el servicio; cuando está activado se requiere al menos un usuario |
| `listenIp` | Vacío | Dirección de escucha; déjalo vacío para escuchar en todas las direcciones |
| `port` | 18800 | Puerto de escucha |
| `tls` | Desactivado | Activa HTTPS; el certificado lo proporciona por SNI el módulo [Certificados SSL](/es/docs/modules/ssl) |
| `users` | Ninguno | Varios usuarios. Cada uno tiene `name`, `password`, `root` (una ruta de directorio absoluta) y `readOnly`, o usa `mounts` para montar varios directorios |
| `sharedUsers` | Desactivado | Permite que los [usuarios compartidos](#shared-users) inicien sesión en este servicio |

Cada usuario puede apuntar a un directorio o montar varios directorios:

- **Un solo directorio**: define `root` y `readOnly`. Tras iniciar sesión, la raíz es ese directorio.
- **Varios directorios**: pulsa «Montar varios directorios» en la interfaz, o rellena varias entradas en `mounts`. Cada entrada tiene `name` (nombre visible, no puede contener `/` y es único dentro de un mismo usuario), `path` (una ruta absoluta) y `readOnly`. Tras iniciar sesión, la raíz lista los directorios por nombre. La propia raíz no admite escritura y los archivos no se pueden mover entre directorios distintos.

## FTP

Se configura en «Servicio FTP», dentro del grupo «Servicios de archivos». Se admiten FTP y FTPS.

| Campo | Por defecto | Descripción |
| --- | --- | --- |
| `enable` | Desactivado | Activa el servicio; cuando está activado se requiere al menos un usuario |
| `listenIp` | Vacío | Dirección de escucha; déjalo vacío para escuchar en todas las direcciones |
| `port` | 2121 | Puerto de escucha |
| `passiveStart` / `passiveEnd` | 0 | Rango de puertos de datos del modo pasivo. Cuando ambos son 0, el sistema asigna los puertos al azar; cuando se definen, debe cumplirse 1 ≤ start ≤ end ≤ 65535, y el rango debe estar permitido en el cortafuegos y en los mapeos de puertos |
| `publicHost` | Vacío | IP que se anuncia al exterior en modo pasivo. Detrás de un NAT, introduce la IP pública o reenviada. Solo se admiten direcciones IP; déjalo vacío para usar automáticamente la dirección de la conexión |
| `tlsMode` | 0 | 0 permite tanto texto plano como FTPS, 1 exige FTPS explícito (`AUTH TLS`), 2 es FTPS implícito (cifrado desde la conexión). Cuando no es 0, el certificado lo proporciona por SNI el módulo [Certificados SSL](/es/docs/modules/ssl) |
| `idleTimeout` | 900 | Segundos de inactividad antes de cerrar una conexión |
| `connectionTimeout` | 30 | Tiempo de espera, en segundos, para establecer una conexión de datos |
| `disableActiveMode` | Desactivado | Usa solo el modo pasivo, más adecuado para entornos con NAT |
| `users` | Ninguno | Varios usuarios, con los mismos campos que WebDAV: `name`, `password`, `root`, `readOnly` o `mounts`; los nombres de usuario deben ser únicos |
| `sharedUsers` | Desactivado | Permite que los [usuarios compartidos](#shared-users) inicien sesión en este servicio |

## SMB

Se configura en «Servicio SMB», dentro del grupo «Servicios de archivos», y comparte directorios locales con el Explorador de Windows, el Finder de macOS, Linux y los clientes SMB de móviles y televisores. La implementación del protocolo viene integrada en Gately (basada en [gosamba](https://github.com/ahmetozer/gosamba)) y admite SMB 2.0.2 a 3.1.1, autenticación NTLMv2 y firma, con cifrado SMB 3 obligatorio opcional; SMB 1 no se admite. Solo está disponible en Linux y macOS.

| Campo | Por defecto | Descripción |
| --- | --- | --- |
| `enable` | Desactivado | Activa el servicio; cuando está activado se requiere al menos un usuario |
| `listenIp` | Vacío | Dirección de escucha; déjalo vacío para escuchar en todas las direcciones |
| `port` | 445 | Puerto de escucha. Windows solo se conecta al 445; si el Samba incluido en un NAS como fnOS o Synology ya usa el 445, elige otro puerto, que macOS, Linux y la mayoría de reproductores pueden indicar en la dirección |
| `requireEncryption` | Desactivado | Exige cifrado SMB 3 en todas las conexiones; los dispositivos antiguos y algunos televisores que solo admiten SMB 2 no podrán conectarse |
| `mdns` | Desactivado | Anuncia el servicio por Bonjour para que aparezca en Red del Finder de macOS; en Docker solo funciona con red host |
| `users` | Ninguno | Varios usuarios con los mismos campos que WebDAV: `name`, `password`, `root`, `readOnly` o `mounts`; los nombres de usuario no distinguen mayúsculas y no pueden repetirse |
| `sharedUsers` | Desactivado | Permite que los [usuarios compartidos](#shared-users) inicien sesión en este servicio |

Los recursos compartidos pertenecen a cada usuario, y un usuario conectado solo ve y abre los suyos:

- **Un solo directorio**: el recurso compartido lleva el nombre del usuario, por ejemplo `\\192.168.1.2\alice` para el usuario `alice`.
- **Varios directorios**: cada directorio es un recurso compartido cuyo nombre es `name` (sin distinguir mayúsculas y único por usuario), y cada uno puede ser de solo lectura. Usuarios distintos pueden tener recursos con el mismo nombre, cada uno apuntando a su propio directorio.

Los nombres de usuario y de recurso no pueden contener `" / \ [ ] : ; | = , + * ? < >` ni empezar o terminar en `.`. La autenticación NTLM necesita la contraseña en texto plano, así que los usuarios cuya contraseña esté guardada como hash bcrypt deben volver a establecerla. Los archivos se leen y escriben con la identidad del proceso de Gately.

«Direcciones de conexión», al final de la página, muestra la dirección de cada recurso según la configuración guardada:

- Windows: escribe `\\host\recurso` en la barra de direcciones del Explorador o ejecuta `net use Z: \\host\recurso /user:usuario`
- macOS: en el Finder elige Ir → Conectarse al servidor e introduce `smb://host:puerto/recurso` (el puerto 445 puede omitirse)
- Linux: `mount -t cifs //host/recurso /mnt -o username=usuario,port=puerto`

Para diagnosticar clientes que no conectan o fallan al autenticarse, define la variable de entorno `GATELY_SMB_DEBUG=1` y reinicia Gately; los registros de depuración del protocolo se escriben en el registro de ejecución. Quita la variable al terminar.

## Usuarios compartidos {#shared-users}

WebDAV, FTP y SMB comparten una lista de «usuarios compartidos» con los mismos campos que los usuarios locales de cada servicio (nombre, contraseña, `root` y `readOnly`, o `mounts`), guardada en `fileusers.json` en el directorio de configuración. Se gestionan en la página «Usuarios compartidos de archivos» del grupo «Servicios de archivos» de la barra lateral y, al guardar, se aplican a los tres servicios; los usuarios locales de cada servicio no cambian. La parte superior de la página muestra si cada servicio permite usuarios compartidos y los usuarios compartidos con el mismo nombre que un usuario local o que SMB no puede usar.

- Cada servicio decide por su cuenta si acepta usuarios compartidos: active «Permitir usuarios compartidos» en la tarjeta «Usuarios compartidos» de la página del servicio (campo `sharedUsers`) y guarde, y los usuarios compartidos podrán iniciar sesión en ese servicio; la tarjeta también muestra los usuarios compartidos guardados y «Gestionar usuarios compartidos» abre la página «Usuarios compartidos de archivos»;
- Si un usuario local tiene el mismo nombre, solo se comprueba el usuario local (sin distinguir mayúsculas en SMB);
- Los nombres de usuarios compartidos no se pueden repetir aunque cambien las mayúsculas y no pueden contener dos puntos;
- Los cambios se aplican al momento: WebDAV usa la nueva configuración en la siguiente petición, FTP y SMB se reinician y los clientes conectados deben volver a conectarse;
- Con el interruptor activado y al menos un usuario compartido, el servicio se puede activar sin usuarios locales;
- Un servicio activado solo con usuarios compartidos debe conservar tras el cambio un usuario compartido que pueda iniciar sesión (en SMB, uno que SMB pueda usar); si no, el cambio se rechaza;
- Si `fileusers.json` se estropea a mano (por ejemplo, una contraseña vacía o un directorio que no es ruta absoluta), ningún usuario compartido puede iniciar sesión y la página lo avisa; corríjalo y guarde para restaurarlos;
- SMB solo acepta contraseñas en texto plano y los nombres de usuario y de recurso deben cumplir las reglas de caracteres de SMB; los usuarios compartidos que no las cumplen se indican en la página «Usuarios compartidos de archivos», en la página de SMB y en el registro de ejecución y se omiten, mientras que WebDAV y FTP pueden seguir usándolos.

La API es `GET` / `PUT /api/fileusers`, que requiere el permiso `fileusers:read` / `fileusers:write` para los tokens de API; `services` en la respuesta muestra cómo los usa cada servicio. Los anteriores `GET` / `PUT /api/<servicio>/shared-users` de cada servicio están obsoletos pero siguen funcionando y leen y escriben la misma lista: la lectura usa el permiso de lectura de ese servicio y un cambio necesita `webdav:write`, `ftp:write` y `smb:write` a la vez.

**Cambio de contraseña por el usuario**: activa Cambio de contraseña por el usuario en la página Usuarios compartidos de archivos y guarda; los usuarios compartidos podrán cambiar su contraseña en `https://dominio-del-sitio/__gately_auth/password` de cualquier sitio del servicio web con autenticación web, indicando el nombre de usuario, la contraseña actual y la nueva (de 8 a 128 caracteres). WebDAV, FTP y SMB usan la nueva contraseña de inmediato (FTP y SMB se reinician, así que los clientes conectados deben reconectarse). Las contraseñas en texto plano siguen en texto plano y los hashes bcrypt siguen como hash; una contraseña actual incorrecta cuenta para el mismo límite de intentos y bloqueo automático que el inicio de sesión web. El interruptor se guarda como `selfService` en `fileusers.json`, con el mismo nombre de campo en la API. Para los usuarios compartidos de inicio de sesión web, consulta [cambio de contraseña en el servicio web](/es/docs/modules/web#self-service-password).

## File Browser {#filebrowser}

Se configura en «File Browser», dentro del grupo «Servicios de archivos». Gately aloja [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser), un gestor de archivos web que previsualiza imágenes, vídeos y documentos, sube y descarga archivos, crea enlaces para compartir y tiene sus propios usuarios y permisos. El filebrowser/filebrowser original se archivó en 2026 y ya no recibe correcciones de seguridad, por eso Gately aloja la bifurcación Quantum, que sigue mantenida.

El programa no viene incluido en Gately. La tarjeta «Instalación en línea» al pie de la página descarga FileBrowser Quantum de GitHub en `bin/filebrowser` dentro del directorio de configuración y permite actualizar, instalar una versión concreta, usar un proxy de descarga propio, revertir y desinstalar; tras actualizar o revertir, las instancias activas se reinician. Consulta [Instalación, actualización y reversión en línea](/es/docs/modules/tunnels#install). El botón «Restaurar valores predeterminados» de la cabecera borra todas las instancias y sus datos; consulta [Restaurar valores predeterminados](/es/docs/modules/tunnels#reset). También puedes colocar el programa tú mismo e indicar su ruta. El programa se busca en este orden: la ruta indicada, `bin/` en el directorio de configuración y `PATH`; la página avisa si el programa encontrado no es Quantum (por ejemplo, el original en `PATH`).

Cada instancia es un proceso de File Browser independiente:

| Campo | Predeterminado | Descripción |
| --- | --- | --- |
| `listenIp` | vacío | Dirección de escucha; vacío escucha en todas las direcciones IPv4. Usa `127.0.0.1` si solo se accede a través de un proxy inverso del Servicio web |
| `port` | 8080 | Puerto de escucha |
| `baseUrl` | `/` | Ruta base; si usas un proxy inverso por ruta en el Servicio web (por ejemplo `/files`), introduce la misma ruta |
| `accessUrls` | Vacío | URL de acceso, admite varias, como `https://files.example.com/` con proxy inverso del servicio web; se muestran en la tarjeta de la instancia. Si está vacío, se deduce de la dirección y el puerto de escucha. El esquema, host y puerto de la primera URL se escriben en `http.externalUrl` de File Browser, que usan los enlaces compartidos |
| `sources` | ninguno | Directorios compartidos, cada uno con `path` (absoluta), `name` (nombre visible, por defecto el del directorio) y `readOnly`; los usuarios nuevos acceden a todos por defecto |
| `authMethod` | `password` | Inicio de sesión: `password` con usuario y contraseña, `proxy` confía en la cabecera de usuario del proxy inverso, `noauth` sin inicio de sesión |
| `proxyHeader` | `Remote-User` | Cabecera con el nombre de usuario para `proxy`; los usuarios que no existen se crean automáticamente |
| `adminUser` | `admin` | Usuario administrador |
| `adminPassword` | ninguno | Contraseña del administrador, al menos 8 caracteres, obligatoria con `password` |
| `trustProxyHeaders` | desactivado | Confía en `X-Forwarded-*` y `X-Real-IP`; actívalo detrás de un proxy inverso |
| `disableWebDav` | desactivado | Desactiva la interfaz WebDAV integrada en File Browser |
| `locale` | vacío | Idioma de la interfaz para usuarios nuevos, como `es`; vacío es inglés |
| `tokenHours` | 2 | Duración de la sesión en horas |

- La contraseña del administrador se sincroniza con File Browser en cada inicio, así que prevalece el valor de Gately; los demás usuarios, enlaces compartidos y preferencias se gestionan en File Browser y se guardan en la base de datos de la instancia.
- El archivo de configuración, la base de datos y la caché se guardan en `filebrowser/<clave de la instancia>/` dentro del directorio de configuración, con el archivo de configuración en 0600; la contraseña del administrador llega al proceso mediante una variable de entorno y nunca se escribe en un archivo. Al eliminar una instancia se elimina este directorio, y los archivos de los directorios compartidos se conservan tal cual.
- Una instancia cuyo directorio compartido no existe no se inicia, y su tarjeta muestra el motivo.
- Para el acceso desde fuera, añade en el [Servicio web](/es/docs/modules/web) un sitio de proxy inverso hacia `http://127.0.0.1:puerto`, protégelo con HTTPS y control de acceso y activa «Confiar en cabeceras del proxy». El método `proxy` necesita la autenticación delegada del Servicio web para poner el nombre de usuario en la cabecera, y el puerto de File Browser debe quedar cerrado al exterior, ya que de lo contrario se podría falsificar la cabecera para iniciar sesión.
- Los archivos se leen y escriben con la identidad del proceso de Gately.

## Montajes de almacenamiento

Añade montajes en «Montajes de almacenamiento», dentro del grupo «Servicios de archivos», y administra los archivos en línea desde la página.

| Campo | Descripción |
| --- | --- |
| `name`, `remark`, `enable` | Nombre, nota e interruptor de activación |
| `type` | `local` (directorio local), `webdav` (WebDAV remoto) o `smb` (recurso compartido SMB) |
| `path` | Directorio local, debe ser una ruta absoluta existente (lo usa `local`) |
| `url`, `username`, `password` | Dirección remota y cuenta; la contraseña se enmascara en la interfaz (lo usan `webdav` y `smb`). La dirección WebDAV empieza por `http://` o `https://`; la dirección SMB tiene la forma `smb://host[:puerto]/recurso[/subdirectorio]`, admite SMB2/3 y con el usuario vacío se conecta como invitado |
| `skipVerify` | Omite la verificación del certificado HTTPS (lo usa `webdav`) |
| `domain` | Dominio o grupo de trabajo para la autenticación SMB, normalmente vacío (lo usa `smb`) |
| `readOnly` | Montaje de solo lectura; el gestor de archivos rechaza cualquier operación de escritura |

El gestor de archivos en línea permite explorar directorios, descargar archivos, subir en streaming (el archivo completo no se carga en memoria), crear directorios, renombrar y eliminar. Un montaje puede servir como ubicación de destino de una tarea de copia de seguridad: los archivos de copia se escriben en el directorio indicado del montaje y los archivos antiguos se eliminan según la política de retención.

## Wake-on-LAN

Gestiona los dispositivos en «Wake-on-LAN» y envíales paquetes mágicos (6 bytes de `0xFF` seguidos de la MAC repetida 16 veces).

| Campo | Por defecto | Descripción |
| --- | --- | --- |
| `name` | Ninguno | Nombre del dispositivo, obligatorio |
| `macs` | Ninguno | Direcciones MAC, una por línea, al menos una; puedes introducir varias para un mismo dispositivo |
| `broadcastIps` | Vacío | Direcciones de difusión IPv4. Cuando está vacío, los paquetes se envían a `255.255.255.255` y a la dirección de difusión de cada interfaz de red activada |
| `port` | 9 | Puerto UDP de destino |
| `repeat` | 3 | Número de veces que se envía a cada MAC, hasta 20 |
| `checkAddr` | Vacío | Dirección de comprobación de conexión con la forma `host:port`, por ejemplo `192.168.1.10:3389`; déjalo vacío para omitir la comprobación |
| `shutdownConn` | Vacío | Conexión SSH de la [terminal web](/es/docs/modules/terminal) usada para el apagado remoto; vacío para no permitir apagado |
| `shutdownCommand` | Ninguno | Comando de apagado, obligatorio si se elige una conexión, de una sola línea, como `shutdown -h now` o `shutdown /s /t 0` |
| `remark` | Vacío | Nota |

Pulsa Encender en la lista de dispositivos para enviar el paquete mágico, y la página muestra si el dispositivo está conectado según `checkAddr`.

### Apagado remoto

Elige una conexión SSH de la terminal web y escribe un comando de apagado, y la tarjeta del dispositivo mostrará el botón *Apagar*; la API es `POST /api/wol/devices/{key}/shutdown`.

- La conexión debe haberse abierto una vez en la terminal web para registrar la clave del host; Gately nunca confía en hosts desconocidos sin supervisión;
- Cada apagado abre una conexión SSH nueva y la cierra al terminar, así que nunca se reutiliza una conexión caducada de antes de reiniciar el dispositivo; el comando dura como máximo 30 segundos y solo se ejecuta uno a la vez por dispositivo;
- Apagar suele cortar la conexión SSH antes de devolver el código de salida; en ese caso se considera que el apagado empezó;
- El formulario ofrece comandos habituales para Linux (`shutdown -h now`), Windows (`shutdown /s /t 0`) y macOS (`sudo -n shutdown -h now`). Windows necesita el servidor OpenSSH activado; en Linux, los usuarios no root pueden usar `sudo -n shutdown -h now` con sudo sin contraseña para ese comando.

## Tareas programadas

Ejecuta comandos o envía peticiones HTTP según una programación cron.

- `spec` admite expresiones estándar de 5 campos, un campo de segundos opcional (6 campos, como `*/30 * * * * *`) y descriptores como `@every 10m` y `@daily`;
- Cuando `type` es `command`, `command` se ejecuta mediante `sh -c` (un `.bat` temporal en Windows), y el entorno recibe `taskName` más una variable en mayúsculas con el mismo nombre y el prefijo `GATELY_`;
- Cuando `type` es `http`, se envía una petición según la configuración `http`, con los campos `url`, `method` (GET, POST, PUT, PATCH, DELETE), `headers` (cada uno como `Clave: Valor`), `body`, `successContent` (el cuerpo de la respuesta debe contener cualquiera de sus entradas para contar como éxito; cuando está vacío, solo se comprueba un estado 2xx), `timeout`, `insecureSkipVerify` y `proxy` (vacío para no indicar proxy, `update` para usar el proxy global de actualizaciones, o una dirección de proxy `http://`, `https://` o `socks5://`);
- Las tareas se pueden activar o desactivar, la página muestra la hora de la próxima ejecución y una tarea se puede ejecutar de inmediato a mano;
- Si la ejecución anterior de la misma tarea no ha terminado, se omite el nuevo disparo;
- Cada tarea conserva sus últimos 20 registros de ejecución, con la hora de inicio, la duración, éxito o fallo, si se activó manualmente y la salida (un resumen de hasta 4000 caracteres);
- El formulario de creación valida e interpreta la expresión.

## Resumen y registros del sistema

- La página «Resumen» muestra la información del host: modelo de CPU, uso y carga, memoria, disco, direcciones de las interfaces de red y velocidades en vivo, IPv4 e IPv6 públicas (pulsa para copiar), el estado del servicio de cada módulo y, para el proceso de Gately, su versión, commit, hora de compilación, hora de inicio, PID, CPU, memoria residente, heap de Go y número de goroutines;
- Cuando Gately gestiona programas externos (frp, Tailscale, NetBird, túnel de Cloudflare, File Browser), la página «Resumen» incluye además la tarjeta «Procesos gestionados», que lista cada proceso con su módulo e instancia, nombre del programa, PID, tiempo activo, número de reinicios, CPU y memoria residente, con los totales en la cabecera; pulsa una fila para abrir su módulo. Solo se cuenta el propio proceso, no los hijos que inicia, y no se muestran rutas ni argumentos de arranque;
- La página «Registros del sistema» reúne los registros de todos los módulos, incluidos los registros de ejecución de módulos como Docker, FTP y Tareas programadas.

## Temas

La interfaz ofrece 15 paletas en dos grupos, originales de Gately (Esmeralda, Océano, Fresco, Sakura, Atardecer, Medianoche, Punk) y clásicas (GitHub, Nord, Dracula, Catppuccin, Tokyo Night, Gruvbox, Rosé Pine, Solarized), y cada paleta tiene variante clara y oscura. El modo puede ser claro, oscuro o seguir el sistema, que usa la variante clara u oscura de la paleta actual. Cualquier paleta admite uno de 10 colores de acento predefinidos o uno elegido con el selector del sistema, y el logotipo y el degradado de la página de inicio de sesión lo siguen; volver a elegir una paleta quita el acento personalizado. Las esquinas pueden ser rectas, las de la paleta o redondeadas. La configuración del tema se guarda localmente en el navegador.
