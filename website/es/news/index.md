---
title: Registro de cambios
description: Anuncios y notas de cada versión de Gately.
---

# Registro de cambios

A continuación se listan los anuncios de las versiones principales. Consulta [GitHub Releases](https://github.com/wangwanjie/gately/releases) para ver todos los cambios y las descargas de cada versión.

## [v0.6.2](/es/news/v0.6.2)

31 de octubre de 2026 · El nuevo módulo Enrutamiento por protocolo permite que HTTPS, SSH, RDP, OpenVPN y otros compartan un mismo puerto; todas las listas largas se pueden agrupar y ordenar; el DNS dinámico puede usar direcciones públicas de STUN; STUN funciona detrás de varios NAT; varias mejoras de seguridad.

## [v0.6.1.1](/es/news/v0.6.1.1)

30 de octubre de 2026 · La prevención de intrusiones bloquea el sondeo de rutas web; una nueva página Usuarios de autenticación gestiona los usuarios compartidos y OIDC; los usuarios compartidos pueden cambiar su propia contraseña; los sitios pueden cerrar la conexión al denegar el acceso; las rutas de los túneles de Cloudflare con token se editan en Gately; los registros incorporan un filtro Excluir.

## [v0.6.1](/es/news/v0.6.1)

30 de octubre de 2026 · Corrección de seguridad: las cuentas y los tokens de API de solo lectura ya no pueden leer en texto plano las variables de entorno ni los comandos de inicio de los contenedores; los certificados pueden emitirse solo por IPv4 o solo por IPv6; la página de Docker ya no actualiza ni recrea el propio contenedor de Gately; las notificaciones y los registros siguen el idioma de la interfaz de administración.

## [v0.6.0.4](/es/news/v0.6.0.4)

29 de octubre de 2026 · Añade una aplicación LuCI para OpenWrt que permite ver el estado, iniciar y detener el servicio y definir el puerto de administración desde LuCI; los sitios de redirección admiten variables como `{path}`, `{1}` y `{STUN_NombreRegla_PORT}`; los usuarios compartidos y los clientes OIDC de los ajustes globales se muestran como resúmenes y se editan en un cuadro de diálogo.

## [v0.6.0.3](/es/news/v0.6.0.3)

28 de octubre de 2026 · Corrige que un Gately de la LAN que recibe HTTP en claro desde un origen PROXY de confianza no reconociera que el visitante usaba HTTPS, lo que hacía fallar el inicio de sesión OIDC con emisor https, dejaba cookies sin Secure y provocaba bucles de redirección.

## [v0.6.0.2](/es/news/v0.6.0.2)

28 de octubre de 2026 · Corrige que el terminal web se quedara en «Conectando» al abrirlo desde el escritorio de fnOS, que File Browser no arrancara con un directorio de configuración relativo y que las respuestas de la API en idiomas distintos del chino simplificado tradujeran en parte el texto chino introducido por los usuarios.

## [v0.6.0.1](/es/news/v0.6.0.1)

27 de octubre de 2026 · El DNS dinámico y el reenvío de puertos admiten grupos, orden y búsqueda; las páginas del Servicio web, DNS dinámico y reenvío de puertos permiten asignar grupo en lote, arrastrar varios elementos marcados a la vez y renombrar grupos.

## [v0.6.0](/es/news/v0.6.0)

27 de octubre de 2026 · Varias instancias de frpc y frps; el Servicio web envía a los usuarios con sesión iniciada a sus propios backends, agrupa, ordena y busca sitios, y puede reenviar las cookies de inicio de sesión de Gately; los certificados detectan la zona DNS de los subdominios delegados; las contraseñas y claves guardadas pueden mostrarse a petición.

## [v0.5.9.1](/es/news/v0.5.9.1)

26 de octubre de 2026 · Los listeners HTTPS del Servicio web pueden aceptar HTTP en el mismo puerto (redirigir o servir); las instancias de File Browser admiten URL de acceso; guardar una instancia de File Browser o un túnel de Cloudflare editados ya no falla.

## [v0.5.9](/es/news/v0.5.9)

26 de octubre de 2026 · File Browser gestionado por Gately; certificados de dirección IP y de corta duración; instalación, actualización, reversión y desinstalación en línea de frp, cloudflared, Tailscale y File Browser; los usuarios compartidos de los servicios de archivos pasan a una página propia.

## [v0.5.8.1](/es/news/v0.5.8.1)

26 de octubre de 2026 · WebDAV, FTP y SMB comparten una lista de usuarios compartidos; el Servicio web gana el tipo de sitio Cerrar conexión y registra Referer y User-Agent; frps puede escuchar en direcciones elegidas; los botones de copiar funcionan por HTTP.

## [v0.5.8](/es/news/v0.5.8)

25 de octubre de 2026 · Proveedor de identidad OIDC integrado para que los usuarios compartidos inicien sesión en Gitea, Grafana y otras aplicaciones; nuevo Servicio SMB; todos los registros se pueden filtrar; los grupos de reglas del WAF se pueden reordenar; las cabeceras de los sitios se pueden elegir entre entradas comunes.

## [v0.5.7](/es/news/v0.5.7)

24 de octubre de 2026 · Montajes de almacenamiento se conecta a recursos compartidos SMB; los temas pasan a ser 15 paletas con claro y oscuro, acento y esquinas personalizables; la interfaz de administración se transfiere comprimida y el binario es un 11 % más pequeño.

## [v0.5.6.4](/es/news/v0.5.6.4)

23 de octubre de 2026 · El despliegue automático de certificados copia a varios directorios locales, cada uno con su comando, y admite nombres de archivo personalizados para el certificado y la clave.

## [v0.5.6.3](/es/news/v0.5.6.3)

23 de octubre de 2026 · Corrige que el resumen ocultara la interfaz br-lan en OpenWrt y que la zona horaria no pudiera configurarse, con la hora siempre en UTC.

## [v0.5.6.2](/es/news/v0.5.6.2)

23 de octubre de 2026 · Nuevo diseño de las páginas de inicio de sesión y de error del servicio web, con la verificación en dos pasos en una ventana emergente. Corrige que el módulo Docker no pudiera conectarse con Docker 29, que el resumen apareciera un instante antes de la página de inicio de sesión y que las credenciales de Cloudflare y los registros DDNS renombrados importados de Lucky no se sincronizaran.

## [v0.5.6.1](/es/news/v0.5.6.1)

22 de octubre de 2026 · Corrige que las reglas del cortafuegos escritas por la imagen Docker hicieran fallar un nft antiguo en el anfitrión.

## [v0.5.6](/es/news/v0.5.6)

22 de octubre de 2026 · Añade prevención de intrusiones: reglas de registros al estilo fail2ban, puertos trampa y detección de escaneos, con bloqueos sincronizados en nftables.

## [v0.5.5.2](/es/news/v0.5.5.2)

22 de octubre de 2026 · Corrige que otros usuarios locales pudieran leer archivos sensibles del directorio de configuración en fnOS y sistemas de archivos similares.

## [v0.5.5.1](/es/news/v0.5.5.1)

21 de octubre de 2026 · Los registros de ejecución se escriben en disco y se conservan tras reinicios y actualizaciones.

## [v0.5.5](/es/news/v0.5.5)

21 de octubre de 2026 · Importación de configuraciones de nginx, despliegue automático de certificados en otros hosts y contenedores, subidas grandes y WebDAV en el WAF, listas de IP suscritas, y el autodiagnóstico comprueba ahora la cobertura de certificados y los conflictos de frp.

## [v0.5.4](/es/news/v0.5.4)

21 de octubre de 2026 · Actualización en línea para las instalaciones binarias en Linux, de modo que ya no hay que recrear los contenedores LXC de PVE; nuevos paquetes para OpenWrt (.ipk / .apk).

## [v0.5.3](/es/news/v0.5.3)

21 de octubre de 2026 · Aplicación nativa para fnOS que se integra en la pasarela unificada de fnOS e inicia la sesión de los administradores de fnOS con su sesión de fnOS; los módulos se pueden activar o desactivar.

## [v0.5.2](/es/news/v0.5.2)

20 de octubre de 2026 · El enrutamiento SNI permite que varios servicios compartan el puerto 443, Servicios web puede sustituir a nginx, los usuarios compartidos añaden TOTP e inicio de sesión único, y una comparativa de rendimiento con haproxy + nginx.

## [v0.5.0](/es/news/v0.5.0)

19 de octubre de 2026 · Registro de auditoría, historial de configuración y reversión, passkey, cuentas de solo lectura, ACME HTTP-01, HTTP/3, protocolo PROXY, métricas globales de Prometheus, autodiagnóstico de red y versiones firmadas.
