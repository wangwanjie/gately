# Diseño técnico y hoja de ruta de Gately

## Objetivo

Construir una herramienta de puerta de enlace de red de código abierto, auditable e impulsada por la comunidad: cubre DDNS, túneles STUN, reenvío de puertos, proxy inverso y gestión de certificados, las funciones que más usan las redes domésticas, los NAS y los servidores, y permite importar configuraciones existentes de Lucky.

Principios de diseño:

1. **Transparencia**: por defecto no se conecta a ningún servidor del proyecto; todas las direcciones externas son visibles, editables y se pueden desactivar.
2. **Migrable**: ofrece importación de configuraciones `.lkcf` de Lucky.
3. **Configuración auditable**: la configuración es JSON en texto plano (permisos de archivo 0600) y se puede versionar directamente en git para comparar diferencias y revertir.
4. **Despliegue en un único archivo**: un binario estático con el frontend integrado, compatible con Docker, systemd y OpenWrt.

## Pila tecnológica

| Capa | Elección | Motivo |
| --- | --- | --- |
| Backend | Go 1.26+ | Permite reutilizar directamente bibliotecas maduras como lego, pion/stun, frp, cloudflared, coraza y rclone; compilación estática y compilación cruzada para routers y NAS arm/arm64/mips/riscv |
| HTTP | Biblioteca estándar `net/http` (sintaxis de rutas de Go 1.22) | Mínimas dependencias, fácil de auditar |
| Almacenamiento | Un archivo JSON en texto plano por módulo, con escritura atómica | Facilita la importación, las copias de seguridad y el git diff |
| Autenticación | Contraseña bcrypt + token de sesión firmado con HMAC, limitación de intentos de inicio de sesión, TOTP más adelante | Sin dependencias externas |
| Registros | Búfer circular en memoria por módulo + archivos JSON Lines escritos por lotes (rotación por tamaño, restaurados tras reiniciar) + API de consulta incremental | API de sondeo de registros simple; el frontend se actualiza de forma incremental |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, empaquetado con `go:embed` | Amplio catálogo de componentes y desarrollo rápido |
| Publicación | Binarios multiarquitectura con GoReleaser + imágenes Docker multiarquitectura | — |

## Estructura de directorios

```
cmd/gately/            Punto de entrada del programa
internal/app/          Ensamblado: carga la configuración, inicia los módulos y el servicio HTTP
internal/config/       Almacenamiento modular de configuración JSON
internal/auth/         Autenticación y sesiones del administrador
internal/logbuf/       Búfer circular de registros por módulo
internal/httpx/        Utilidades y middleware HTTP
internal/module/       Interfaz y registro de módulos
internal/modules/*/    Módulos de funciones
internal/luckyimport/  Descifrado de .lkcf de Lucky y mapeo de campos
web/                   Proyecto frontend; web/embed.go integra el resultado de la compilación
```

## Hitos

Ordenados según el uso real de los usuarios (STUN 18, reenvío de puertos 25, proxy inverso 27, certificados 5, DDNS 75 registros).

| Fase | Contenido | Criterio de aceptación |
| --- | --- | --- |
| ✅ **M0 Esqueleto** | Almacenamiento de configuración, inicio de sesión del administrador, framework de módulos, búfer de registros, estructura de la interfaz de administración, Dockerfile | Tras iniciar sesión se ven el resumen y el menú de módulos |
| ✅ **M1 Reenvío de puertos** | TCP/UDP, v4/v6, varios puertos y rangos de puertos, varios destinos, límite de conexiones, inicio/parada y registros; importación de reglas de reenvío de puertos de Lucky | Pruebas unitarias del reenvío TCP/UDP; importación de 25/25 reglas de la muestra real |
| ✅ **M2 DDNS** | Obtención de IP (URL/interfaz de red/comando), IPv4/IPv6, Alibaba Cloud/Cloudflare/DNSPod (Tencent Cloud)/Huawei Cloud/callback personalizado, Webhook; importación | 11 proveedores (la firma de Alibaba Cloud se verificó con los ejemplos oficiales); importación de 2/2 tareas y 75 registros de la muestra real |
| ✅ **M3 Túnel STUN** | Obtención del mapeo público por STUN, keep-alive TCP, UPnP/NAT-PMP, reenvío integrado, Webhook y variables de script; importación | Prueba de extremo a extremo local + túnel real en una red doméstica; importación de 18/18 reglas de la muestra real |
| ✅ **M4 Certificados** | ACME (lego, DNS-01 reutiliza los Provider de M2), renovación automática, exportación a una ruta, script al cambiar | Prueba de extremo a extremo con Pebble (comodín + renovación); importación de 5/5 certificados de la muestra real sin reemisión |
| ✅ **M5 Servicio web** | Proxy inverso, redirección, archivos estáticos, enrutamiento por dominio, TLS automático, BasicAuth, filtrado de IP; importación | Cubre enrutamiento, los 4 tipos de sitio, autenticación, WebSocket, HTTPS, envío y recepción del protocolo PROXY; importación de los 27 sitios de la muestra real |
| ✅ M6 Operaciones | Wake-on-LAN, terminal web (SSH/shell local), tareas programadas, verificación en dos pasos del administrador | Importación de WOL 3/3 y de conexiones de terminal 14/14; TOTP supera los vectores de prueba de RFC 6238 |
| ✅ M7 Ecosistema (parcial) | Coraza WAF, inicio de sesión de terceros con GitHub/Google/QQ/OIDC (autenticación de sitios e inicio de sesión en la administración, con migración de la tabla de identidades de Lucky), gestión de Docker, WebDAV, servicio FTP, gestión de almacenamiento (montajes locales/WebDAV remotos y gestión de archivos en línea) | WAF probado con cargas de ataque reales; Docker verificado en modo de solo lectura en un entorno real |
| ✅ M7.5 Túneles de red | Tailscale, NetBird, frp (varias instancias de frpc y frps) y cloudflared, integrados mediante la supervisión de procesos de los binarios oficiales | Pruebas unitarias para la generación de configuración, la supervisión de procesos y la recogida de registros; los programas originales no se incluyen |
| ✅ M8 Confianza y operaciones | Registro de auditoría de operaciones, historial de configuración y reversión, directorio de configuración declarativa; inicio de sesión con passkey y cuentas de solo lectura; ACME HTTP-01; HTTP/3; métricas globales de Prometheus y panel de Grafana; autodiagnóstico de red y asistente de primer inicio; DDNS añade Route 53, OVH, Gandi, NS1, Bunny, Namecheap y DynDNS2; las notificaciones añaden ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus y plantillas de reglas; firma cosign, atestación de procedencia de compilación y SBOM de los artefactos publicados; plantillas de servicio para systemd y OpenWrt | WebAuthn probado de extremo a extremo con un autenticador por software; firma de Route 53 verificada con los vectores de prueba oficiales SigV4 de AWS; HTTP/3 probado con un cliente quic-go |
| ✅ M8.5 Instalación y actualización | Aplicación nativa para fnOS (.fpk, integrada en la pasarela unificada y en la sesión de fnOS); actualización en línea de los binarios de Linux; paquetes de instalación para OpenWrt (.ipk para 24.10 y anteriores, .apk desde 25.12) | fpk probado en una máquina virtual con fnOS; paquetes de OpenWrt probados en los contenedores rootfs oficiales de 24.10 / 25.12 y en iStoreOS, con pruebas de humo en CI que cubren instalación y desinstalación |
| ✅ M8.6 Migración y operaciones | Importación de configuración de nginx (`nginx -T`, con expansión de include, upstream y `auth_request` de Authelia); despliegue automático de certificados (envío por SSH con ejecución de comandos, reinicio de contenedores Docker); límite de inspección del cuerpo de la solicitud del WAF, inspección solo de la primera parte al superarlo y omisión por ruta; listas de IP suscritas (actualización periódica, referenciadas desde sitios y reglas por ruta); el autodiagnóstico comprueba la cobertura de certificados de los sitios HTTPS y los conflictos de proxies frp; la actualización en línea admite números de versión de cuatro partes | La importación de nginx se validó con una configuración real de producción (77 sitios, Authelia, upstream); el envío por SSH tiene pruebas de extremo a extremo con un servidor SFTP de prueba; el WAF se validó con cuerpos de solicitud de 2 MB para el rechazo, la inspección solo de la primera parte y las rutas omitidas |
| ✅ M8.7 Prevención de intrusiones | Reglas de registros al estilo fail2ban (archivos de registro, diario de systemd, salida de comandos, registros de contenedores Docker, inicios de sesión fallidos en el panel, bloqueos del WAF) con plantillas para servicios habituales; puertos trampa y detección de escaneos; una lista de bloqueos (integra los bloqueos por fallos de autenticación del servicio web, se guarda, duraciones progresivas, bloqueos manuales de redes y permanentes, lista blanca); el reenvío de puertos, el servicio web, STUN, WebDAV, FTP y el panel rechazan las direcciones bloqueadas; los bloqueos se sincronizan con nftables (una tabla propia `inet gately`) y se descartan en el kernel | Puertos trampa, detección de escaneos (IPv4/IPv6), los cuatro orígenes de registros, la persistencia y la autorreparación de las reglas verificados en espacios de nombres de red aislados en fnOS (nftables 1.0.6) e iStoreOS 24.10 (nftables 1.1.1); detección de permisos y alternativa de puertos trampa en espacio de usuario verificadas en contenedores Docker |
| M9 Por decidir | Varios administradores y roles detallados | Se planificará según la demanda de la comunidad |

## Decisiones de diseño

- La actualización en línea solo está disponible en despliegues con binario de Linux y solo la inicia un administrador de forma manual: tras la descarga se comprueba el SHA-256 contra checksums.txt y se ejecuta el nuevo programa como prueba; al reemplazarlo se conserva la versión anterior. Docker, fnOS y los paquetes de OpenWrt siguen actualizándose mediante la imagen, el centro de aplicaciones y el gestor de paquetes.
- No se envía configuración remota ni se reporta información del dispositivo.
- La configuración se guarda en texto plano; los campos sensibles se enmascaran en las respuestas de la API.
