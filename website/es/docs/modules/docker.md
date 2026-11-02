# Docker

Gestiona contenedores, proyectos Compose, imágenes, redes y volúmenes del host a través del socket de Docker. El punto de entrada es «Docker» en la barra lateral, y la página se divide en seis pestañas: «Contenedores», «Compose», «Imágenes», «Redes», «Volúmenes» y «Sistema». Cuando no se puede acceder a Docker, la parte superior de la página muestra los detalles del error y un botón de acceso directo para abrir los ajustes.

## Activación

Cuando Gately se ejecuta en un contenedor, monta el socket de Docker del host:

```bash
-v /var/run/docker.sock:/var/run/docker.sock
```

La dirección de Docker es por defecto `unix:///var/run/docker.sock` y se puede cambiar en los ajustes de la pestaña «Sistema».

::: warning Nota de seguridad
Montar `docker.sock` equivale a conceder acceso de nivel root al host. Si no necesitas la gestión de Docker, quita el montaje y protege la interfaz de administración (autenticación de dos factores, ruta de acceso segura, acceso solo desde la red local).
:::

## Contenedores

- La lista se puede filtrar por «Todos», «En ejecución» y «Detenidos». Las tarjetas muestran la imagen, la red, los puertos, los montajes y el estado de actualización;
- Acciones: iniciar, detener, reiniciar, pausar, reanudar, forzar la terminación, renombrar, eliminar (opcionalmente con los volúmenes anónimos) y crear una imagen a partir del contenedor (commit);
- **Autoprotección**: Gately reconoce su propio contenedor y su proyecto Compose, y rechaza las operaciones que cortarían el acceso a sí mismo: detener, forzar la detención, pausar o eliminar su contenedor, y ejecutar `down` en su proyecto. Editar y recrear, ejecutar `up` en su proyecto o eliminarlo requieren detener antes el contenedor anterior; el proceso de Gately terminaría con él y no podría completar la recreación, así que también se rechazan y los botones correspondientes aparecen desactivados en la interfaz. La protección también se aplica al llamar a la API con un nombre de contenedor o un prefijo de ID.
- **Actualizar el propio Gately**: al actualizar el contenedor de Gately, este descarga primero la nueva imagen y, solo si hay una versión nueva, crea con su imagen actual un contenedor auxiliar temporal (`<nombre del contenedor>-upgrade-<hora>`). El auxiliar detiene el contenedor anterior, crea e inicia el nuevo con la misma configuración y elimina el anterior; si el nuevo no arranca, restaura el anterior. Mientras tanto la interfaz muestra una pantalla de espera y se recarga cuando Gately vuelve; el nuevo Gately escribe la salida del auxiliar en el log de Docker y lo elimina. Las actualizaciones por lotes procesan Gately al final y no lo actualizan si falló otro contenedor. El auxiliar accede al socket de Docker mediante el mismo montaje que Gately (por ejemplo `/var/run/docker.sock:/var/run/docker.sock`); sin ese montaje la actualización se rechaza. No recrees Gately con `docker compose up -d`, `docker restart` u órdenes similares en la shell local del Terminal web: en un despliegue con Docker esa shell se ejecuta dentro del contenedor de Gately, así que la orden termina junto con Gately y deja el contenedor anterior detenido junto a uno nuevo con un nombre temporal (`<prefijo de ID>_<nombre del contenedor>`). Si ocurre, ejecuta otra vez `docker compose up -d` en el directorio de Compose del host para recuperarlo. Actualizar en el host con `docker compose pull && docker compose up -d` sigue funcionando como antes.
- **Información sensible**: cuando una cuenta de solo lectura o un token de API con solo `docker:read` consulta un contenedor, las variables de entorno muestran solo su nombre, y los valores, Command, Entrypoint, los argumentos de inicio y los argumentos de los comandos en la lista de procesos se muestran como `********`. Los detalles de la imagen ocultan igualmente las variables de entorno, Cmd, Entrypoint y los comandos del historial de compilación (al crear una imagen a partir de un contenedor, esta configuración se escribe tal cual en la imagen). Los administradores con permiso de escritura obtienen el texto en claro automáticamente al abrir el formulario de edición.
- **Crear y editar**: el formulario cubre nombre, imagen (se puede descargar antes de crear), iniciar tras la creación, comando y Entrypoint, variables de entorno, mapeos de puertos (TCP/UDP, con IP del host opcional), montajes (bind de directorio y volumen, opcionalmente de solo lectura), red principal y redes adicionales, IP fija, alias, política de reinicio (`no`, `always`, `unless-stopped`, `on-failure` con un número máximo de reintentos), modo privilegiado, añadir o quitar capabilities, mapeo de dispositivos, usuario, directorio de trabajo, hostname, DNS, etiquetas, límite de memoria (MB), número de CPU, TTY y mantener stdin abierto. Al editar se recrea el contenedor a partir de su configuración original; los ajustes que el formulario no cubre conservan sus valores originales, y si la recreación falla se revierte automáticamente;
- Pestañas de la página de detalle:
  - Resumen: estado, código de salida, número de reinicios, estado de salud, detalles de red, montajes, y el proyecto y servicio de Compose al que pertenece;
  - Registros: consulta de los registros recientes, con marcas de tiempo opcionales;
  - Terminal: abre un terminal interactivo dentro de un contenedor en ejecución (`/bin/sh` por defecto; se pueden indicar el comando y el usuario; los colores siguen la preferencia del terminal web);
  - Archivos: explora los archivos del contenedor, edita texto en línea (hasta 1 MiB), sube, descarga (los directorios se empaquetan como tar.gz), renombra, crea directorios y elimina. Al eliminar se ejecuta `rm -rf` dentro del contenedor, por lo que el contenedor debe estar en ejecución;
  - Monitorización: muestras de CPU, memoria, red y E/S de disco, y número de procesos, además de la lista de procesos dentro del contenedor.

## Compose

Los proyectos Compose se agregan a partir de etiquetas de contenedor como `com.docker.compose.project`. Muestran un estado (en ejecución, parcialmente en ejecución, detenido) y sus servicios.

- Iniciar, detener, reiniciar, pausar, reanudar, `down` (opcionalmente con volúmenes con nombre) y ver los registros combinados (cada línea lleva como prefijo el nombre del servicio);
- Crear un proyecto: pega el contenido del compose y Gately lo escribe en `compose/<nombre>/compose.yaml` dentro del directorio de configuración y ejecuta up. Estos proyectos se marcan como «gestionados»; puedes editar el archivo Compose, guardarlo y aplicarlo, o eliminar el proyecto completo;
- En los proyectos existentes, el primer archivo compose se puede editar en línea (el archivo debe tener permisos de escritura), con la opción de ejecutar up inmediatamente después de guardarlo;
- `up`, la descarga de imágenes, las compilaciones y la lectura y escritura de archivos dependen de la línea de comandos `docker compose` y del acceso a la ruta del archivo compose. Cuando no están disponibles, la interfaz muestra «docker compose command not found».

## Imágenes

- La lista muestra etiquetas, tamaño, fecha de creación, número de contenedores que hacen referencia a cada imagen y estado dangling. Los detalles incluyen el historial de capas, variables de entorno, puertos expuestos y más;
- **Descargar (pull)**: se autentica con las cuentas de registro de los ajustes de «Sistema». Si falla una descarga directa y hay direcciones de mirror configuradas, se prueban en orden y la imagen se vuelve a etiquetar con su nombre original. Se admite la búsqueda en Docker Hub;
- Push, añadir etiqueta, eliminar (opcionalmente forzando), compilar (Dockerfile en línea o URL de Git, con argumentos de compilación) e importar y exportar tar (formato `docker save`);
- **Comprobación de actualizaciones**: comprueba un contenedor concreto o usa «Comprobar todas las actualizaciones»; los resultados se marcan en cada contenedor;
- **Actualización con un clic**: descarga la nueva imagen y recrea el contenedor con su configuración actual; si la actualización falla, se revierte automáticamente al contenedor original. Se admite la selección múltiple para actualizaciones por lotes;
- «Limpiar dangling» elimina con un clic todas las imágenes sin etiqueta y sin usar.

## Redes y volúmenes

- Redes: la lista muestra el driver, el ámbito, la subred, la puerta de enlace y los contenedores conectados. Puedes crear (driver, subred, puerta de enlace, red interna, IPv6) y eliminar redes, y conectar o desconectar contenedores. Las redes integradas están protegidas;
- Volúmenes: muestra el driver, el punto de montaje, el tamaño, el número de referencias y los contenedores que usan cada volumen. Puedes crear, eliminar (opcionalmente forzando), exportar como tar.gz e importar desde tar.gz. La exportación y la importación usan un contenedor auxiliar alpine temporal, y la imagen se descarga automáticamente si falta.

## Sistema

La pestaña «Sistema» contiene:

- Información del host y de Docker: versión, versión de la API, SO y arquitectura, kernel, CPU y memoria, driver de almacenamiento, directorio de datos, número de contenedores e imágenes, versión de la CLI de Compose;
- Uso de disco: número, tamaño y espacio recuperable de imágenes, contenedores, volúmenes y caché de compilación;
- Limpieza: contenedores detenidos, imágenes dangling, todas las imágenes sin usar, redes sin usar, volúmenes sin usar (los datos se pierden de forma permanente) y caché de compilación;
- Ajustes:

| Campo | Descripción |
| --- | --- |
| `host` | Dirección de Docker, por defecto `unix:///var/run/docker.sock` |
| `mirrors` | Lista de direcciones de mirror, que se prueban en orden cuando falla una descarga directa. Son independientes de `registry-mirrors` en `/etc/docker/daemon.json`, que Docker usa automáticamente y la página de ajustes muestra en solo lectura como «Réplicas del daemon» |
| `registries` | Credenciales de registros privados (`server`, `username`, `password`); la contraseña se enmascara en la interfaz |
| `autoCheckHours` | Si es mayor que 0, las actualizaciones de las imágenes de los contenedores se comprueban automáticamente con este intervalo en horas; 0 lo desactiva; las imágenes más recientes publican un evento `docker.update`, una vez por imagen nueva |

## Tareas en segundo plano

Las operaciones largas como pull, push, compilación, comprobación por lotes, actualización por lotes y Compose up se ejecutan como tareas en segundo plano. El panel de tareas de la parte inferior de la página muestra la salida en vivo y el estado (en ejecución, terminada, error, detenida), permite ver el historial y limpia las tareas finalizadas. Las tareas que siguen en ejecución se restauran tras actualizar la página.

Una tarea en curso se puede detener con «Detener» en el panel de tareas o en «Sistema → Historial de tareas»; tras confirmar:

- Pull, push, compilación de imágenes y comprobación de actualizaciones se detienen de inmediato, y Docker también aborta la descarga o compilación correspondiente, así que no queda ninguna imagen a medio descargar;
- Los comandos de Compose reciben primero una señal de interrupción (la misma que Ctrl+C en un terminal) y terminan por sí mismos; si no han salido en 30 segundos, se fuerzan a terminar;
- La actualización por lotes se detiene cuando acaba el contenedor actual y los siguientes no se actualizan. El contenedor que se está recreando (el tramo de parar el antiguo, crear e iniciar el nuevo) ignora la interrupción y completa la recreación; si falla, se restaura el contenedor original como siempre, sin quedar a medias.

Tras detenerla, la tarea queda en estado «Detenida» y el registro de ejecución anota «Tarea detenida». Las tareas que ya habían terminado en el momento de detener muestran su resultado real. Una tarea atascada se puede limpiar con «Borrar finalizadas» una vez detenida. La API correspondiente es `POST /api/docker/tasks/{id}/cancel`.

## Notificaciones de eventos

Gately se suscribe al flujo de eventos de Docker y publica un evento `docker.exited` cuando un contenedor termina de forma anómala (código de salida distinto de cero) o es eliminado por el OOM killer. Puedes configurar los canales receptores en el [Centro de notificaciones](/es/docs/modules/notify). Una parada normal con código de salida 0 no genera ninguna notificación. Cuando Docker no está disponible temporalmente, la suscripción se reintenta cada 30 segundos.

## API

Consulta la [API de Docker](/es/docs/reference/docker-api) para ver la referencia completa de la API.
