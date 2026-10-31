# Terminal web

Conéctate desde el navegador a servidores SSH y Telnet o al shell local. Las sesiones se conservan en el servidor y se pueden volver a conectar tras una desconexión. El punto de entrada es «Terminal web» en la barra lateral.

## Tipos de conexión

| Tipo | Descripción |
| --- | --- |
| SSH | Admite autenticación por contraseña y por clave privada (pega una clave privada o indica una ruta `keyFile` en el servidor; se admiten frases de contraseña de las claves). Puedes definir el `workDir` al que entrar tras iniciar sesión, `keepAlive`, el tiempo de espera de la conexión (de 1 a 120 segundos, por defecto 15) y un proxy |
| Telnet | Puerto por defecto 23. Cuando se detectan indicaciones como `login:`, `username:` y `password:`, el campo correspondiente se rellena automáticamente una vez cada uno. Se puede definir un proxy |
| Shell local | Abre un shell directamente en el dispositivo donde se ejecuta Gately. Puedes indicar la ruta del shell, el directorio de trabajo y variables de entorno adicionales |

Cada conexión puede tener una nota. La lista de conexiones admite ordenar arrastrando y «Probar conexión»: para SSH devuelve la versión del servidor y la latencia, para Telnet solo verifica la accesibilidad TCP (también a través del proxy), y las conexiones de shell local no se pueden probar. Al editar una conexión, la contraseña, la clave privada, la frase de contraseña de la clave y la contraseña del proxy se muestran enmascaradas, y se conservan los valores originales si no se modifican.

En «Ajustes» puedes gestionar los «preajustes de credenciales SSH» (contraseña o clave privada, con nombre de usuario predeterminado opcional). Al añadir o editar una conexión SSH, elige un preajuste para reutilizarlo: varias conexiones pueden compartir la misma credencial y los cambios del preajuste se aplican en la siguiente conexión. Un preajuste que aún usa alguna conexión no se puede eliminar.

### Proxy

Las conexiones SSH y Telnet pueden usar cada una su propio proxy de salida de tipo `socks5` o `http`, con usuario y contraseña de proxy opcionales.

### Confianza en el primer uso para las claves del host

Cuando se establece por primera vez una conexión SSH, se registra y se confía en la huella del host del servidor, y se verifica en cada conexión posterior. Tras reinstalar un servidor o cambiar sus claves, ejecuta «Restablecer huella del host» en la conexión para volver a confiar en él.

## Sesiones

- Las sesiones se conservan en el servidor. Tras actualizar el navegador o desconectarte, puedes volver a conectarte desde la lista de sesiones; al conectar se reproduce primero el búfer de salida y después se transmite la salida en vivo;
- Varios clientes pueden conectarse a la misma sesión a la vez, con entrada compartida y salida difundida;
- Las sesiones pueden tener una nota. La lista de sesiones muestra la hora de creación, la hora de la última actividad, el tamaño de la ventana y los bytes enviados y recibidos;
- Tras actualizar la página, las sesiones que siguen activas se restauran como pestañas, junto con la disposición dividida;
- El terminal incluye búsqueda (escribe una palabra clave, pulsa Intro para la siguiente coincidencia y Mayús+Intro para la anterior), copiar, pegar, borrar pantalla, ajuste del tamaño de fuente, pantalla completa y varios esquemas de color de terminal (por defecto sigue el tema de la interfaz, o se fija uno).

### Ajustes

Se ajustan en «Ajustes», arriba a la derecha de la página:

| Campo | Por defecto | Descripción |
| --- | --- | --- |
| `allowLocalShell` | Desactivado | Si se permite abrir el shell local |
| `maxSessions` | 20 | Máximo de sesiones simultáneas, hasta 1000. Crear una sesión por encima del límite devuelve HTTP 429 |
| `idleTimeout` | 0 | Una sesión termina automáticamente tras estos minutos sin entrada ni salida; 0 significa sin límite |
| `bufferSize` | 256 | Búfer de reproducción que se conserva por sesión (KB), hasta 16384 |
| `heartbeatInterval` | 30 | Intervalo de latido de WebSocket (segundos), hasta 600 |
| `sessionKeepAlive` | 0 | Minutos que se conserva una sesión tras desconectarse el navegador; 0 la termina inmediatamente al desconectar, hasta 10080 |

::: tip
Para poder volver a conectarte tras una desconexión, define `sessionKeepAlive` con un valor mayor que 0. Con 0, la sesión termina en cuanto se desconecta el navegador.
:::

## Gestor de archivos SFTP

El icono de carpeta de la barra de herramientas del terminal abre el panel del gestor de archivos. Las conexiones SSH operan sobre los archivos remotos mediante SFTP, y las conexiones de shell local operan directamente sobre el sistema de archivos del dispositivo donde se ejecuta Gately. Las conexiones Telnet no tienen gestor de archivos. El gestor de archivos local requiere que «Permitir shell local» esté activado.

| Capacidad | Descripción |
| --- | --- |
| Explorar | Navegación con ruta de migas y directorio superior, directorios primero y ordenados por nombre, con orden conmutable; muestra permisos, tamaño, fecha de modificación y propietario |
| Crear | Nueva carpeta, nuevo archivo vacío |
| Subir | Subida de varios archivos; en escritorio se pueden subir carpetas completas (se conserva la estructura de subdirectorios). Los archivos con el mismo nombre se sobrescriben y el panel muestra el progreso de la subida |
| Descargar | Descarga en streaming con reanudación (`Range`) |
| Edición en línea | Vista previa y guardado de archivos de texto; los archivos de más de 1 MB o binarios no se pueden previsualizar |
| Renombrar, copiar, pegar | Al copiar en el mismo directorio, el archivo se renombra automáticamente a `nombre copia.ext`, `nombre copia 2.ext` |
| Eliminar | Eliminación por lotes con selección múltiple; los directorios se eliminan de forma recursiva; el directorio raíz está protegido |
| Comprimir y extraer | Comprime a `zip` o `tar.gz`; extrae `zip`, `tar`, `tar.gz` y `tgz`; el contenido del archivo (hasta 10000 entradas) se puede previsualizar antes de extraer. La compresión y la extracción las hace el propio Gately, por lo que no hace falta instalar comandos en el host remoto |
| Cambiar permisos | Permisos octales (de 0 a 7777), con una opción recursiva para directorios |
| Acceso rápido | Guarda los directorios más usados de cada conexión y salta a ellos con un clic |

Al extraer, se rechaza por completo un archivo que contenga cualquier entrada con `..` para evitar el recorrido de directorios, se omiten las entradas de enlaces simbólicos y enlaces duros, y solo se conservan los bits rwx de los permisos de archivo. En móvil, el panel ofrece un modo de selección múltiple y una hoja de acciones.

## Atajos

El desplegable «Atajos» lista los atajos globales, y al pulsar uno se envía al terminal actual. Cada atajo tiene un `name`, un `command` y `sendEnter` (si se pulsa Intro automáticamente tras enviarlo). Añade, edita y elimina atajos con «Gestionar atajos», en la parte inferior del desplegable.

## Vista dividida

El menú «Dividir» de la barra de herramientas ofrece disposiciones en paralelo, apiladas y en cuadrícula, con hasta 4 paneles a la vez. Con la vista dividida activada, cualquier conexión que abras se une automáticamente a la división, y también puedes pulsar otra pestaña para añadirla. La barra de título de un panel permite quitar una sesión de la división. La disposición dividida se guarda en el servidor y se restaura tras actualizar la página. En pantallas estrechas, la vista dividida se desactiva automáticamente y la disposición guardada no se elimina.

## Compatibilidad con móviles

- En pantallas estrechas, la lista de conexiones y el terminal son vistas separadas, y el botón de volver, arriba a la izquierda de la vista del terminal, regresa a la lista;
- El área del terminal sigue el viewport visual y se reduce cuando aparece el teclado en pantalla;
- La barra de teclas virtuales es una sola fila que se desliza lateralmente. Ofrece `Esc`, `Tab`, `Ctrl`, `Alt` (persistentes), teclas de flecha (mantén pulsado para repetir), símbolos habituales y un acceso a los atajos, con un botón de teclado fijo a la derecha que muestra u oculta el teclado. Por defecto la barra aparece y desaparece con el teclado en pantalla; con el teclado cerrado, un botón flotante en la esquina inferior derecha lo vuelve a mostrar. Para usar flechas, paginación y otras teclas sin el teclado, activa **Mostrar siempre la barra de teclas** en el menú **Más**.

## Seguridad

Un terminal equivale a ejecutar comandos arbitrarios en el host de destino, por lo que Gately aplica estas restricciones por defecto:

- **El shell local está desactivado por defecto** y debe activarse explícitamente en los ajustes;
- **Confirmación del acuerdo**: la primera vez que activas el shell local, creas una sesión, te conectas a una sesión o usas el gestor de archivos, aparece un acuerdo de usuario que debes marcar para continuar;
- **Autenticación de dos factores (2FA)**: tras vincular un autenticador TOTP escaneando el código QR en los ajustes de seguridad, las acciones anteriores requieren un código de 6 dígitos. Tras la verificación, el token de inicio de sesión actual no necesita volver a verificarse durante 30 minutos; tras 5 códigos erróneos consecutivos, ese token de inicio de sesión se bloquea durante 1 minuto;
- Las contraseñas de conexión, las claves privadas y las contraseñas de proxy se enmascaran en las respuestas de la API.

::: warning
Antes de activar el shell local, activa la autenticación de dos factores en la interfaz de administración y úsalo junto con «Acceso solo desde la red local» o una VPN.
:::

## API

Consulta la [API del terminal web](/es/docs/reference/webterminal-api) para ver la referencia completa de la API.
