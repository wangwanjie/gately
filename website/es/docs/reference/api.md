# Tokens de API y OpenAPI

Todas las API que usa la interfaz de administración de Gately se pueden llamar por HTTP. Los scripts y la automatización se autentican con tokens de API, y la referencia de la API se genera automáticamente a partir de las rutas registradas.

## Tokens de API

Ruta en la interfaz: grupo «Sistema» de la barra lateral → «Tokens de API».

### Crear un token

Pulsa «Nuevo token» y rellena:

| Elemento | Descripción |
| --- | --- |
| Nombre | Obligatorio, hasta 64 caracteres, identifica el propósito, como «script de monitorización» |
| Caducidad | Elige «No caduca», 7 días, 30 días, 90 días, 1 año o una fecha personalizada; una hora personalizada debe ser posterior al momento actual |
| Permisos | Obligatorio, selecciona al menos uno. Un token nuevo no tiene permisos por defecto |

Cada instancia puede almacenar hasta 100 tokens.

Tras crearlo, el cuadro de diálogo muestra el token en texto plano una sola vez, con el formato `gtl_` seguido de 48 caracteres hexadecimales. En el disco solo se guarda el hash SHA-256. Si lo pierdes, solo puedes «Regenerarlo», y el valor anterior deja de funcionar de inmediato. La lista identifica los tokens con una pista con la forma `gtl_xxxx…yyyy` y muestra la hora del último uso y la IP de origen (se guardan en el disco con un retraso de hasta un minuto).

Al editar un token puedes cambiar su nombre, permisos, caducidad e interruptor de activación. Los cambios surten efecto de inmediato y el token en texto plano no cambia. Un token caducado puede conservar su caducidad original mientras cambias solo los demás campos.

### Autenticación

Envía `Authorization: Bearer <token>` en la cabecera de la petición:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status
```

Los cuerpos de las peticiones usan `Content-Type: application/json`, y los errores se devuelven de forma uniforme como `{"error": "mensaje"}`. Si un token está desactivado, caducado o es desconocido, la petición se comprueba igualmente como sesión de inicio de sesión y termina con un error de no autenticado.

### Alcances

Un permiso tiene la forma `<alcance>:read` o `<alcance>:write`, y write incluye read. Un alcance es el ID de un módulo (como `ddns`, `portforward`, `notify`, `backup`) o uno de los cinco alcances integrados:

| Alcance | Nombre | Permisos disponibles | Endpoints |
| --- | --- | --- | --- |
| `status` | Estado del sistema | `status:read` | `/api/status`, `/api/system/info`, `/api/system/update` |
| `logs` | Registros del sistema | `logs:read` | `/api/logs` |
| `import` | Importación de configuración de Lucky | `import:write` | `/api/import/lucky` |
| `fileusers` | Usuarios compartidos de los servicios de archivos | `fileusers:read`, `fileusers:write` | `/api/fileusers` |
| `metrics` | Métricas globales (Prometheus) | `metrics:read` | `/api/metrics` |

Comodines:

| Forma | Significado |
| --- | --- |
| `*:read` | Acceso de solo lectura a todos los módulos |
| `*:write` o `*` | Acceso de lectura y escritura a todos los módulos |
| `ddns:read` | Acceso de solo lectura únicamente a DDNS |

El permiso que necesita un endpoint lo determinan su ruta y su método:

- Una ruta `/api/<módulo>/...` necesita un permiso del alcance `<módulo>`: GET y HEAD requieren `read`, y el resto de métodos requieren `write`.
- Los registros de módulo `GET /api/<módulo>/logs` son de solo lectura, y borrar los registros (DELETE) es una operación de escritura.
- Los siguientes endpoints GET devuelven contenido sensible o abren un canal interactivo, por lo que se tratan como `write`: los endpoints cuya ruta contiene un segmento `ws` (terminal WebSocket), `exec`, `download`, `export`, `read`, `config`, `preview-archive`, `files`, `sftp` o `secrets`, por ejemplo la descarga de certificados, la ejecución de comandos en un contenedor y la lectura de archivos. Las contraseñas y claves enmascaradas se pueden obtener en texto plano desde el endpoint `.../secrets` del recurso correspondiente; requiere permiso de escritura, y las cuentas y los tokens de solo lectura reciben un 403. Los detalles del contenedor en `/api/docker/containers/{id}` solo requieren `docker:read`; los valores de las variables de entorno y los comandos de inicio se enmascaran sin `docker:write`, y el texto plano se obtiene de `/api/docker/containers/{id}/secrets`.
- Los endpoints antiguos `PUT /api/webdav/shared-users`, `PUT /api/ftp/shared-users` y `PUT /api/smb/shared-users` modifican la misma lista de usuarios compartidos por los tres servicios, por lo que requieren `webdav:write`, `ftp:write` y `smb:write` a la vez; el endpoint nuevo `/api/fileusers` usa el alcance `fileusers`.
- `GET /api/session` y `GET /api/openapi.json` aceptan cualquier token válido.
- El acceso de escritura a los siguientes módulos puede ejecutar comandos en el servidor, lo que equivale a ejecutar comandos como el usuario del proceso de Gately: `ddns` (origen de IP por comando), `cron` (tareas programadas), `guard` (origen de registros por comando), `stun` (scripts de regla), `ssl` (scripts de asignación y comandos de despliegue local), `webterminal`, `docker` (ejecución en contenedores y Compose), `frp`, `filebrowser` y `cloudflared` (rutas de programa personalizadas). Al crear un token, estos módulos llevan la marca «Ejecuta comandos»; concédelos solo a scripts de plena confianza. Cuando un token llama a la importación de Lucky (`import:write`), los elementos importados quedan siempre desactivados; `enable=1` solo se aplica a sesiones iniciadas.

### Operaciones que los tokens no pueden realizar

Los siguientes endpoints solo aceptan sesiones de inicio de sesión, y cualquier acceso con token devuelve 403:

- Cuenta y autenticación de dos factores: `/api/account`
- Gestión de tokens: `/api/tokens`
- Registro de auditoría e historial de configuración: `/api/audit`, `/api/history`
- Ajustes del sistema, token del sistema, reinicio, actualización en línea: `/api/system/settings`, `/api/system/token`, `/api/system/restart`, `/api/system/upgrade`
- Paquete de diagnóstico, autocomprobación y configuración inicial: `/api/system/diagnostics`, `/api/system/selfcheck`, `/api/system/onboarding`
- Descarga y restauración de la copia de seguridad de la configuración: `/api/system/backup`, `/api/system/restore`
- Descarga de archivos de copia locales: `/api/backup/local`
- Ajustes de seguridad del terminal web: `/api/webterminal/security`
- Inicio y cierre de sesión: `/api/login`, `/api/logout`

Una petición con permisos insuficientes devuelve `token lacks permission <permiso>` y registra un aviso en los registros del sistema, con el nombre del token, la IP de origen y el endpoint rechazado.

### Token heredado

El token global único de versiones anteriores se migra automáticamente al actualizar a un token llamado «Token heredado (migrado de una versión anterior)» con el permiso `*`, marcado como «Heredado» en la lista. Crea tokens nuevos con solo los permisos que necesiten y elimínalo. «Regenerar» en el token heredado también quita la marca de heredado.

::: tip Recomendación
Crea un token independiente para cada script, selecciona solo los permisos que necesite y define una caducidad. Si se filtra un token, elimina o regenera solo ese.
:::

## OpenAPI y documentación de la API

### La página de documentación en la interfaz de administración

Grupo «Sistema» de la barra lateral → «Documentación de la API». La página muestra la URL base y las notas de autenticación, lista todos los endpoints agrupados por función y admite la búsqueda por ruta o resumen. Al expandir un endpoint se muestran el permiso necesario, los parámetros, los campos del cuerpo de la petición y un ejemplo de curl que se puede copiar. Los endpoints que necesitan una sesión de inicio de sesión se marcan como «Solo sesión de inicio de sesión» y no tienen ejemplo de curl.

### Obtener el documento OpenAPI

Mientras una instancia está en ejecución, `GET /api/openapi.json` devuelve un documento OpenAPI 3.0 que puede leer cualquier token válido o sesión de inicio de sesión:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/openapi.json -o openapi.json
```

El documento usa el esquema de seguridad `bearerAuth` y añade campos de extensión a cada endpoint:

| Campo de extensión | Significado |
| --- | --- |
| `x-required-scope` | El permiso de token necesario para acceder a este endpoint |
| `x-token-allowed` | Si los tokens pueden acceder a él; `false` significa solo sesión de inicio de sesión |
| `x-scope-domains` (raíz del documento) | Lista de todos los alcances |

La lista de endpoints que usan [CLI y MCP](/es/docs/guide/cli-mcp) se lee de este documento.

### Exportación desde la línea de comandos

La opción `-api-docs` compila la aplicación en un directorio de configuración temporal (no escucha en ningún puerto y no afecta a tu configuración actual), escribe los documentos en el directorio indicado y termina:

```bash
gately -api-docs ./api-docs
```

Genera dos archivos:

- `openapi.json`: la descripción OpenAPI 3.0
- `API.md`: la documentación de la API en Markdown, con un inicio rápido, una tabla de alcances y tablas de endpoints agrupados por función

## Ejemplos

```bash
# Ver el estado del sistema (requiere status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# Listar todas las tareas DDNS (requiere ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Sincronizar una tarea DDNS de inmediato (requiere ddns:write; toma la clave de la lista devuelta en el paso anterior)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/KEY/sync

# Obtener solo los registros del módulo con un número de secuencia mayor que 10 (requiere ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?since=10"

# Obtener solo las 200 entradas más recientes; pasar el número más bajo como before para ver las anteriores
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?limit=200"
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?before=1200&limit=200"
```

## Idioma de las respuestas

Las respuestas siguen el idioma de la cabecera `Accept-Language`, o el idioma del servidor (el idioma de la interfaz en Ajustes) si falta. Solo se traduce el texto generado por Gately: respuestas de error, estado y registros, descripciones de proveedores y permisos, y los campos de mensaje de los datos de configuración como `error`, `lastError`, `details`, `warnings`, `message` y `title`. Los nombres, notas, grupos, comandos, dominios, valores de registro y demás contenido que haya introducido se devuelven siempre tal como se guardaron, así que leerlos y volver a guardarlos nunca los reescribe. Añada `-H "Accept-Language: zh-CN"` si un script necesita los mensajes en chino.

## Referencia completa de la API

Consulta la [Referencia de la API](/en/docs/reference/api-reference) para ver todos los endpoints, parámetros y permisos necesarios.
