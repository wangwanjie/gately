# CLI y MCP

El ejecutable `gately` incluye dos subcomandos: `gately ctl` llama desde la terminal a la API de administración de una instancia en ejecución, y `gately mcp` ofrece esos mismos endpoints como herramientas MCP para clientes de IA. Ambos acceden a la instancia por HTTP con un [token de API](/es/docs/reference/api); la lista de endpoints procede del `/api/openapi.json` de la instancia y los permisos los determina el token.

Además, `gately reset-password` y `gately reset-totp` trabajan directamente sobre el directorio de configuración local, sin pasar por la API, para recuperar la cuenta de administrador si olvidaste la contraseña o perdiste el autenticador; consulta [Restablecer la contraseña y la autenticación de dos factores](/es/docs/guide/install#reset).

`gately service install|uninstall|start|stop|restart|status` registra Gately como servicio del sistema que se inicia con el equipo; consulte [Inicio automático](/es/docs/guide/install#service).

## Configuración de la conexión

Los datos de conexión se determinan en el siguiente orden, y cada paso sobrescribe al anterior:

1. La dirección por defecto `http://127.0.0.1:16680`
2. El archivo de configuración `ctl.json`
3. Las variables de entorno `GATELY_URL` y `GATELY_TOKEN`
4. Las opciones de línea de comandos `-url` y `-token`

El archivo de configuración está en `gately/ctl.json` dentro del directorio de configuración del usuario del sistema (`~/.config/gately/ctl.json` en Linux y `~/Library/Application Support/gately/ctl.json` en macOS). Ejemplo de contenido:

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

`gately ctl login` genera este archivo.

## gately ctl

### Subcomandos

| Comando | Descripción |
| --- | --- |
| `gately ctl ls [palabra clave]` | Lista los endpoints a los que puede acceder el token, con método, ruta y resumen; se puede filtrar por palabra clave |
| `gately ctl get <ruta>` | Envía una petición GET |
| `gately ctl post <ruta> [JSON o @archivo]` | Envía una petición POST |
| `gately ctl put <ruta> [JSON o @archivo]` | Envía una petición PUT |
| `gately ctl delete <ruta> [JSON o @archivo]` | Envía una petición DELETE |
| `gately ctl login <dirección> <token>` | Valida la conexión y la guarda en el archivo de configuración |

La ruta debe empezar por `/api/`. El cuerpo de la petición puede ser un literal JSON, `@nombre-de-archivo` (lee un archivo) o `-` (lee la entrada estándar), y debe ser JSON válido. El método `patch` también está disponible, y `ls` se puede escribir como `list`.

### Opciones de conexión

Las opciones se escriben antes del subcomando:

| Opción | Descripción |
| --- | --- |
| `-url <dirección>` | Dirección de administración, que empieza por http o https |
| `-token <token>` | Token de API |
| `-insecure` | Omite la verificación del certificado HTTPS, útil para instancias internas con certificado autofirmado |
| `-raw` | Imprime la respuesta tal cual; por defecto se formatea el JSON |

Si el comando termina bien, el código de salida es 0; si el endpoint devuelve un estado distinto de 2xx, imprime `Error: HTTP <código de estado>: <descripción>` y termina con 1; los problemas de argumentos terminan con 2.

### Ejemplos

```bash
# Guardar la configuración de conexión (primero solicita /api/session para validar el token)
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# Ver los endpoints accesibles para el token, filtrando las líneas que contienen ddns
gately ctl ls ddns

# Leer el estado del sistema
gately ctl get /api/status

# Despertar un dispositivo
gately ctl post /api/wol/devices/abc123/wake

# Desactivar una tarea DDNS
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# Leer el cuerpo de la petición desde un archivo
gately ctl post /api/ddns/tasks @task.json

# Usar variables de entorno de forma temporal
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip Consejo
`ls` lista los endpoints a los que puede acceder el token; los que solo admiten sesiones de inicio de sesión (cuenta, gestión de tokens, copia de seguridad y restauración, etc.) no aparecen. Que se permita una operación de escritura depende de los permisos del token; si son insuficientes se devuelve 403.
:::

## gately mcp

`gately mcp` ofrece un servicio MCP por entrada y salida estándar (JSON-RPC, un mensaje por línea), que el cliente de IA lanza como subproceso.

### Opciones

| Opción | Descripción |
| --- | --- |
| `--url <dirección>` | Dirección de administración |
| `--token <token>` | Token de API |
| `--write` | Expone también las herramientas de escritura |
| `--insecure` | Omite la verificación del certificado HTTPS |

El origen de los datos de conexión es el mismo que en `gately ctl` (archivo de configuración, `GATELY_URL`, `GATELY_TOKEN` y opciones).

### Reglas de generación de herramientas

- Tras arrancar el servicio, cuando el cliente solicita por primera vez la lista de herramientas se lee el documento OpenAPI de la instancia y se genera una herramienta por cada endpoint accesible para el token.
- El nombre de la herramienta se forma con el método y la ruta, como `get_ddns_tasks` o `put_ddns_tasks_key_enable`, con un máximo de 64 caracteres; si hay nombres repetidos se añade un número de orden.
- Los parámetros de ruta se convierten en parámetros de cadena obligatorios; las herramientas GET incluyen un parámetro opcional `query` (como `since=10`); los endpoints con cuerpo incluyen un parámetro `body`, cuya estructura procede de OpenAPI.
- La descripción de la herramienta incluye el resumen del endpoint, el método y la ruta, y el permiso necesario; las herramientas llevan la anotación `readOnlyHint` y las de DELETE llevan `destructiveHint`.
- El contenido devuelto se trunca cuando supera los 200000 caracteres.

### Solo lectura por defecto

Sin `--write` solo se exponen las herramientas GET y HEAD, y quedan fuera los endpoints como `.../secrets` que devuelven contraseñas en texto plano; solo se ofrecen con `--write`. Añade `--write` cuando quieras que la IA pueda modificar la configuración. Aunque actives la escritura, los permisos del propio token siguen vigentes, así que lo más seguro es crear un token exclusivo para MCP con solo los permisos necesarios.

::: warning Atención
`--write` permite al cliente de IA llamar a todos los endpoints de escritura dentro del alcance de los permisos del token, incluido el borrado. Valida primero el flujo con un token de solo lectura y concede permisos de escritura solo cuando haga falta.
:::

### Configuración del cliente

Para clientes como Claude Desktop o Cursor, que usan el formato de configuración `mcpServers`:

```json
{
  "mcpServers": {
    "gately": {
      "command": "gately",
      "args": ["mcp"],
      "env": {
        "GATELY_URL": "http://127.0.0.1:16680",
        "GATELY_TOKEN": "gtl_xxxx"
      }
    }
  }
}
```

Si activas la escritura, cambia `args` a `["mcp", "--write"]`. En `command` puedes poner la ruta absoluta de `gately`.

Claude Code se configura añadiéndolo por línea de comandos:

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```
