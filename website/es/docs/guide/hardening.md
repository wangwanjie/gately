# Guía de refuerzo

La interfaz de administración controla el DNS, los certificados y los puertos. Repasa la siguiente lista de comprobación.

## Interfaz de administración

- [ ] Cambia la contraseña del administrador tras el primer inicio de sesión
- [ ] Activa la autenticación de dos factores (TOTP) o registra una passkey
- [ ] Si familiares o compañeros solo necesitan consultar, crea para ellos una cuenta de solo lectura en lugar de compartir la contraseña del administrador
- [ ] Define una ruta de acceso segura para que la interfaz de administración solo sea accesible mediante una ruta personalizada
- [ ] Activa «Acceso solo desde la red local» y usa una VPN (como [Tailscale](/es/docs/modules/tunnels)) cuando necesites administrar de forma remota
- [ ] Si el puerto de la consola es accesible desde Internet, cambia «Ajustes → Control de acceso → Respuesta no autorizada» a una página 404 simple, página en blanco, cerrar la conexión, sin respuesta o una página personalizada, para que los visitantes no autorizados no sepan que es Gately; un escaneo de puertos aún encuentra el puerto, así que ocúltalo del todo con un cortafuegos o quitando la redirección
- [ ] Activa HTTPS en la interfaz de administración cuando accedas a través de redes no confiables
- [ ] Da a los scripts de automatización y a la [CLI / MCP](/es/docs/guide/cli-mcp) [tokens de API](/es/docs/reference/api) con los mínimos privilegios y fecha de caducidad, y evita compartir la contraseña del administrador
- [ ] Define una contraseña de cifrado para las [copias de seguridad de la configuración](/es/docs/modules/backup) y activa las copias programadas
- [ ] Configura avisos en el [Centro de notificaciones](/es/docs/modules/notify) con las plantillas «Seguridad e inicios de sesión», «Cambios de configuración» y «Fallos críticos»
- [ ] Si expones la interfaz de administración a través de un proxy inverso local, comprueba que el [registro de auditoría](/es/docs/guide/audit) muestra el origen real; en las entradas de reenvío de puertos, activa el [protocolo PROXY](/es/docs/modules/portforward#protocolo-proxy)
- [ ] Tras descargar una nueva versión, comprueba su firma según [Verificar versiones](/es/docs/guide/verify)
- [ ] Desactiva en «Ajustes → Módulos» los módulos que no uses para reducir los servicios a la escucha y los endpoints invocables
- [ ] En fnOS, activa «Iniciar sesión con fnOS» solo si confías en todas las cuentas de administrador de fnOS; una vez activado, una cuenta de administrador de fnOS comprometida equivale a una cuenta de administrador de Gately comprometida

## Servicios expuestos a Internet

- Al exponer servicios de la red local mediante [Servicios web](/es/docs/modules/web), activa HTTPS y combínalo con autenticación básica, autenticación web o inicio de sesión de terceros;
- Activa el [WAF](/es/docs/modules/waf) en los sitios públicos: empieza en modo «Solo detectar» para vigilar los falsos positivos y después cambia a «Bloquear»;
- Restringe los orígenes con listas de IP permitidas o bloqueadas, filtrado por país con GeoIP y bloqueo automático tras fallos de autenticación.

## Docker

Montar `docker.sock` equivale a conceder acceso de nivel root al host. Quita el montaje cuando no necesites la [Gestión de Docker](/es/docs/modules/docker).

## Terminal web

El shell local está desactivado por defecto. Actívalo solo cuando sea realmente necesario y asegúrate de que la autenticación de dos factores esté activada en la interfaz de administración.

## Copia de seguridad de la configuración

Guarda el directorio de configuración en un repositorio git privado o haz copias de seguridad con regularidad. `ssl.json` contiene claves privadas de certificados, así que guárdalo con cuidado.
