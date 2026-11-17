# Auditoría e historial de configuración

Gately maneja claves DNS y claves privadas de certificados, así que debe poder averiguarse quién hizo qué y cuándo, y cómo cambió la configuración, y todo cambio debe poder deshacerse.

## Registro de auditoría

La pestaña «Registro de auditoría» de la página «Sistema → Registros» de la barra lateral muestra las siguientes operaciones de la más reciente a la más antigua. Puedes filtrar por categoría o buscar por ruta, usuario o IP:

| Categoría | Qué se registra |
| --- | --- |
| Inicio de sesión | Inicios de sesión correctos y fallidos con contraseña, passkey o inicio de sesión de terceros, y cierres de sesión |
| Cambio | Todas las operaciones de escritura iniciadas desde la interfaz de administración o con tokens de API (crear, modificar, eliminar, iniciar o detener, revertir, etc.) |
| Acceso sensible | Descarga de copias de seguridad de la configuración, paquetes de diagnóstico o claves privadas de certificados, apertura del terminal web o de un terminal de contenedor, etc. |

Cada entrada incluye la hora, el autor (nombre de usuario o `api-token:<nombre del token>`), la IP de origen, el método, la ruta y el código de estado. **Nunca se registran cuerpos de petición ni secretos**.

- El registro se escribe en `audit.log` (JSON Lines) dentro del directorio de configuración; cuando el archivo supera 5 MiB se rota y se conservan 3 archivos anteriores;
- Si la interfaz de administración está detrás de un proxy inverso local, se registra el origen real y, aparte, la dirección del proxy por el que pasó; el funcionamiento se explica en el [Modelo de seguridad](/es/docs/guide/security);
- Los cambios y los accesos sensibles se emiten como eventos `audit.change` y `audit.access`, que puedes enviar desde el [Centro de notificaciones](/es/docs/modules/notify) con las plantillas «Cambios de configuración» y «Seguridad e inicios de sesión»;
- Las cuentas de solo lectura y los tokens de API no pueden ver el registro de auditoría.

## Historial de configuración y reversión

Antes de sobrescribir un archivo de configuración, Gately guarda el contenido anterior como una versión del historial en `history/<nombre>/` dentro del directorio de configuración, y conserva las 30 más recientes de cada configuración. `account.json` (credenciales de la cuenta) y `tokens.json` (tokens) no se guardan en el historial.

En la pestaña «Historial de configuración» de la página «Sistema → Registros»:

1. Elige un archivo de configuración para ver la fecha de guardado y el tamaño de cada versión;
2. Pulsa «Comparar» para ver las diferencias línea a línea entre esa versión y la configuración actual;
3. Pulsa «Revertir» para restaurar esa versión; Gately se reinicia automáticamente para aplicarla.

El contenido vigente antes de revertir también se guarda como una nueva versión, así que la propia reversión se puede deshacer.
