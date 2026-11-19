# Configuración y parámetros de ejecución

## Opciones de línea de comandos

| Opción | Descripción |
| --- | --- |
| `-c` | Directorio de configuración; también se puede definir con la variable de entorno `GATELY_CONFIG_DIR` |
| `-listen` | Sobrescribe la dirección de escucha de la interfaz de administración, p. ej. `:16680` |
| `-apply` | Directorio de configuración declarativa; también se puede definir con la variable de entorno `GATELY_APPLY_DIR`. Consulta más abajo |
| `-gateway-socket` | Sirve también la interfaz de administración en este socket Unix, por ejemplo para la pasarela unificada de fnOS |
| `-gateway-prefix` | Prefijo de ruta que se usa al acceder a través de la pasarela, por ejemplo `/app/gately` |
| `-api-docs` | Exporta la documentación de la API (`openapi.json` y `API.md`) al directorio indicado y termina |
| `-version` | Muestra la información de versión |

| Variable de entorno | Descripción |
| --- | --- |
| `GATELY_INITIAL_PASSWORD` | Contraseña inicial del administrador en el primer arranque (8 caracteres como mínimo); si no se define, se usa la contraseña predeterminada `gately.cn`. No tiene efecto si la cuenta ya existe. Si tiene menos de 10 caracteres o no es lo bastante segura, se pedirá cambiarla en el primer inicio de sesión |
| `GATELY_LANG` | Código de idioma que se lee primero cuando el idioma del servidor es «Seguir el sistema»; después se leen `LC_ALL`, `LC_MESSAGES` y `LANG`, en ese orden |
| `GATELY_LOG_LEVEL` | Nivel mínimo de los registros de ejecución que se guardan: `debug`, `info`, `warn` o `error`. Se aplica cuando «Nivel de registro» en los ajustes es «Predeterminado»; `info` si no está definida |

## Directorio de configuración

Toda la configuración vive en el directorio de configuración, con un archivo de texto plano `<nombre>.json` por módulo, escrito de forma atómica con permisos 0600. Esto te permite:

- Inspeccionarlo con cualquier editor de texto;
- Guardar el directorio en git, auditar cada cambio con diferencias y revertir en cualquier momento;
- Hacer copias de seguridad simplemente copiando el directorio.

::: warning Nota
`ssl.json` contiene claves privadas de certificados. Antes de subirlo al control de versiones, asegúrate de que el repositorio sea privado o añade el archivo a tu lista de ignorados.
:::

**Ajustes → Red y ejecución → Uso de almacenamiento** muestra la capacidad de la partición que contiene el directorio de configuración, el tamaño total del directorio de configuración y el uso por finalidad: registros de ejecución, auditoría de visitantes, registro de auditoría de operaciones, historial de configuración, copias de configuración, datos de protección, datos de red superpuesta, listas de IP, certificados, base de datos GeoIP, programas gestionados (`bin/`), archivos de configuración y otros, para saber qué está llenando el disco. Solo se cuentan tamaños y no se siguen enlaces simbólicos; un **Directorio de registros** fuera del directorio de configuración se lista aparte y no cuenta en su tamaño. Se cuenta al abrir la pestaña; pulse **Actualizar** para volver a contar.

## Configuración declarativa

Con `-apply <directorio>`, en cada arranque los archivos `<nombre>.json` de ese directorio se copian sobre el directorio de configuración; los archivos con contenido idéntico no se reescriben. Es útil para distribuir la configuración desde un repositorio git, un ConfigMap de Kubernetes o un script de despliegue.

- Antes de escribir se comprueba que cada archivo sea JSON válido; si alguno no lo es, Gately se niega a arrancar;
- `account.json` y `tokens.json` no se pueden sobrescribir: la cuenta y los tokens solo se gestionan desde la interfaz de administración;
- El contenido sobrescrito se conserva en el [historial de configuración](/es/docs/guide/audit) y se puede revertir en cualquier momento.

## Directorio de programas externos

Tailscale, frp, cloudflared y File Browser supervisan los programas oficiales externos. Gately busca cada ejecutable en este orden:

1. La ruta definida en los ajustes del módulo;
2. El subdirectorio `bin/` del directorio de configuración;
3. `PATH`.

Cada página de módulo permite instalar, actualizar y revertir en línea: los programas se instalan en `bin/` y las versiones anteriores se guardan en `bin/.versions/`. Consulta [Instalación, actualización y reversión en línea](/es/docs/modules/tunnels#install). También puedes colocar los binarios tú mismo en el directorio de configuración, en Docker por ejemplo `./gately-data/bin/frpc`; Tailscale necesita tanto `tailscale` como `tailscaled`.

## Registros de ejecución

Los registros de ejecución del sistema y de cada módulo se escriben por defecto en `logs/` dentro del directorio de configuración, así que siguen disponibles en la pestaña «Registros del sistema» de la página «Sistema → Registros» y en la página de cada módulo después de reiniciar o actualizar:

- Un archivo JSON Lines por origen: `logs/<ID del módulo>.log`, y los del sistema en `logs/gately.log`. Al arrancar se restauran desde el archivo las entradas más recientes, hasta «Líneas de registro conservadas»;
- Las entradas nuevas se escriben por lotes con un retraso máximo de 1 segundo. Cuando un archivo supera el límite (2 MB por defecto) se rota a `.1` y solo se conserva ese archivo anterior, de modo que cada origen ocupa como máximo el doble del límite;
- Los registros de acceso del Servicio web son voluminosos y se suelen revisar más tarde, por lo que se archivan por día: cuando el archivo actual cambia de día (o supera una cuarta parte del tamaño total, como máximo 16 MB), se renombra a `logs/webservice.<hora de inicio>.log` y se comprime en segundo plano a `.log.gz`; los archivos más antiguos se eliminan según «Retención de registros de acceso (días)» (7 por defecto) y «Tamaño total de registros de acceso (MB)» (el archivo actual más todos los archivos comprimidos, 32 MB por defecto y como máximo el 5 % de la partición donde están los registros);
- Cada módulo anota los orígenes de registro que han aparecido en `logs/<ID del módulo>.sources`, que se conserva tras reiniciar;
- En Ajustes → Red y ejecución → Parámetros de ejecución puedes desactivar «Guardar registros en disco», cambiar «Límite del archivo de registro», «Retención de registros de acceso (días)» y «Tamaño total de registros de acceso (MB)», o mover «Directorio de registros» a otra ruta absoluta (por ejemplo, un directorio en un disco externo; si no existe se crea con acceso solo para el usuario con el que se ejecuta Gately), con efecto inmediato. Tras cambiar el directorio, los registros nuevos se escriben en el nuevo y los archivos del anterior se quedan donde están. Si se desactiva el guardado en disco, los registros solo se guardan en memoria y se borran al reiniciar;
- «Nivel de registro» decide qué registros de ejecución se guardan. El valor predeterminado es «Info y superiores»: los registros de depuración no se guardan y no llegan a las páginas de registros, a los archivos de registro ni a la salida de error estándar. La línea por conexión del reenvío de puertos y del enrutamiento por protocolo, las desconexiones de clientes y los fallos de handshake TLS de dominios ajenos son de nivel depuración; para verlos, cambia «Nivel de registro» a «Depuración y superiores (todo)». Se aplica en cuanto se guarda y añade una carga apreciable con muchas conexiones. Los registros de acceso de los servicios web son de nivel info y dejan de guardarse con «Advertencia y superiores» o «Solo errores». La variable de entorno `GATELY_LOG_LEVEL` también fija el nivel; un nivel concreto elegido en los ajustes tiene prioridad;
- Los registros de ejecución también se muestran en la salida de error estándar (lo que enseñan `docker logs` y `journalctl`), en el idioma del servidor. Esa salida se escribe por lotes en segundo plano y se completa antes de que el proceso termine con normalidad o se reinicie; cuando la salida de error estándar está bloqueada o es demasiado lenta y hay más de 1 MB pendiente, las líneas nuevas se descartan y una línea indica cuántas, sin afectar a las páginas de registros ni a los archivos de registro. El controlador de registros predeterminado de Docker no limita el tamaño de los registros del contenedor; el ejemplo de despliegue [docker-compose.yml](https://github.com/wangwanjie/gately/blob/main/deploy/docker-compose.yml) fija un límite con `max-size` y `max-file` en `logging.options`;
- Borrar los registros de un módulo en la interfaz también elimina sus archivos, el archivo rotado, los archivos comprimidos y la lista de orígenes;
- Las copias de seguridad de la configuración no incluyen `logs/` ni el registro de auditoría `audit.log`.

### Ver y filtrar {#log-filter}

La pestaña «Registros del sistema» de la página «Registros» y la tarjeta de registros al final de cada página de módulo usan el mismo visor. Las siguientes condiciones se pueden combinar; los cambios se aplican al instante y solo se añaden las entradas nuevas que coinciden:

- Nivel y origen (la lista desplegable muestra todos los orígenes de la configuración del módulo y los que han aparecido en los registros, por ejemplo todas las escuchas del Servicio web, incluidas las que aún no tienen accesos o cuyos registros solo están en disco; también se puede escribir uno);
- Palabras clave: los términos separados por espacios deben coincidir todos, las comillas dobles agrupan una frase y un `-` inicial excluye un término, por ejemplo `GET "timeout" -debug`; las coincidencias se resaltan. Si la interfaz está en otro idioma, busque con el texto traducido que ve en pantalla;
- Excluir: se ocultan las entradas que contienen cualquiera de sus términos, junto con las palabras clave; se escribe igual (separado por espacios, comillas dobles para frases), por ejemplo `debug "health check"`. Para excluir un término que empieza por `-`, póngalo entre comillas dobles, como `"-v"`;
- IP en Más filtros: dirección completa, rango (como `10.0.0.0/8`) o parte de una dirección, separadas por comas; cuando una entrada no tiene un campo IP propio, las direcciones se reconocen en el texto, así que también sirve para inicios de sesión fallidos y registros de protección contra intrusiones; el prefijo `!` o `-` excluye, por ejemplo `!1.2.3.4, !10.0.0.0/8` oculta esos orígenes, y se puede combinar con elementos incluidos (`10.0.0.0/8, !10.0.0.5`);
- Intervalo de tiempo en Más filtros: desde los últimos 15 minutos hasta los últimos 7 días, o un inicio y fin personalizados;
- La barra del Servicio Web también tiene dos interruptores de visualización que solo afectan a lo que se muestra (los registros se conservan y la elección se recuerda en el navegador): Ocultar orígenes bloqueados oculta las solicitudes rechazadas porque el origen está bloqueado (detrás de una CDN, los bloqueos del kernel no se aplican a las conexiones de la CDN, así que los visitantes bloqueados se rechazan en la capa web y dejan entradas 403 / 444), y también todas las solicitudes de los orígenes que siguen bloqueados, incluidas las que provocaron el bloqueo (vuelven a mostrarse cuando el bloqueo se levanta o caduca); Ocultar solicitudes de la consola oculta las solicitudes con una sesión de la consola de administración de esta instancia, como el sondeo de la página de registros al abrir la consola a través de un sitio del Servicio Web, y está activado por defecto. Estas entradas llevan la etiqueta «Bloqueado por=Bloqueado» o «Usuario de la consola=…», y los parámetros de la API son `hideBanned=1` y `hideAdmin=1` (solo cuentan las sesiones de administrador, no las cuentas de solo lectura; las solicitudes bloqueadas nunca llevan la etiqueta de usuario de la consola);
- Los registros del Servicio web también se pueden filtrar por nombre de sitio o dominio y por código de estado (`404`, `4xx`, `500-599`, separados por comas); los del Cortafuegos WAF, por dominio, y las detecciones registran el dominio y el ID de la regla.

Haga clic en un origen del registro, o en una etiqueta como «Sitio=…» o «Dominio=…» al final de una línea, para filtrar por ese valor; en los registros del Servicio Web, «Sitio=…» filtra por el nombre del sitio con coincidencia exacta y la barra muestra una etiqueta «Sitio: …» que se puede quitar (parámetro de la API `site`), de modo que las entradas de sitios con nombres parecidos no se mezclan. La API acepta además un parámetro `listener` (clave del listener, coincidencia exacta); cuando dos listeners comparten nombre, sus nombres de origen en el registro llevan la dirección de escucha, por ejemplo `HTTPS (0.0.0.0:443)` y `HTTPS ([::]:443)`, y los nombres únicos se mantienen. Cuando las entradas en memoria no bastan, Cargar registros anteriores continúa por el historial guardado en los archivos de registro (incluidos los archivos comprimidos del Servicio web; al filtrar por intervalo de tiempo se omiten los que quedan fuera), por lo que con la persistencia de registros activada el filtrado no está limitado por el número de líneas conservadas. `GET /api/logs` y `GET /api/<módulo>/logs` aceptan los mismos parámetros de filtro; consulte la [referencia de la API](/es/docs/reference/api-reference).

La altura del área de registros se puede ajustar: arrastra la barra situada debajo (en pantallas táctiles, mantén pulsado y arrastra), haz doble clic en la barra para restaurar la altura predeterminada o usa las flechas cuando la barra tenga el foco. Las tarjetas de registros de las páginas y el diálogo de registros del sitio recuerdan su altura por separado, en el navegador actual. El botón de pantalla completa a la derecha de la barra de herramientas hace que los registros ocupen toda la ventana; vuelve a pulsarlo o pulsa Esc para salir. Mientras te desplazas hacia arriba por el historial, los registros nuevos se siguen añadiendo pero ya no devuelven la vista al final; en la esquina inferior derecha aparece «Ir a lo más reciente» con el número de registros llegados. Volver al final o pulsar el botón reanuda el desplazamiento automático.

### Exportar {#log-export}

«Exportar» en la barra de herramientas de registros exporta todas las entradas que cumplen los filtros actuales, incluido el historial guardado en los archivos de registro y en los archivos comprimidos, de la más antigua a la más reciente, y el navegador las descarga directamente:

- CSV (hoja de cálculo): UTF-8 con BOM, se abre directamente en Excel; las columnas son hora, nivel, origen, mensaje y campos como IP, dominio, sitio y código de estado; las celdas que empiezan por `=`, `+`, `-` o `@` llevan delante una comilla simple para que la hoja de cálculo nunca ejecute como fórmula las rutas enviadas por los visitantes;
- JSON Lines: una entrada por línea, con los mismos campos que la API de registros;
- Los mensajes se exportan en el idioma de la interfaz; las cuentas de solo lectura y los tokens de API con permiso solo de lectura también pueden exportar, y las contraseñas y tokens de las direcciones se ocultan igual que al verlos;
- La API es `GET /api/logs/export` y `GET /api/<módulo>/logs/export`, con `format` igual a `csv` o `jsonl` y el resto de parámetros de filtro iguales que al ver los registros; la API de la lista de orígenes es `GET /api/logs/sources` y `GET /api/<módulo>/logs/sources`.

## Confirmación tras guardar y reversión automática {#confirm}

Al administrar Gately a distancia mediante un dominio, cambiar el reenvío de puertos, el enrutamiento por protocolo, el servicio web o los ajustes globales puede cortar el propio camino que usa la interfaz de administración. Por eso, tras guardar estos ajustes, la interfaz comprueba con una conexión recién abierta que sigue llegando a Gately:

- Si el acceso funciona, se confirma en segundo plano sin molestar;
- Si no se puede confirmar, aparece un diálogo con cuenta atrás que se confirma solo cuando vuelve el acceso; «Revertir ahora» se ofrece mientras aún se llega a Gately; si no, esa petición tampoco llega, así que espera a la cuenta atrás o ejecuta `gately rollback -pending` en el servidor;
- Si al agotarse el plazo sigue sin confirmarse, Gately se reinicia y restaura la configuración anterior al arrancar. La versión modificada queda en el [Historial de configuración](/es/docs/guide/audit). La página se recarga cuando Gately vuelve e indica qué cambios se revirtieron.

La confirmación usa un WebSocket: el navegador siempre abre una conexión nueva para él, así que pasa por el reenvío y las escuchas modificados. Las conexiones persistentes abiertas antes del cambio siguen llegando al destino anterior tras una actualización en caliente y no prueban nada sobre la nueva configuración.

Los ajustes que solo se aplican tras reiniciar (direcciones de escucha de la interfaz, módulos activados) se registran al reiniciar desde la interfaz de administración, y el plazo empieza tras el reinicio. Si Gately arranca 3 veces seguidas sin confirmación (por ejemplo, la nueva configuración impide que funcione), también se revierte. Si abres la interfaz en una dirección nueva con un cambio pendiente, un diálogo te pregunta si «Conservar cambios» o «Revertir ahora».

El plazo se ajusta en «Ajustes → Control de acceso → Listener de la consola → Confirmar tras guardar» (30–600 segundos, 90 por defecto) y también se puede desactivar. Solo afecta a lo que se guarda desde la interfaz de administración; los cambios hechos con tokens de API y scripts no se ven afectados. La API es `GET /api/system/confirm`.

### Aviso antes de guardar: camino de acceso actual {#access-path}

Gately sabe por cuál de sus propios reenvíos pasa tu conexión a la interfaz de administración: el reenvío de puertos y el enrutamiento por protocolo registran un salto al conectar con un destino de este equipo, y el servicio web añade información firmada del camino cuando hace de proxy hacia la interfaz (no se puede falsificar desde fuera). La tarjeta «Camino de acceso actual», arriba en «Ajustes → Control de acceso», muestra el camino, por ejemplo `Visitante → Enrutamiento por protocolo «vps-443» → Servicio web «main» › sitio «admin.example.com» → Interfaz de administración`. Los programas de este equipo, como frp o Cloudflare Tunnel, aparecen como «Este equipo».

Antes de guardar, la interfaz pregunta al servidor si el cambio afecta a ese camino. Si es así, un diálogo explica por qué y resalta el salto afectado; puedes cancelar o «Guardar igualmente»:

- cambiar ajustes de reenvío de una regla, listener o sitio del camino (cambiar solo nombre, grupo o nota no cuenta), desactivarlo o eliminarlo, o mover el sitio a otro listener;
- desactivar un módulo del camino (su interruptor general o el ajuste «Módulos»);
- cambiar la dirección de escucha de la interfaz que estás usando;
- activar «Solo LAN» conectándote desde fuera, o «Forzar HTTPS» usando HTTP.

Además del aviso, la confirmación y la reversión automática de arriba siguen activas, así que también se protegen caminos que Gately no ve (por ejemplo, otro proxy inverso delante). La API es `GET /api/system/access-path` y `POST /api/system/access-path/check`.

### Cambios por lotes: varios cambios que solo funcionan juntos {#changeset}

Algunos cambios solo funcionan juntos, por ejemplo cambiar a 18081 el puerto al que el enrutamiento por protocolo envía al servicio web y luego mover el listener del servicio web a 18081. Si guardas solo el primero, se corta tu acceso actual y ya no puedes hacer el segundo. Usa los cambios por lotes:

1. En el aviso antes de guardar elige «Guardar en espera y aplicar luego», o pulsa «Iniciar cambios por lotes» en la tarjeta «Camino de acceso actual» de «Ajustes → Control de acceso»;
2. Desde entonces, lo que guardes en reenvío de puertos, enrutamiento por protocolo y servicio web solo actualiza la configuración: las páginas muestran los valores nuevos, pero las reglas y listeners en marcha no cambian y tu camino de acceso actual sigue funcionando. Un aviso arriba en la página enumera las reglas y listeners en espera, y mientras tanto no aparecen avisos antes de guardar;
3. Al terminar, pulsa «Aplicar todo» en el aviso: los cambios en espera se aplican de una vez en el orden en que se guardaron y se confirman por una conexión nueva como un guardado individual; si no se puede confirmar, todo el lote vuelve a la configuración de antes de empezar;
4. Para descartarlo, pulsa «Descartar cambios»: Gately restaura la configuración de antes del lote y se reinicia.

Antes de «Aplicar todo», Gately comprueba si las direcciones de escucha de las reglas y listeners en espera se pueden usar en este equipo y, si no, las enumera con el motivo del sistema para que decidas; el aviso antes de guardar también señala una nueva dirección de escucha inutilizable. Si el cambio no se puede confirmar o ya se revirtió, el diálogo y el aviso de reversión enumeran los errores que informaron los módulos entretanto, por ejemplo un listener que no arranca. Ten en cuenta que macOS solo configura 127.0.0.1 por defecto; para usar 127.0.0.2 u otra dirección de loopback ejecuta antes `sudo ifconfig lo0 alias 127.0.0.2`. En Linux todo 127.0.0.0/8 funciona tal cual.

Los ajustes globales no forman parte de los lotes y se siguen aplicando al momento. Si Gately se reinicia durante un lote, arranca con la configuración nueva y espera la confirmación como en un guardado individual. La API es `POST /api/system/changeset/begin`, `/apply` y `/discard`.

### Reversión desde la línea de comandos {#rollback-cli}

Si la interfaz de administración ya no es accesible, ejecuta `gately rollback` en el servidor (Docker: `docker exec -it gately gately rollback`). Enumera el cambio pendiente y los guardados recientes y te deja elegir uno por número para deshacerlo, o puedes usar opciones:

| Comando | Efecto |
| --- | --- |
| `gately rollback -list` | Solo enumera, sin cambiar nada |
| `gately rollback -last` | Deshace el guardado más reciente: devuelve esa configuración a su contenido anterior |
| `gately rollback -pending` | Revierte ya el cambio de configuración pendiente |
| `gately rollback -config portforward -version <versión>` | Restaura una configuración a una versión del historial (véase `-list`) |

Se confirma escribiendo `yes`; en scripts añade `-y`. Si Gately se está ejecutando, lo hace él y se reinicia; si no, se reescriben directamente los archivos y se aplica en el próximo arranque. El contenido anterior también queda en el [Historial de configuración](/es/docs/guide/audit), así que la reversión puede deshacerse. Usa `-c` si el directorio de configuración no está en la ubicación por defecto.

## Copia de seguridad y restauración

«Ajustes» ofrece la copia de seguridad y la restauración de la configuración, y es también el punto de entrada para importar configuraciones de Lucky.

Al restaurar puede marcar «Conservar la cuenta de administrador actual» (parámetro de API `keepAccount=1`): se ignoran `account.json`, `tokens.json` y el inicio de sesión de terceros de los ajustes del sistema de la copia; la cuenta, la contraseña, la verificación en dos pasos, los usuarios de solo lectura, los tokens de API y el inicio de sesión de terceros en la interfaz de administración actuales no cambian, y el resto de ajustes se toma de la copia, y también se puede restaurar una copia sin `account.json`, lo que resulta útil para traer la configuración de otra instancia. Mientras se use la contraseña predeterminada, la restauración se rechaza; cambie antes la contraseña.
