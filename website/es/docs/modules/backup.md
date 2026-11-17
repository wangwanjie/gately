# Copia de seguridad de la configuración y copia programada

Toda la configuración de Gately se guarda en archivos `*.json` del directorio de configuración. La función de copia de seguridad empaqueta estos archivos y admite la descarga manual, la escritura programada en varias ubicaciones y la restauración sobrescribiendo con una copia. La interfaz de administración está en el grupo «Sistema» de la barra lateral → «Ajustes» → pestaña «Copia de seguridad y migración».

## Contenido de la copia

Una copia es un archivo zip que contiene todos los `*.json` de la raíz del directorio de configuración (cuenta, reglas de los módulos, certificados, hashes de tokens, etc.). Incluye hashes de contraseñas, secretos, hashes de tokens de API y claves privadas de certificados, así que guárdala con cuidado. Los registros de ejecución (`logs/`) y el registro de auditoría (`audit.log`) no se incluyen.

El formato del nombre de archivo es `gately-backup-YYYYMMDD-HHMMSS.zip`, y las copias cifradas usan `gately-backup-YYYYMMDD-HHMMSS.zip.enc`.

## Copia y restauración manuales

### Descargar una copia

El botón «Descargar copia» de la tarjeta «Copia de seguridad de la configuración de Gately» descarga un zip con la configuración actual. Las copias descargadas manualmente no están cifradas.

### Restaurar

En la tarjeta «Restaurar configuración», elige un archivo de copia, pulsa «Restaurar» y escribe la palabra «restaurar» en el cuadro de confirmación.

- La restauración sobrescribe toda la configuración actual. El límite de tamaño de subida es de 64 MB.
- Antes de sobrescribir, la configuración existente se copia en una carpeta `backup-<hora>` dentro del directorio de configuración (formato de hora `YYYYMMDD-HHMMSS`), y la interfaz muestra esa ruta tras una restauración correcta.
- Gately se reinicia automáticamente tras la restauración.
- Por defecto, la configuración de la cuenta de la copia sobrescribe la cuenta actual, y una copia sin `account.json` se rechaza. Si marcas «Conservar la cuenta de administrador actual», la cuenta no se sobrescribe: la cuenta actual, la contraseña, la autenticación de dos factores, las cuentas de solo lectura, los tokens de API y el inicio de sesión de terceros de la interfaz de administración se mantienen, y la copia puede no incluir `account.json`; es útil para traer las reglas de otra máquina.
- Cuando se restaura la configuración de la cuenta se usa una clave de sesión nueva, y todas las sesiones emitidas antes de la restauración dejan de ser válidas.
- Mientras se siga usando la contraseña predeterminada, la restauración se rechaza; primero hay que cambiar la contraseña de administrador.
- De la copia solo se aceptan los archivos `*.json` situados en la raíz del zip, que no estén ocultos y que contengan JSON válido. El resto de entradas se ignoran.
- Al elegir un archivo `.enc`, aparece un campo «Contraseña de la copia». Introduce la contraseña que se definió al crear la copia. Una contraseña incorrecta o un archivo dañado hacen que la restauración falle.

Las API de descarga y restauración de copias solo aceptan sesiones de inicio de sesión, y los [tokens de API](/es/docs/reference/api) no pueden llamarlas.

## Copia programada

En la tarjeta «Copia programada» situada bajo la pestaña «Copia de seguridad y migración», el interruptor de la parte superior derecha activa o desactiva la función. El ID del módulo es `backup` y los alcances del token son `backup:read` y `backup:write`.

| Ajuste | Descripción |
| --- | --- |
| Horario | Una expresión cron de 5 campos, por defecto `0 3 * * *` (cada día a las 03:00). También funcionan descriptores como `@daily` y `@weekly` |
| Retención | Conserva las últimas N copias de cada destino, de 1 a 365, por defecto 7 |
| Contraseña de cifrado | Déjala vacía para no cifrar; cuando se define, se generan copias `.zip.enc`. Si ya hay una contraseña definida, dejar el campo vacío la conserva sin cambios |
| Directorio local | Interruptor, activado por defecto. Escribe en la carpeta `backups` del directorio de configuración |

Pulsa «Guardar ajustes» para aplicar el horario y «Copiar ahora» para ejecutar una de inmediato. La tarjeta muestra la hora de la última ejecución, cómo se activó (manual o programada), la hora de la próxima ejecución, el resultado de éxito o fallo de cada destino y el número de copias antiguas eliminadas. Los archivos de copia locales aparecen listados en la tarjeta y se pueden descargar.

### Destinos de la copia

Además del directorio local, puedes añadir varios destinos remotos. Cada uno escribe en un punto de montaje de «Montajes de almacenamiento» (un directorio local, un WebDAV remoto o un recurso compartido SMB):

| Campo | Descripción |
| --- | --- |
| Nombre | Obligatorio |
| Punto de montaje de almacenamiento | Elige un montaje existente de «Montajes de almacenamiento». No se pueden seleccionar los montajes de solo lectura o desactivados |
| Directorio | Subdirectorio dentro del montaje; déjalo vacío para la raíz |
| Activado | Interruptor |

Si todavía no tienes ningún montaje, añade antes uno en «Montajes de almacenamiento». Eliminar un destino no elimina las copias ya escritas.

### Flujo de ejecución

1. Empaqueta el directorio de configuración y lo cifra si hay una contraseña de cifrado definida.
2. Escribe en el directorio local (si está activado) y en cada destino activado, con el mismo nombre de archivo en todos.
3. Tras una escritura correcta, elimina las copias antiguas de esa ubicación según el número de retención. La limpieza solo afecta a archivos cuyo nombre empieza por `gately-backup-` y termina en `.zip` o `.zip.enc`; el resto de archivos se dejan como están.
4. Resume los resultados, los escribe en el registro del módulo y publica un evento de notificación.

Si no hay activados ni el directorio local ni ningún destino, la copia se registra como fallida. Solo se ejecuta una copia a la vez.

### Eventos de notificación

| Evento | Nivel | Desencadenante |
| --- | --- | --- |
| `backup.done` | Info | Todas las ubicaciones se escribieron correctamente; el mensaje lista el nombre del archivo y el resultado de cada ubicación |
| `backup.failed` | Error | Falló el empaquetado o la escritura en alguna ubicación; el mensaje lista el resultado de cada ubicación |

Crea una regla que se suscriba a `backup.*` en el [Centro de notificaciones](/es/docs/modules/notify) para recibir los resultados de las copias por Telegram, correo y otros canales.

## Cifrado

Cuando se define una contraseña de cifrado, las copias se cifran de la siguiente manera:

- Derivación de la clave: scrypt (N=32768, r=8, p=1), que produce una clave de 32 bytes, con una sal aleatoria de 16 bytes.
- Cifrado: AES-256-GCM, con un nonce aleatorio de 12 bytes.
- Formato de archivo: el número mágico `GATELYBK1`, la sal, el nonce y el texto cifrado, concatenados en ese orden. El número mágico también se usa como datos autenticados adicionales de GCM.

La contraseña de cifrado solo se guarda en `backup.json`, dentro del directorio de configuración. Si se pierde la contraseña, la copia no se puede descifrar. La contraseña solo se puede ver y cambiar tras iniciar sesión en la interfaz de administración; los tokens de API no pueden leerla, cambiarla ni borrarla, aunque el resto de ajustes de la copia de seguridad sí se pueden cambiar con un token.

::: warning Nota
Cuando `backup.json` y los archivos de copia están en la misma máquina, la protección por contraseña aporta poco valor. Escribe las copias cifradas en un montaje de otro dispositivo y guarda la contraseña por separado.
:::
