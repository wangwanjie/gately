# Traversal STUN

Cuando tu operador no ofrece un puerto público fijo, el traversal STUN usa la perforación de NAT para obtener una dirección y un puerto públicos mapeados, de modo que los servicios de tu red local sean accesibles desde Internet.

## Capacidades

- Traversal TCP / UDP con mantenimiento de la conexión;
- UPnP / NAT-PMP crea automáticamente mapeos de puertos en el router;
- El reenvío integrado envía el puerto público obtenido mediante el traversal directamente a un destino de la red local;
- Webhooks y scripts se disparan cuando cambia la dirección o el puerto públicos, y puedes usarlos para actualizar el DNS, avisar a los clientes, etc.

## Uso junto con otros módulos

El puerto público obtenido con STUN suele cambiar. Combínalo con:

- [DNS dinámico](/es/docs/modules/ddns) o un webhook / script para sincronizar la nueva dirección con tus registros DNS; los valores de registro del DNS dinámico también pueden usar `{STUN_nombre de regla_PORT}` y variables similares, que se sincronizan en cuanto cambia la dirección de la regla, consulta [Variables del valor del registro](/es/docs/modules/ddns#record-vars);
- [Servicios web](/es/docs/modules/web) para ofrecer un punto de entrada HTTPS.
- Los sitios de redirección de los Servicios web pueden referenciar la dirección pública actual de la regla con variables como `{STUN_NombreRegla_PORT}` y seguir el puerto nuevo automáticamente cuando cambia; consulta [Variables de redirección](/es/docs/modules/web#redirect-vars).

## Limitaciones y NAT de varios niveles

- **NAT de operador (CGNAT)**: cuando el WAN del router recibe una dirección interna como `100.64.0.0/10`, la traversía depende del tipo de NAT del operador. Con cono completo (cualquier dirección externa llega al puerto mapeado) funciona; con NAT de puerto restringido o simétrico no, y en ese caso usa [Tailscale, frp o Cloudflare Tunnel](/es/docs/modules/tunnels).
- **UPnP detrás de varios NAT**: con UPnP / NAT-PMP activado, Gately mapea en el router el puerto del túnel a este equipo. Si la dirección externa del router difiere de la dirección pública que obtiene STUN, hay otro NAT por fuera y el puerto público pertenece a ese NAT exterior, así que Gately conserva solo el mapeo del puerto del túnel y deja de añadir un mapeo para el puerto público.
- **Fija el puerto local y permítelo en el cortafuegos**: si dejas vacío el puerto local, se elige al azar en cada arranque. Si el equipo usa un cortafuegos (por ejemplo el cortafuegos del sistema de fnOS), fija el puerto local de la regla y permítelo en el cortafuegos (puedes reservar un rango de puertos para las reglas STUN); si no, el cortafuegos descarta las conexiones entrantes y la dirección no es accesible.
- **TCP y el NAT de cono del router**: en algunos routers «NAT1 / cono completo» solo se aplica a UDP y en TCP se reescribe el puerto de origen al salir. Las reglas TCP necesitan UPnP o un mapeo del puerto del túnel a este equipo en el router.

## Consejo de despliegue

STUN necesita escuchar directamente en puertos del host. En despliegues con Docker se recomienda `--network host`.
