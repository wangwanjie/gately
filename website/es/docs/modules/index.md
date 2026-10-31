# Resumen de módulos

Cada capacidad de Gately es un módulo independiente. Cada uno se puede activar, desactivar y configurar por separado, y guarda su configuración en un archivo JSON de texto plano.

Puedes desactivar un módulo completo en «Ajustes → Módulos». Un módulo desactivado sigue cargando su configuración, pero no inicia servicios, no ofrece API de administración (sus endpoints devuelven 403) y se oculta de la interfaz; su configuración se conserva y vuelve a aplicarse cuando lo reactivas. Los módulos de los que dependen otros no se pueden desactivar, y los cambios se aplican tras un reinicio.

| Módulo | Capacidades | Documentación |
| --- | --- | --- |
| DNS dinámico | 11 proveedores DNS, IP obtenida por URL / interfaz de red / comando, IPv4 / IPv6, webhook | [Ver](/es/docs/modules/ddns) |
| Traversal STUN | TCP / UDP, mantenimiento de la conexión, UPnP / NAT-PMP, reenvío integrado, webhooks y scripts al producirse cambios | [Ver](/es/docs/modules/stun) |
| Reenvío de puertos | TCP / UDP, IPv4 / IPv6, rangos de puertos, conmutación por error entre varios destinos, límites de conexiones, estadísticas de tráfico | [Ver](/es/docs/modules/portforward) |
| Enrutamiento por protocolo | Varios servicios comparten un puerto: TLS por SNI y ALPN, HTTP por Host, Minecraft por dirección del servidor, además de SSH, RDP, SOCKS, OpenVPN, RTSP, SIP, XMPP, MQTT, AMQP, PostgreSQL, Redis, SMB, Git, IRC, ADB, BitTorrent y una ruta de tiempo de espera para VNC y similares | [Ver](/es/docs/modules/protoroute) |
| Certificados SSL | Emisión y renovación automáticas con ACME DNS-01, subida de certificados, exportación a un directorio con scripts | [Ver](/es/docs/modules/ssl) |
| Servicios web | Proxy inverso, redirecciones, archivos estáticos, HTTPS, autenticación, control de acceso, balanceo de carga y comprobaciones de estado, caché y límite de peticiones, estadísticas de acceso | [Ver](/es/docs/modules/web) |
| Inicio de sesión único OIDC | Ofrece los usuarios compartidos a Gitea, OpenList, Grafana y otras apps mediante OpenID Connect; sustituye al OIDC de Authelia | [Ver](/es/docs/modules/oidc) |
| Cortafuegos WAF | Coraza + OWASP CRS v4 | [Ver](/es/docs/modules/waf) |
| Prevención de intrusiones | Reglas de registros (al estilo fail2ban), puertos trampa y detección de escaneos, con bloqueos sincronizados en nftables | [Ver](/es/docs/modules/guard) |
| Tailscale / frp / cloudflared | Redes y traversal NAT con binarios oficiales gestionados | [Ver](/es/docs/modules/tunnels) |
| Docker | Creación y edición de contenedores, registros y terminal, Compose, actualización de imágenes, redes y volúmenes | [Ver](/es/docs/modules/docker) |
| Terminal web | SSH, Telnet, shell local, gestor de archivos SFTP, sesiones persistentes | [Ver](/es/docs/modules/terminal) |
| Diagnóstico de red | Ping, traceroute, sondeo de puertos, consultas DNS, prueba de velocidad, escaneo de la red local | [Ver](/es/docs/modules/nettools) |
| Centro de notificaciones | Envío de eventos por varios canales y reglas | [Ver](/es/docs/modules/notify) |
| Copia de seguridad de la configuración | Copias cifradas, política de retención, copia programada y restauración | [Ver](/es/docs/modules/backup) |
| Archivos y herramientas de operación | WebDAV, FTP, SMB, File Browser, montajes de almacenamiento, Wake-on-LAN, tareas programadas | [Ver](/es/docs/modules/tools) |
