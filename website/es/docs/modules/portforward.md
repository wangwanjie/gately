# Reenvío de puertos

Reenvía un puerto en el que escucha Gately a otro dispositivo o servicio.

## Capacidades

- TCP y UDP;
- IPv4 e IPv6, con reenvío entre v4 y v6;
- Rangos de puertos y varios puertos;
- Varios destinos con conmutación por error;
- Límites de conexiones;
- Estadísticas de tráfico y registros de ejecución;
- Envío del protocolo PROXY (v1 / v2) al destino, para entregar la dirección real del cliente al siguiente salto;
- Para que varios servicios compartan un puerto como el 443 por dominio o protocolo, usa [Enrutamiento por protocolo](/es/docs/modules/protoroute).

## Protocolo PROXY

El reenvío de puertos solo retransmite TCP, así que por defecto el servicio de destino solo ve la dirección de Gately. Si en una regla defines «Protocolo PROXY» como v1 o v2, Gately envía primero una cabecera PROXY tras conectar con el destino, y este obtiene la dirección real del cliente para sus registros, límites de peticiones y control de acceso.

El destino debe tener activada la opción de recepción correspondiente; de lo contrario, rechazará la conexión. Por ejemplo:

- haproxy: `bind 192.168.1.8:4434 accept-proxy`;
- nginx: `listen 4435 ssl proxy_protocol;`;
- Servicio web de Gately: activa «Aceptar protocolo PROXY» en el listener y añade el origen a los orígenes PROXY de confianza.

Esta opción solo afecta a TCP; el reenvío UDP no cambia.

## Escenarios típicos

- Exponer los puertos de los servicios de un NAS a través de un único punto de entrada;
- Conectar el tráfico IPv6 con IPv4;
- Configurar un backend principal y otro de reserva para un mismo servicio.

## Consejo de despliegue

El reenvío de puertos escucha directamente en puertos del host. En despliegues con Docker se recomienda `--network host`.

## Grupos y orden

Puede asignar a las reglas un «Grupo» (hasta 64 caracteres). La lista muestra los grupos como secciones plegables: los grupos aparecen en el orden en que surgen por primera vez, las reglas sin grupo van al final y el estado plegado se guarda en el navegador. El cuadro de búsqueda de la parte superior filtra por nombre, grupo, puerto de escucha o destino.

Pulse «Ordenar» para entrar en el modo de orden. Use las flechas para subir o bajar las reglas o mover grupos enteros. Marque varias y arrastre cualquiera de ellas para moverlas juntas; al soltarlas sobre un elemento o el encabezado de otro grupo pasan a ese grupo. La barra de herramientas mueve de una vez los elementos marcados a un grupo, o los saca de su grupo si deja el nombre vacío. El orden y los grupos solo afectan a la lista, y al cambiarlos el reenvío no se reinicia y las conexiones abiertas se mantienen.

Pase el ratón por el encabezado de un grupo y pulse el lápiz para renombrarlo: todos sus elementos pasan al nuevo nombre y conservan su posición y orden. Si elige un nombre ya usado, tras confirmar se fusionan los dos grupos.
