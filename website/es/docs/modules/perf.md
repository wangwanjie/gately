# Rendimiento

Rendimiento reúne en una página los datos del host, del proceso de Gately y de cada sitio, y envía una alerta por el [Centro de notificaciones](/es/docs/modules/notify) cuando una métrica se mantiene por encima de su umbral, seguida de un aviso de recuperación cuando la condición deja de cumplirse. Está en la barra lateral, en Sistema → Rendimiento.

Cubre casos como una CPU al máximo de madrugada, un pico de conexiones en un sitio o un backend que se vuelve lento sin que nadie se entere, y guarda sus datos solo en memoria. Para historial de más de 24 horas, agregación entre instancias o consultas complejas, usa [Prometheus y Grafana](/es/docs/reference/monitoring).

## Panel

Cambia entre 60 segundos, 1 hora y 24 horas en la esquina superior derecha de la página.

### Host y proceso de Gately

| Gráfico | Contenido |
| --- | --- |
| CPU del host | Uso medio de todos los núcleos de la máquina |
| Memoria / disco del host | Memoria usada respecto a la memoria física (sin la caché recuperable), proporción usada de la partición del directorio de configuración |
| Carga del sistema | Carga de 1 minuto |
| CPU del proceso de Gately | Proporción de toda la máquina que usa el proceso de Gately, medida igual que la CPU del host |
| Memoria del proceso de Gately | Memoria residente (MB) |
| Descriptores de archivo / goroutines | Descriptores de archivo abiertos y goroutines de Go del proceso |

Se toma una muestra cada 5 segundos: el rango de 60 segundos muestra cada muestra; el de 1 hora, un punto por minuto (la media del minuto); el de 24 horas, un punto cada 5 minutos. La línea discontinua de los gráficos de CPU es el pico de cada punto, de modo que los picos breves no se pierden en la media. Pasa el ratón por un gráfico para ver los valores de un momento concreto.

Algunas plataformas no ofrecen todas las métricas (por ejemplo, en macOS no hay uso de CPU); esos gráficos muestran «Sin datos».

### Sitios

Con el módulo Servicios web activado se listan todos los sitios en ejecución (incluidos los sitios por defecto):

| Columna | Significado |
| --- | --- |
| En curso | Peticiones que el sitio atiende ahora mismo, incluidas conexiones largas como WebSocket |
| Peticiones/s | Media de peticiones por segundo en aproximadamente los últimos 60 segundos |
| Proporción 5xx | Respuestas 5xx como porcentaje de las respuestas completadas en aproximadamente los últimos 60 segundos |
| Latencia P95 | Percentil 95 estimado del tiempo hasta el primer byte en aproximadamente los últimos 60 segundos |
| Backends | Backends sanos / todos los backends, y conexiones que se están reenviando |

Haz clic en un sitio para abrir sus [estadísticas](/es/docs/modules/web#site-stats) con las tendencias de 24 horas de peticiones, errores y latencia.

### Tiempo hasta el primer byte

La latencia es el «tiempo hasta el primer byte»: desde la llegada de la petición hasta el envío de las cabeceras de respuesta, lo que refleja la rapidez del backend o del propio sitio. No se cuenta:

- el tiempo de transferencia del cuerpo de la respuesta (las descargas grandes no aumentan la latencia);
- WebSocket y otras peticiones con cambio de protocolo;
- las peticiones cortadas por el control de acceso o por un administrador.

Cada petición cae en uno de 50 tramos finos (de 1 ms a 60 s, con unos 10 % a 25 % de diferencia entre vecinos). Cada tramo acumula el número de peticiones y el tiempo total; los percentiles usan el tiempo medio del tramo en el que caen, así que el error nunca supera el ancho del tramo, y no se guarda nada de peticiones individuales. El histograma `gately_site_request_duration_seconds` exportado a Prometheus usa 13 límites habituales de entre ellos.

## Reglas de alerta

Haz clic en «Añadir regla de alerta». Al crearla puedes pulsar una plantilla para rellenar una regla habitual (CPU del host alta, poco espacio en disco, errores 5xx del sitio, respuestas lentas del sitio, etc.) y ajustarla.

| Campo | Descripción |
| --- | --- |
| Nombre | Obligatorio, hasta 64 caracteres, aparece en el título de la notificación |
| Métrica | Ver la tabla siguiente |
| Umbral | Operador `>`, `>=`, `<` o `<=` más un valor; la unidad depende de la métrica |
| Duración | De 1 a 1440 minutos. La alerta solo se envía cuando la condición se ha mantenido este tiempo; una sola bajada por debajo del umbral reinicia el contador |
| Sitios | Para métricas de sitios y de nodos de backend; vacío para todos los sitios |
| Peticiones mínimas | Para la proporción 5xx y la latencia, 20 por defecto. No se evalúa mientras los últimos ~60 segundos tengan menos peticiones, para que uno o dos errores o peticiones lentas en un sitio con poco tráfico no disparen una alerta |
| Nivel de alerta | Aviso o error; fija el nivel del evento de notificación |
| Nota, habilitar | Opcional |

| Métrica | Unidad | Se evalúa por |
| --- | --- | --- |
| Uso de CPU del host | % | Este host |
| Uso de memoria del host | % | Este host |
| Uso de disco del host (partición del directorio de configuración) | % | Este host |
| Uso de CPU del proceso de Gately | % | Proceso de Gately |
| Memoria residente del proceso de Gately | MB | Proceso de Gately |
| Peticiones en curso del sitio | número | Cada sitio |
| Peticiones por segundo del sitio | pet/s | Cada sitio |
| Proporción de respuestas 5xx del sitio | % | Cada sitio |
| Tiempo hasta el primer byte P95 del sitio | ms | Cada sitio |
| Conexiones del nodo de backend | número | Cada nodo de backend |

Las métricas de sitios y de nodos de backend se evalúan y alertan por objeto, así que una regla «proporción 5xx > 5 %» puede alertar por separado sobre dos sitios. «Peticiones por segundo del sitio < 1» detecta tráfico que se ha detenido de repente.

### Evaluación y notificaciones

1. Todas las reglas habilitadas se evalúan cada 5 segundos.
2. Cuando la condición empieza a cumplirse el objeto pasa a «Pendiente»; tras mantenerse la duración pasa a «Activa» y se publica un evento `perf.alert`.
3. Cuando deja de cumplirse la condición de una alerta activa se publica un evento `perf.recovered`. La proporción 5xx y la latencia de un sitio no se pueden evaluar si los últimos ~60 segundos tienen pocas peticiones; eso también cuenta como recuperación y el aviso lo indica.
4. Si un sitio se elimina o se desactiva, sus alertas desaparecen sin aviso de recuperación.

La parte superior de la página lista los objetos pendientes y activos con su valor actual y su duración.

Cambiar el nombre, el nivel o la nota de una regla mantiene el contador en curso; cambiar la métrica, el umbral, la duración, los sitios o las peticiones mínimas, o desactivar la regla, borra sus contadores y alertas. Una alerta ya disparada recibe un aviso de recuperación que indica que la regla se eliminó, desactivó o modificó. Los contadores y estados de alerta solo viven en memoria y empiezan de nuevo tras un reinicio.

Las alertas se envían por el Centro de notificaciones: añade una regla de notificación que suscriba «Rendimiento: alerta activada» y «Rendimiento: alerta recuperada» (la plantilla «Alertas de rendimiento» lo hace); si no, las alertas solo se escriben en el registro de esta página y la página muestra un aviso. El silencio del Centro de notificaciones también se aplica a las alertas de rendimiento; cada objeto tiene un origen de evento distinto, así que no se silencian entre sí. La duración ya evita alertas intermitentes, por lo que normalmente no hace falta silencio.

## Recursos y límites

- El historial solo se guarda en memoria: el host y el proceso tienen un punto por minuto durante 24 horas; la latencia de los sitios guarda P50 y P95 por minuto, unos 23 KB por sitio con tráfico.
- Hasta 200 reglas de alerta y hasta 200 sitios por regla.
- Con el módulo Servicios web activado, cada petición cuesta una lectura de reloj adicional y unas pocas actualizaciones de contadores.

## Métricas de monitorización

Con Rendimiento activado, `/api/metrics` exporta además las métricas del host y del proceso de la última muestra y el número de alertas activas por regla; consulta [Monitorización](/es/docs/reference/monitoring).

## API

| Endpoint | Descripción |
| --- | --- |
| `GET /api/perf/overview?range=60s\|1h\|24h` | Historial del host y del proceso |
| `GET /api/perf/sites` | Métricas de cada sitio en aproximadamente los últimos 60 segundos |
| `GET /api/perf/alerts` | Objetos pendientes y activos |
| `GET /api/perf/metrics` | Métricas disponibles para alertas |
| `/api/perf/rules` | Crear, leer, actualizar y eliminar reglas de alerta, habilitarlas, ordenarlas y agruparlas |

Leer requiere el permiso `perf:read` y modificar reglas `perf:write`; consulta [Tokens de API](/es/docs/reference/api).
