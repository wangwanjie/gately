# Grupos, orden, operaciones en lote y duplicado de las listas

Las listas con muchas entradas se pueden agrupar, ordenar manualmente, buscar y modificar en lote, y los controles funcionan igual en todas las páginas. El orden y los grupos solo afectan a la visualización en la interfaz de administración; cambiarlos no reinicia las instancias en ejecución ni corta conexiones.

## Listas compatibles

| Página | Listas |
| --- | --- |
| DNS dinámico | Tareas; registros DNS dentro de una tarea |
| Traversal STUN | Reglas |
| Reenvío de puertos | Reglas |
| Certificados SSL | Certificados |
| Servicios web | Sitios dentro de una escucha (las propias escuchas se pueden subir y bajar) |
| Cortafuegos WAF | Instancias |
| Prevención de intrusiones | Reglas de registro |
| frp | Instancias frpc, instancias frps, proxies |
| Túnel de Cloudflare | Túneles |
| File Browser | Instancias |
| Montajes de almacenamiento | Montajes |
| Wake-on-LAN | Dispositivos |
| Tareas programadas | Tareas |
| Notificaciones | Canales, reglas |
| Terminal web | Conexiones guardadas |
| Tokens de API | Tokens |

La mayoría de estas listas ofrecen "Vista de tarjetas" y "Vista compacta", que se cambian en la esquina superior derecha de la lista; en pantallas estrechas se usa siempre la vista de tarjetas.

## Grupos

Rellena "Grupo" en el diálogo de edición (hasta 64 caracteres; puedes elegir un grupo existente). Déjalo vacío para no asignar grupo. En cuanto una entrada tiene grupo, la lista se muestra en bloques plegables por grupo: los grupos aparecen en el orden en que surgen por primera vez, las entradas sin grupo van al final y el estado plegado de cada grupo se guarda en el navegador. El cuadro de búsqueda filtra entradas por nombre, grupo, nota y campos similares, y despliega todos los grupos mientras se busca.

Pasa el ratón sobre el encabezado de un grupo y pulsa el botón del lápiz para renombrarlo: todas las entradas del grupo adoptan el nuevo nombre y conservan su posición y orden. Si le das el nombre de un grupo existente, los dos grupos se fusionan tras confirmar.

## Modo de ordenación

Pulsa "Ordenar y lotes" para entrar en el modo de ordenación. Se muestran todas las entradas y la búsqueda y los filtros quedan en pausa:

- Usa las flechas para subir o bajar entradas dentro de un grupo, o las del encabezado para mover el grupo completo;
- En escritorio puedes arrastrar las entradas directamente; al soltarlas sobre una entrada o el encabezado de otro grupo pasan a ese grupo;
- Tras marcar varias entradas (la casilla del encabezado selecciona el grupo completo), arrastra cualquiera de ellas y todas las seleccionadas se mueven juntas en su orden actual;
- La barra de herramientas permite "Mover al grupo" las entradas seleccionadas en bloque; si se deja vacío, salen de su grupo.

Los teléfonos y otras pantallas estrechas no admiten arrastrar; usa las flechas y la barra de herramientas para hacer lo mismo.

## Operaciones en lote

Tras marcar entradas en el modo de ordenación, la barra de herramientas también permite:

- **Activar / Desactivar**: guarda todas las entradas marcadas de una vez; las que ya están en el estado deseado no se reinician;
- **Eliminar**: el diálogo de confirmación enumera las entradas que se van a eliminar, y la eliminación no se puede deshacer. Si alguna entrada no se puede eliminar (por ejemplo, un certificado que aún usa un sitio), no se elimina ninguna y se muestra el motivo.

Una operación en lote procesa hasta 500 entradas. Solo se guarda cuando todas las entradas superan la validación; si alguna falla, no se modifica nada. Los dispositivos de Wake-on-LAN no tienen interruptor de activación y solo admiten la eliminación en lote; los tokens de API todavía no admiten activar, desactivar ni eliminar en lote. Servicios web ofrece además cambios en lote de dominios y opciones; consulta [Servicios web](/es/docs/modules/web#operaciones-en-lote). Los sitios también se pueden mover a otro listener, consulta [Mover sitios](/es/docs/modules/web#move-sites).

## Duplicar elementos

Todos los elementos de la tabla anterior, salvo los tokens de API, se pueden duplicar, al igual que las listas de IP suscritas, los destinos de copia de seguridad, las credenciales predefinidas del Terminal web, los registros de una tarea DDNS y los sitios de un listener del Servicio web. Pulsa «Duplicar» en un elemento para abrir el diálogo de alta con ese elemento como plantilla; el nombre recibe el sufijo «(copia)». Cambia los ajustes que deban ser distintos y guarda para añadir un elemento nuevo. El original no cambia y la copia se coloca justo después.

- **Los secretos se toman del original**: las contraseñas, tokens y claves privadas guardados siguen ocultos en el diálogo. Si no los cambias, la copia usa los valores del original y el texto plano nunca pasa por el navegador. El botón del ojo muestra el texto plano del original. Para las conexiones y credenciales predefinidas del Terminal web hay que aceptar el acuerdo de usuario y superar la verificación en dos pasos antes de reutilizar secretos.
- **No se copia el contenido que mantiene el sistema**: los certificados obtenidos por ACME o leídos de archivos se emiten o leen de nuevo con la configuración de la copia; un certificado subido se copia junto con su clave privada. Los registros de despliegue de certificados se vacían, las listas de IP suscritas se descargan de nuevo y los enlaces compartidos de los sitios no se copian, porque sus tokens omiten la autenticación.
- **Destinos de despliegue del certificado**: se desactivan en la copia para que no escriba en los mismos archivos que el certificado original. Actívalos tras comprobar que los nombres de archivo no coinciden.
- **Servicio web**: al duplicar un sitio puedes colocarlo en otro listener (por ejemplo, de un listener HTTP a uno HTTPS). Al duplicar un listener puedes elegir si copiar también sus sitios.
- La copia conserva el estado de activación del original. Antes de guardar, cambia los puertos, dominios, registros y demás ajustes que deban diferir para que no entre en conflicto con el original.

## Notas por página

- **DNS dinámico**: "Ordenar registros" en la tarjeta de una tarea abre un diálogo para ajustar el orden y los grupos de los registros DNS de esa tarea. Al guardar se aplica de inmediato sin volver a sincronizar. Al editar una tarea, el mismo diálogo está disponible en la pestaña "Registros DNS" y se guarda junto con la tarea. Cuando los registros tienen grupos, en la tarjeta se muestran por grupo y cada grupo se puede plegar por separado.
- **frp**: La lista de proxies se puede filtrar por instancia frpc; el modo de ordenación muestra todos los proxies y ajusta el orden completo.
- **Tokens de API**: Hasta que los ordenes manualmente, se muestran por fecha de creación, del más reciente al más antiguo. Después de ajustar el orden se muestran en el orden guardado, y los tokens nuevos se colocan al principio.
- **Usuarios de autenticación**: "Sitios que usan usuarios compartidos" se muestra por escucha, y los sitios de cada escucha se agrupan según sus grupos en Servicios web. Las escuchas y los grupos se pueden plegar por separado, y la barra de herramientas permite plegar o desplegar todo de una vez.

## API

Todos los endpoints de listas genéricas ofrecen `PUT <ruta de la lista>-order` (cuerpo `{"keys": [...]}`, que debe incluir todas las entradas) para cambiar el orden y `PUT <ruta de la lista>-group` (cuerpo `{"keys": [...], "group": "..."}`) para asignar el grupo en bloque, `PUT <ruta de la lista>-enable` (cuerpo `{"keys": [...], "enable": true}`) para activar o desactivar en lote y `POST <ruta de la lista>-delete` (cuerpo `{"keys": [...]}`) para eliminar en lote, por ejemplo `PUT /api/stun/rules-group` y `POST /api/portforward/rules-delete`. Los registros de una tarea DDNS usan `PUT /api/ddns/tasks/{key}/records-layout`, y los tokens de API usan `PUT /api/tokens/order` y `PUT /api/tokens/group`. Consulta la [Referencia de la API](/es/docs/reference/api-reference) para más detalles.

Para duplicar se usa `POST <ruta de la lista>/{key}/copy`. El cuerpo es el elemento nuevo, y los secretos que sigan siendo el marcador `********` toman los valores del elemento `{key}`, por ejemplo `POST /api/frp/clients/{key}/copy`.
