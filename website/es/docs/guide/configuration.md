# Configuración y parámetros de ejecución

## Opciones de línea de comandos

| Opción | Descripción |
| --- | --- |
| `-c` | Directorio de configuración; también se puede definir con la variable de entorno `GATELY_CONFIG_DIR` |
| `-listen` | Sobrescribe la dirección de escucha de la interfaz de administración, p. ej. `:16680` |
| `-apply` | Directorio de configuración declarativa; también se puede definir con la variable de entorno `GATELY_APPLY_DIR`. Consulta más abajo |
| `-version` | Muestra la información de versión |

| Variable de entorno | Descripción |
| --- | --- |
| `GATELY_INITIAL_PASSWORD` | Contraseña inicial del administrador en el primer arranque (8 caracteres como mínimo); si no se define, se usa la contraseña predeterminada `gately.cn`. No tiene efecto si la cuenta ya existe |

## Directorio de configuración

Toda la configuración vive en el directorio de configuración, con un archivo de texto plano `<nombre>.json` por módulo, escrito de forma atómica con permisos 0600. Esto te permite:

- Inspeccionarlo con cualquier editor de texto;
- Guardar el directorio en git, auditar cada cambio con diferencias y revertir en cualquier momento;
- Hacer copias de seguridad simplemente copiando el directorio.

::: warning Nota
`ssl.json` contiene claves privadas de certificados. Antes de subirlo al control de versiones, asegúrate de que el repositorio sea privado o añade el archivo a tu lista de ignorados.
:::

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

Los registros de ejecución del sistema y de cada módulo se escriben por defecto en `logs/` dentro del directorio de configuración, así que siguen disponibles en la página de registros del sistema y en la de cada módulo después de reiniciar o actualizar:

- Un archivo JSON Lines por origen: `logs/<ID del módulo>.log`, y los del sistema en `logs/gately.log`. Al arrancar se restauran desde el archivo las entradas más recientes, hasta «Líneas de registro conservadas»;
- Las entradas nuevas se escriben por lotes con un retraso máximo de 1 segundo. Cuando un archivo supera el límite (2 MB por defecto) se rota a `.1` y solo se conserva ese archivo anterior, de modo que cada origen ocupa como máximo el doble del límite;
- En Ajustes → Red y ejecución → Parámetros de ejecución puedes desactivar «Guardar registros en disco» o cambiar «Límite del archivo de registro», con efecto inmediato. Si se desactiva, los registros solo se guardan en memoria y se borran al reiniciar;
- Borrar los registros de un módulo en la interfaz también elimina sus archivos;
- Las copias de seguridad de la configuración no incluyen `logs/` ni el registro de auditoría `audit.log`.

### Ver y filtrar {#log-filter}

La página Registros del sistema y la tarjeta de registros al final de cada página de módulo usan el mismo visor. Las siguientes condiciones se pueden combinar; los cambios se aplican al instante y solo se añaden las entradas nuevas que coinciden:

- Nivel y origen (elija un origen que ya haya aparecido o escríbalo);
- Palabras clave: los términos separados por espacios deben coincidir todos, las comillas dobles agrupan una frase y un `-` inicial excluye un término, por ejemplo `GET "timeout" -debug`; las coincidencias se resaltan. Si la interfaz está en otro idioma, busque con el texto traducido que ve en pantalla;
- Excluir: se ocultan las entradas que contienen cualquiera de sus términos, junto con las palabras clave; se escribe igual (separado por espacios, comillas dobles para frases), por ejemplo `debug "health check"`. Para excluir un término que empieza por `-`, póngalo entre comillas dobles, como `"-v"`;
- IP en Más filtros: dirección completa, rango (como `10.0.0.0/8`) o parte de una dirección, separadas por comas; cuando una entrada no tiene un campo IP propio, las direcciones se reconocen en el texto, así que también sirve para inicios de sesión fallidos y registros de protección contra intrusiones; el prefijo `!` o `-` excluye, por ejemplo `!1.2.3.4, !10.0.0.0/8` oculta esos orígenes, y se puede combinar con elementos incluidos (`10.0.0.0/8, !10.0.0.5`);
- Intervalo de tiempo en Más filtros: desde los últimos 15 minutos hasta los últimos 7 días, o un inicio y fin personalizados;
- La barra del Servicio Web también tiene dos interruptores de visualización que solo afectan a lo que se muestra (los registros se conservan y la elección se recuerda en el navegador): Ocultar orígenes bloqueados oculta las solicitudes rechazadas porque el origen está bloqueado (detrás de una CDN, los bloqueos del kernel no se aplican a las conexiones de la CDN, así que los visitantes bloqueados se rechazan en la capa web y dejan entradas 403 / 444), y también todas las solicitudes de los orígenes que siguen bloqueados, incluidas las que provocaron el bloqueo (vuelven a mostrarse cuando el bloqueo se levanta o caduca); Ocultar solicitudes de la consola oculta las solicitudes con una sesión de la consola de administración de esta instancia, como el sondeo de la página de registros al abrir la consola a través de un sitio del Servicio Web, y está activado por defecto. Estas entradas llevan la etiqueta «Bloqueado por=Bloqueado» o «Usuario de la consola=…», y los parámetros de la API son `hideBanned=1` y `hideAdmin=1` (solo cuentan las sesiones de administrador, no las cuentas de solo lectura; las solicitudes bloqueadas nunca llevan la etiqueta de usuario de la consola);
- Los registros del Servicio web también se pueden filtrar por nombre de sitio o dominio y por código de estado (`404`, `4xx`, `500-599`, separados por comas); los del Cortafuegos WAF, por dominio, y las detecciones registran el dominio y el ID de la regla.

Haga clic en un origen del registro, o en una etiqueta como «Sitio=…» o «Dominio=…» al final de una línea, para filtrar por ese valor; en los registros del Servicio Web, «Sitio=…» filtra por el nombre del sitio con coincidencia exacta y la barra muestra una etiqueta «Sitio: …» que se puede quitar (parámetro de la API `site`), de modo que las entradas de sitios con nombres parecidos no se mezclan. Cuando las entradas en memoria no bastan, Cargar registros anteriores continúa por el historial guardado en los archivos de registro, por lo que con la persistencia de registros activada el filtrado no está limitado por el número de líneas conservadas. `GET /api/logs` y `GET /api/<módulo>/logs` aceptan los mismos parámetros de filtro; consulte la [referencia de la API](/es/docs/reference/api-reference).

## Copia de seguridad y restauración

«Ajustes» ofrece la copia de seguridad y la restauración de la configuración, y es también el punto de entrada para importar configuraciones de Lucky.
