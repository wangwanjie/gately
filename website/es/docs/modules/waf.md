# Cortafuegos WAF

Un cortafuegos de aplicaciones web basado en [Coraza](https://coraza.io/) y OWASP CRS v4, que se puede asociar a los sitios de [Servicios web](/es/docs/modules/web).

## Capacidades

- Conjunto de reglas OWASP CRS v4 integrado, con archivos de reglas que se pueden activar individualmente;
- Reglas SecLang personalizadas;
- Los grupos de reglas se pueden reordenar; los registros de coincidencias guardan la IP del cliente, el dominio, la ruta y el ID de regla, y se pueden filtrar por dominio, IP o palabras clave;
- Modo de bloqueo y modo de solo detección;
- Límite de inspección del cuerpo de la solicitud ajustable: al superarlo se rechaza la solicitud o se inspecciona solo la primera parte; también se puede omitir la inspección del cuerpo por ruta;
- Modo de compatibilidad con WebDAV: permite los métodos, las cabeceras y cualquier tipo de contenido de subida que usan las nubes personales y WebDAV;
- Puede usar un motor de detección SafeLine autoalojado, solo o junto con grupos de reglas;
- Varias instancias, a las que los sitios hacen referencia según necesiten.

## Recomendaciones

1. En un sitio nuevo, empieza con «Solo detectar» y observa los registros durante un tiempo para confirmar que no hay falsos positivos;
2. Ajusta los archivos de reglas o añade excepciones personalizadas para los falsos positivos;
3. Después cambia a «Bloquear».

## Orden de los grupos de reglas

Los grupos de reglas de una instancia se cargan de arriba abajo; usa los botones subir y bajar para reordenarlos. Dentro de la misma fase, las reglas anteriores se ejecutan primero:

- Las reglas de bloqueo (deny y similares) funcionan en cualquier posición;
- Las reglas de permiso o exclusión en tiempo de ejecución (`allow`, `ctl:ruleRemoveById`, `ctl:ruleRemoveTargetById`, etc.) deben ir encima del grupo OWASP CRS, o CRS puede bloquear la solicitud antes;
- Las directivas de configuración como `SecRuleRemoveById` y `SecRuleUpdateTargetById` solo afectan a reglas ya cargadas, así que deben ir debajo del grupo CRS; encima no tienen efecto.

## Sitios con subidas de archivos grandes

Por defecto, el WAF solo inspecciona cuerpos de solicitud de hasta 12,5 MB y rechaza las solicitudes mayores (413), por lo que las nubes personales, WebDAV y los gestores de archivos no pueden recibir archivos grandes. Puedes ajustarlo en la sección «Inspección del cuerpo de la solicitud» de la instancia:

- **Límite de inspección**: 0 significa el valor predeterminado de 12,5 MB; el máximo es 1024 MB;
- **Al superar el límite**: «Rechazar la solicitud» mantiene el comportamiento anterior; «Inspeccionar solo la primera parte» inspecciona solo la parte dentro del límite y reenvía el resto al backend sin cambios, lo adecuado para sitios con subidas grandes;
- **Compatible con WebDAV y subidas**: por defecto, OWASP CRS solo permite GET, HEAD, POST y OPTIONS, y bloquea las cabeceras `Lock-Token`, `If` y `Content-Range` y los tipos de contenido fuera de su lista permitida, por lo que los clientes WebDAV no pueden listar carpetas, subir archivos ni bloquearlos. Al activarlo se permiten PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK y otros métodos, además de esas cabeceras, y se deja de restringir el tipo de contenido de las subidas; el cuerpo de PUT y de `application/octet-stream` se trata como contenido de archivo y ya no se inspecciona como parámetros de formulario. Las comprobaciones de inyección, path traversal y otros ataques siguen activas;
- **Rutas sin inspección del cuerpo**: un prefijo de ruta por línea (como `/api/upload` o `/remote.php/dav/`); en las solicitudes coincidentes se omite la inspección del cuerpo, y las cabeceras, la URL y los parámetros se siguen inspeccionando con normalidad.

Se recomienda crear una instancia aparte para las nubes personales y los sitios WebDAV: activa «Compatible con WebDAV y subidas» y elige «Inspeccionar solo la primera parte»; si hace falta, añade las rutas de subida a las rutas sin inspección. Los demás sitios siguen con los ajustes predeterminados.

curl, rclone y otros clientes envían la cabecera `Expect: 100-continue` al subir contenido de cierto tamaño, y CRS 4 la bloquea por defecto. Todas las instancias de Gately permiten esa cabecera (Gately gestiona el 100-continue por su cuenta), así que no hace falta configurar nada.

## Usar SafeLine

[SafeLine](https://waf.chaitin.com/) es un firewall de aplicaciones web de Chaitin Tech que detecta ataques con un motor de análisis semántico y genera pocos falsos positivos. Gately puede enviar las solicitudes a tu motor de detección SafeLine autoalojado (safeline-detector) y permitirlas o bloquearlas según el resultado; los puntos de entrada, los certificados, la autenticación y los bloqueos siguen en Gately, así que SafeLine ya no necesita ser una capa más de proxy inverso.

1. Instala SafeLine Community Edition siguiendo su documentación oficial; el directorio de instalación predeterminado es `/data/safeline`.
2. Elige cómo conectar con el motor de detección:
   - **Socket Unix** (Gately y SafeLine en la misma máquina): el motor escucha por defecto en `/data/safeline/resources/detector/snserver.sock`, así que usa la dirección `unix:/data/safeline/resources/detector/snserver.sock`. Si Gately se ejecuta en Docker, monta el directorio `/data/safeline/resources/detector` en el contenedor de Gately y usa la ruta dentro del contenedor.
   - **TCP**: añade las dos líneas siguientes a `/data/safeline/resources/detector/detector.yml` y ejecuta `docker restart safeline-detector`. Tras pasar a TCP, el motor deja de escuchar en el socket Unix.

     ```yaml
     bind_addr: 0.0.0.0
     listen_port: 8000
     ```

     Usa como dirección la IP y el puerto del contenedor safeline-detector, por ejemplo `172.22.222.5:8000`; `docker inspect safeline-detector | grep IPAddress` muestra la IP del contenedor. Si Gately se ejecuta en una red bridge de Docker, únelo a la red de SafeLine con `docker network connect safeline-ce <nombre del contenedor de Gately>`. El motor de detección no tiene autenticación: no expongas el puerto 8000 a Internet.
3. En la instancia de WAF, activa "Usar SafeLine", introduce la dirección y pulsa "Probar conexión". Gately envía un latido y después una solicitud de inyección SQL de ejemplo para inspección; "se detectó el ataque de ejemplo" indica que el motor funciona. Este ataque de ejemplo aparece en los registros de ataques de la consola de SafeLine con origen 127.0.0.1 y dominio gately-test.invalid.
4. Haz referencia a la instancia desde un sitio del Servicio web.

Notas:

- Con SafeLine conectado, los grupos de reglas pueden quedar vacíos; si los mantienes, las solicitudes pasan primero por SafeLine y después por los grupos de reglas. En el modo "Solo detectar", los ataques detectados por SafeLine solo se registran y la solicitud continúa.
- Solicitudes bloqueadas: los navegadores ven una página de bloqueo con el ID de evento y los demás clientes reciben el mismo JSON que los plugins oficiales de SafeLine (con `event_id`). El código de estado es el que indica el motor, normalmente 403. Los bloqueos cuentan en los registros de acceso (`deny=waf`), las estadísticas del sitio y los eventos de bloqueo del WAF, y el preajuste "Bloqueos del WAF" de la Protección contra intrusiones también se aplica.
- Los ataques también quedan registrados en la consola de SafeLine y se pueden buscar por ID de evento; estos registros no pertenecen a ningún sitio de SafeLine.
- Por defecto se envían a inspección hasta 1024 KB del cuerpo de la solicitud; el resto no se inspecciona y se reenvía al backend sin cambios. "Rutas sin inspección del cuerpo" y "Compatible con WebDAV y subidas" también se aplican a SafeLine; los cuerpos gRPC sobre HTTP/2 de longitud desconocida no se inspeccionan.
- El tiempo de espera de detección es de 1 segundo por defecto. Si el motor no está disponible (fallo de conexión o tiempo agotado), las solicitudes se permiten por defecto, la instancia muestra "SafeLine caído" y se registra un error; con "Rechazar solicitudes" esas solicitudes reciben 503. Justo después de arrancar, el motor puede devolver resultados no reconocidos durante unos segundos, que se tratan igual.
- Solo se inspeccionan las solicitudes; las respuestas no se envían a SafeLine.

## Pruebas

El WAF se prueba con cargas de ataque reales, que cubren el bloqueo de path traversal y ataques similares.
