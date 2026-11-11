# Introducción

Gately es un conjunto de herramientas de pasarela de red de código abierto y auditable para redes domésticas, NAS, servidores en la nube y servidores de todo tipo. DDNS, traversal NAT con STUN, reenvío de puertos, proxy inverso, certificados, WAF y mucho más se reúnen en un único binario de unos 32 MB.

## Por qué Gately

Un software que vive en el borde de la red y guarda claves DNS y claves privadas de certificados debe ser transparente. Gately es totalmente de código abierto: los usuarios y la comunidad pueden revisar directamente el código y la configuración y ver exactamente qué hace. Los compromisos de Gately:

- El código fuente es abierto bajo Apache-2.0 y acepta la revisión de la comunidad
- Sin conexiones a servidores del proyecto ni envío de datos del dispositivo por defecto; el inicio de sesión de terceros usa tus propias aplicaciones OAuth, sin relé
- Cada dirección saliente (consulta de IP pública, servidores STUN, etc.) es visible, editable y se puede desactivar
- La configuración es JSON plano, lista para auditarse y revertirse con git

## Principios de diseño

1. **Transparencia**: sin conexiones a servidores del proyecto por defecto; cada dirección saliente es visible, editable y se puede desactivar.
2. **Migrable**: permite importar configuraciones existentes, como sitios de nginx; las reglas importadas quedan desactivadas por defecto para facilitar una transición gradual.
3. **Configuración auditable**: JSON plano (permisos 0600) que se puede llevar directamente a git para ver diferencias y revertir.
4. **Despliegue en un solo archivo**: un binario estático con el frontend integrado, compatible con Docker, systemd y OpenWrt.

## Stack tecnológico

| Capa | Elección |
| --- | --- |
| Backend | Go, compilado estáticamente, con compilación cruzada para routers y NAS arm / arm64 / mips / riscv |
| HTTP | Biblioteca estándar `net/http` |
| Almacenamiento | Un archivo JSON plano por módulo, escrito de forma atómica |
| Autenticación | Contraseñas con bcrypt + tokens de sesión firmados con HMAC, límite de intentos de inicio de sesión, autenticación de dos factores TOTP |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, empaquetado en el binario con `go:embed` |

## Resumen de funciones

DNS dinámico, traversal STUN, reenvío de puertos, enrutamiento por protocolo, certificados SSL, servicios web (proxy inverso, control de acceso, comprobaciones de estado, caché y límite de peticiones, estadísticas de tráfico, inicio de sesión único OIDC), cortafuegos WAF, prevención de intrusiones, port knocking, Tailscale, frp, Cloudflare Tunnel, gestión de Docker, terminal web, diagnóstico de red, Wake-on-LAN, WebDAV, FTP, SMB, File Browser (gestor de archivos web FileBrowser Quantum administrado por Gately), montajes de almacenamiento, tareas programadas, centro de notificaciones y copia de seguridad de la configuración, además de autenticación de dos factores, ruta de acceso segura, tokens de API, CLI y MCP. Consulta el [Resumen de módulos](/es/docs/modules/) para más detalles.

## Siguientes pasos

- Si quieres probarlo ya: [Inicio rápido](/es/docs/guide/quick-start)
- Si migras desde herramientas existentes: [Guía de migración](/es/docs/guide/migration)
- Si te importan la privacidad y la seguridad: [Modelo de seguridad y compromiso de privacidad](/es/docs/guide/security)
