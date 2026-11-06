# Paquete de diagnóstico

Cuando Gately no funciona como debe, exporta un paquete de diagnóstico desde **Ajustes → Diagnóstico** y adjúntalo a tu [issue de GitHub](https://github.com/wangwanjie/gately/issues/new). Ayuda a los mantenedores a encontrar la causa sin pedirte capturas de pantalla ni registros copiados a mano.

El paquete se genera en tu propia máquina y nunca se sube a ningún sitio. Pulsa **Previsualizar contenido** antes de descargarlo para ver exactamente qué se exportará. El zip contiene solo texto plano y JSON, así que también puedes descomprimirlo y revisar cada archivo.

## Qué se exporta

| Archivo | Contenido |
| --- | --- |
| `README.txt` | Una descripción bilingüe del paquete |
| `summary.json` | Versión e información de compilación de Gately, sistema operativo y arquitectura, modelo y número de núcleos de la CPU, totales de memoria / disco, tiempo en funcionamiento, estadísticas del runtime de Go, números de puerto de la interfaz de administración, **nombres** de las interfaces de red con las **categorías** de sus direcciones (por ejemplo `private: 1, public: 2`) |
| `modules.json` | Lista de módulos y el resumen de estado que se muestra en la página de resumen |
| `config/*.json` | **Estructura** de la configuración: nombres de campos, números, interruptores y valores enumerados como el protocolo y el modo |
| `logs/*.log` | Registros de ejecución recientes del sistema y de cada módulo, incluidos los guardados en disco antes de un reinicio (se puede desactivar) |
| `logs/crash.log` | Registro de salidas anómalas: mensaje y pilas de los fallos del runtime de Go (panic, fatal error) y la hora aproximada en que se terminó el proceso a la fuerza (por ejemplo, por falta de memoria con el OOM killer); solo aparece tras una salida anómala (se puede desactivar) |

## Qué no se exporta nunca

- Contraseñas, hashes de contraseñas, tokens de API, secretos TOTP, claves de sesión, secretos de cliente OAuth, claves de proveedores de DNS, contraseñas y claves privadas SSH, certificados y claves privadas. `account.json` y `tokens.json` nunca se leen.
- Nombres de dominio, nombres de host, direcciones IP reales y direcciones MAC. Se sustituyen por marcadores como `<domain>`, `<ipv4:private>`, `<ipv4:public>` y `<mac>`; los puertos se conservan.
- La ruta del directorio de configuración y tu directorio personal (se sustituyen por `<config-dir>` y `<home>`).
- Nombres, notas y cualquier otro texto que hayas escrito en la configuración: toda cadena que no sea un valor enumerado conocido pasa a ser `<redacted>`.
- Contenido de archivos, contenedores, imágenes y variables de entorno de Docker, sesiones de terminal y contenido del tráfico.

## Cómo funciona el ocultamiento

- **Configuración**: los valores se eliminan por defecto. Solo se conservan los nombres de campos, los números, los booleanos, una lista fija de campos enumerados (`protocol`, `mode`, `provider`, …) y las marcas de tiempo. Los campos de dirección conservan solo el puerto y la categoría de la IP.
- **Registros**: se enmascaran las direcciones IP, los dominios, los correos electrónicos, las URL, los pares del tipo `password=…`, los tokens `Bearer`, las cadenas largas con aspecto de token, las direcciones MAC y los directorios de usuario. Los dominios de servicios públicos como `github.com` y `letsencrypt.org` se conservan porque indican si un fallo es un problema de red.
- Un nombre que hayas escrito (por ejemplo la nota de una regla) todavía puede aparecer en una línea de registro si no parece una IP o un dominio. Echa un vistazo a `logs/*.log` antes de publicarlo y quita lo que no quieras compartir.

## Diagnosticar salidas anómalas

Mientras se ejecuta, Gately escribe los detalles de los fallos en `crash.log` dentro del directorio de configuración. Si el proceso termina de forma inesperada, en el siguiente arranque el registro del sistema indica si la última ejecución falló o fue terminada a la fuerza:

- **Fallo**: `crash.log` contiene la causa y las pilas de todas las goroutines; exporta el paquete de diagnóstico y adjúntalo a la issue. En errores fatales como `concurrent map writes`, Go imprime la línea `fatal error: …` solo en la salida de error estándar, que puedes ver con `docker logs`.
- **Terminación inesperada**: el proceso no dejó nada, normalmente porque el sistema lo terminó por falta de memoria (OOM), se terminó con `kill -9` / `docker kill` o hubo un corte de luz. Ejecuta `dmesg -T | grep -i -E "oom|killed process"` en el host para comprobar si fue OOM.

## API

`GET /api/system/diagnostics` (solo con sesión iniciada; los tokens de API se rechazan). `?logs=0` excluye los registros y `?preview=1` devuelve los archivos como JSON.
