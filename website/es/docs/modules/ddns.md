# DNS dinámico (DDNS)

La IP pública de una conexión de banda ancha doméstica cambia con el tiempo. El DDNS actualiza automáticamente tus registros DNS tras un cambio de IP, de modo que tu dominio siempre apunte a tu dirección actual.

## Proveedores compatibles

Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod, Huawei Cloud, GoDaddy, Porkbun, dynv6, DuckDNS, Amazon Route 53, OVHcloud, Gandi, NS1, Bunny DNS, Namecheap, DynDNS2 y una devolución de llamada personalizada (para proveedores que no están integrados o para endpoints autoalojados).

| Proveedor | Credenciales | Válido para la validación DNS de certificados |
| --- | --- | --- |
| Amazon Route 53 | Access Key ID / Secret Access Key, ID de zona alojada opcional | Sí |
| OVHcloud | Application Key / Secret y Consumer Key, endpoint ovh-eu / ovh-ca / ovh-us | Sí |
| Gandi | Personal Access Token | Sí |
| NS1 | API Key | Sí |
| Bunny DNS | Account API Key | Sí |
| Namecheap | Contraseña de Dynamic DNS del dominio (solo registros A) | No |
| DynDNS2 | Usuario, contraseña y URL de actualización; para No-IP, Dynu y otros servicios compatibles con el protocolo dyndns2 | No |

## Cómo se obtiene la IP

- **URL**: solicita una dirección de consulta de IP pública (sustitúyela por el servicio que prefieras o aloja el tuyo);
- **Interfaz de red**: lee directamente la dirección de una interfaz de red concreta, adecuado para dispositivos que tienen IP pública;
- **Comando**: ejecuta un comando y usa su salida, adecuado para configuraciones de red poco habituales.

Se admiten tanto IPv4 como IPv6.

## Variables del valor del registro {#record-vars}

Los valores de registro pueden usar estas variables, que se sustituyen por los valores actuales en cada sincronización:

- `{ipv4Addr}`, `{ipv6Addr}`: las direcciones IPv4 e IPv6 que obtiene la tarea (se usan por defecto si el valor de un registro A o AAAA queda vacío);
- `{STUN_nombre de regla_PORT}`, `{STUN_nombre de regla_IP}`, `{STUN_nombre de regla_ADDR}`: puerto público, IP e IP:puerto actuales de una regla STUN; el nombre de la regla puede contener guiones bajos. Por ejemplo, un registro TXT `port={STUN_ssh_PORT}` o un registro A `{STUN_ssh_IP}`.

Cuando cambia la dirección pública de una regla STUN, las tareas que la usan se sincronizan al momento, sin esperar a la siguiente comprobación. Mientras la regla esté desactivada o aún no haya atravesado el NAT, el registro se omite en esa ronda y su estado indica el motivo. Las variables STUN también pueden usarse en el Webhook de la tarea. Si solo aparecen en el Webhook, este se llama igualmente una vez tras el cambio de dirección. Puede usarse la clave (key) de la regla en lugar de su nombre.

## Notificaciones

Se puede llamar a un webhook cuando cambia la IP o falla una sincronización. Si la dirección de retorno no es accesible directamente, en «Proxy» del webhook se puede elegir el proxy global de actualizaciones o indicar una dirección de proxy `http://`, `https://` o `socks5://`; las peticiones HTTP del traversal STUN y de las tareas programadas tienen la misma opción.

## Notas {#remark}

Los registros pueden tener una nota, que se escribe en el proveedor DNS al sincronizar y se ve en la lista de registros del panel del proveedor. Si la nota del registro está vacía se usa la de la tarea; si ambas están vacías no se modifica la nota que ya exista en el proveedor. Cambiar solo la nota también lanza una sincronización. Lo que pase de 100 caracteres no se escribe (el límite del plan gratuito de Cloudflare). En Alibaba Cloud la nota admite 50 caracteres entre chino, letras inglesas, dígitos y `_ - , . ，。`; los espacios y otros símbolos se cambian por guiones bajos.

Proveedores que reciben notas: Alibaba Cloud, Cloudflare, Tencent Cloud, DNSPod (China e internacional) y Huawei Cloud. En los demás, la nota solo se guarda en Gately.

## Eliminar registros y tareas {#delete}

Al eliminar un registro (el icono de eliminar a la derecha del registro en la lista, o quitar un registro guardado en el editor y guardar) o una tarea, un diálogo de confirmación permite marcar «Eliminar también en el proveedor DNS»:

- Si se marca, primero se eliminan los registros en el proveedor y la configuración de Gately solo se borra si todos tienen éxito; si alguno falla se muestra el motivo y la configuración de Gately no cambia, así que puedes reintentar o desmarcar la opción;
- Solo se elimina el registro que sincroniza Gately: el que tiene el mismo nombre y tipo (y línea) y el valor de la última sincronización de Gately;
- No se elimina nada, y se indica el motivo, cuando Gately todavía no ha sincronizado el registro (su valor es desconocido), cuando el valor en el proveedor difiere de la última sincronización (por ejemplo, tras cambiarlo en la consola) o cuando un proveedor que gestiona conjuntos de registros (Huawei Cloud, Route 53, Gandi, NS1, GoDaddy) aún tiene otros valores en el conjunto; sincroniza una vez antes o resuélvelo en la consola del proveedor;
- Un registro que ya no existe en el proveedor cuenta como éxito;
- Si quitas un registro al editar una tarea y marcas la opción, primero se valida la configuración que se va a guardar, y si la validación falla no se eliminan los registros en el proveedor;
- DuckDNS, Namecheap, DynDNS2 y la devolución de llamada personalizada no tienen API de eliminación, así que la opción no aparece y el registro del proveedor se borra a mano.

El estado predeterminado de la casilla se define en Ajustes globales.

## Ajustes globales {#settings}

Los Ajustes globales de la cabecera de la página contienen:

- **Activar módulo**: mientras está desactivado no se comprueba la IP ni se actualiza ningún registro DNS;
- **Eliminar también en el proveedor**: estado predeterminado de la casilla «Eliminar también en el proveedor DNS» al borrar un registro o una tarea. Está desmarcada por defecto y se puede cambiar cada vez.

## Recomendaciones

- Crea en tu proveedor DNS una clave de subcuenta limitada a la edición de registros DNS. Evita usar la clave de la cuenta principal;
- Tras crear una tarea, pulsa «Forzar sincronización» para confirmar que el registro se ha actualizado;
- Para quitar un registro que ya no necesitas, pulsa el icono de eliminar a la derecha de ese registro en la lista, sin abrir el editor; la tarea mantiene al menos un registro, y cómo eliminar también el registro en el proveedor se explica en [Eliminar registros y tareas](#delete);
- La dirección de consulta de IP pública ya no depende de ningún servicio operado por el proyecto y puedes cambiarla en una tarea en cualquier momento.

## Grupos y orden

Puede asignar a las tareas un «Grupo» (hasta 64 caracteres). La lista muestra los grupos como secciones plegables: los grupos aparecen en el orden en que surgen por primera vez, las tareas sin grupo van al final y el estado plegado se guarda en el navegador. El cuadro de búsqueda de la parte superior filtra por nombre, grupo, dominio o proveedor.

Pulse «Ordenar» para entrar en el modo de orden. Use las flechas para subir o bajar las tareas o mover grupos enteros. Marque varias y arrastre cualquiera de ellas para moverlas juntas; al soltarlas sobre un elemento o el encabezado de otro grupo pasan a ese grupo. La barra de herramientas mueve de una vez los elementos marcados a un grupo, o los saca de su grupo si deja el nombre vacío. El orden y los grupos solo afectan a la lista, y al cambiarlos las tareas no se reinician.

Pase el ratón por el encabezado de un grupo y pulse el lápiz para renombrarlo: todos sus elementos pasan al nuevo nombre y conservan su posición y orden. Si elige un nombre ya usado, tras confirmar se fusionan los dos grupos.
