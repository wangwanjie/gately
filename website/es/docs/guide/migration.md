# Migrar de Lucky a Gately

> Gately es un proyecto independiente, sin afiliación con Lucky. La compatibilidad se basa en formatos observables públicamente.

Gately puede leer directamente los archivos de configuración `.lkcf` de Lucky o el zip del sitio completo que exporta la función «Copia de seguridad de la configuración» de Lucky, y migrar reglas, tareas y certificados. No hace falta desinstalar Lucky: ambos pueden funcionar en paralelo y puedes pasar módulo a módulo.

::: tip Migrar desde nginx
Esta guía trata la migración desde Lucky. Los sitios de nginx existentes se pueden importar directamente desde «Servicios web → Importar nginx»; consulta [Servicios web · Importar configuración de nginx](/es/docs/modules/web#import-nginx).
:::

## 1. Preparación

1. Localiza el directorio de configuración de Lucky: en un despliegue con Docker suele ser el directorio del host asignado a `/app/conf` en el contenedor, que contiene archivos como `lucky_base.lkcf` y `lucky_ddns.lkcf`.
2. Despliega Gately (consulta el README). Recomendaciones:
   - Usa `--network host`, para que escuche directamente en los puertos del host;
   - El puerto de administración es por defecto el 16680, que no entra en conflicto con el 16601 de Lucky;
   - Si los scripts o los mapeos de certificados de Lucky usan rutas internas del contenedor (por ejemplo `/etc/lucky/custom_scripts` o `/data/nginx/...`), móntalas en el contenedor de Gately en la **misma ruta**, y no tendrás que modificar nada tras importar.

## 2. Importación

Abre «Ajustes → Copia de seguridad y migración» en Gately, selecciona todos los archivos `lucky_*.lkcf` o directamente el zip de configuración del sitio completo exportado por Lucky (los `.lkcf` del archivo se detectan automáticamente y el resto de archivos se ignora). Pulsa primero «Vista previa» para ver cuántas entradas se pueden importar en cada módulo y qué avisos hay, y tras confirmar pulsa «Importar».

Importar varias veces es seguro: las reglas, tareas y certificados con el mismo nombre que uno existente se omiten.

Archivos compatibles:

| Archivo | Se importa a | Notas |
| --- | --- | --- |
| lucky_portforward.lkcf | Reenvío de puertos | Las opciones de TLS / cifrado de flujo y de límite de velocidad aún no se admiten y se muestra un aviso |
| lucky_ddns.lkcf | DNS dinámico | Admite 11 proveedores; se eliminan las direcciones oficiales de consulta de IP de Lucky |
| lucky_stun.lkcf | Traversal STUN | Incluye la lista global de servidores STUN y el webhook global |
| lucky_ssl.lkcf | Certificados SSL | Los certificados ya emitidos se importan tal cual y no se vuelven a solicitar antes de que caduquen |
| lucky_reverseproxy.lkcf | Servicios web | Sitios, autenticación y referencias a WAF (cuando el WAF de una subregla es main, se reutiliza la instancia de la regla de escucha); las variables de las subreglas de redirección, como `{host}{path}{args}`, `{port}`, `{1}` y `{STUN_NombreRegla_PORT}`, siguen funcionando |
| lucky_coraza.lkcf | Cortafuegos WAF | Conserva la clave de instancia, por lo que las referencias de los sitios siguen siendo válidas |
| lucky_wol.lkcf | Wake-on-LAN | |
| lucky_webterminal.lkcf | Terminal web | Conexiones SSH y Telnet, proxies, `keyFile`, `workDir`, tiempos de espera, variables de entorno, directorios de acceso rápido, ajustes de sesión, atajos globales, y el acuerdo de usuario y el secreto de 2FA; en las conexiones SSH, la huella del host se registra en la primera conexión |
| lucky_webdav.lkcf | WebDAV | La lista de montajes de cada usuario se convierte en varios directorios y los usuarios con el mismo nombre y contraseña se fusionan; los montajes que hacen referencia a la gestión de almacenamiento se resuelven al directorio local correspondiente |
| lucky_ftpserver.lkcf | FTP | Puerto de escucha, rango de puertos pasivos, TLS y parámetros de tiempo de espera, además de los usuarios (se tratan igual que en WebDAV) |
| lucky_storagemanagement.lkcf | Montajes de almacenamiento | Almacenamientos de directorio local y WebDAV; Alibaba Cloud Drive, los montajes del sistema y el proxy de acceso no se migran |
| lucky_frp.lkcf | frp | La primera instancia de cliente (dirección del servidor, token, protocolo de transporte y proxies) y la primera instancia de servidor; los visitantes, los proxies de plugin y el panel no se migran |
| lucky_cloudflared.lkcf | Túnel de Cloudflare | Túneles que usan token de túnel (con su token de API); los clientes de Access y los túneles que solo tienen un token de API no se migran |
| lucky_base.lkcf | Ajustes globales, Servicios web | Acceso solo desde la red local, límite de fallos de inicio de sesión, duración de la sesión, zona horaria, DNS personalizado (incluida la sintaxis doh@), parámetros de GC y orígenes de confianza para el protocolo PROXY. El puerto de administración, la ruta de acceso segura, la cuenta de administrador y la autenticación de dos factores no se migran |

Las tareas programadas, el filtrado de IP y los ajustes de Docker de Lucky están vacíos en las configuraciones habituales y por ahora no se migran; si tienen contenido, configúralos manualmente.

**Las reglas importadas quedan desactivadas por defecto**: así se evita que, tras importar en un equipo de pruebas, los dominios de producción apunten a direcciones erróneas, o que compitan por los puertos y los mapeos UPnP del router con un Lucky que sigue en marcha. Cuando confirmes que Gately se ejecuta en el dispositivo de destino y que Lucky está detenido, puedes marcar «Mantener el estado de activación de Lucky tras importar», y las reglas se ejecutarán de inmediato según el estado de activación que tenían en Lucky (incluidos los interruptores generales de cada módulo).

## 3. Cambio

Se recomienda cambiar módulo a módulo y verificar cada uno antes de pasar al siguiente:

1. **Certificados**: activa el certificado en Gately. Al activarlo, escribe el certificado en el directorio de mapeo y ejecuta el script de cambio (con los mismos nombres de archivo que Lucky); comprueba que el directorio tenga permisos de escritura.
2. **DDNS**: desactiva la tarea en Lucky → activa la tarea correspondiente en Gately → pulsa «Forzar sincronización» para confirmar que el registro se ha actualizado.
3. **Reenvío de puertos / STUN**: desactiva primero la regla en Lucky (para liberar el puerto y el mapeo UPnP) y después actívala en Gately. Al activar una regla STUN el puerto público puede cambiar, y los scripts y webhooks avisan automáticamente.
4. **Servicios web**: desactiva la regla de escucha de Lucky → actívala en Gately. Los sitios HTTPS necesitan que el módulo de certificados tenga un certificado activo que cubra el dominio.
5. Detén el contenedor de Lucky solo cuando todo esté confirmado.

## 4. Montajes en despliegues con Docker

Los mapeos de scripts y certificados de Lucky usan rutas del interior del contenedor de Lucky, así que el contenedor de Gately debe montar los mismos directorios del host en las mismas rutas del contenedor. Ve añadiéndolos uno a uno según el compose de Lucky, por ejemplo:

```yaml
volumes:
  - ./config:/etc/gately
  - /var/run/docker.sock:/var/run/docker.sock
  # Si en Lucky el certificado se mapea a /data/nginx/ssl, monta el mismo directorio del host en la misma ruta
  - /srv/nginx/ssl:/data/nginx/ssl
  # Scripts invocados en las reglas (comprueba que la ruta indicada en Lucky exista realmente dentro del contenedor de Lucky)
  - /srv/lucky/custom_scripts:/etc/lucky/custom_scripts:ro
```

La imagen de Gately incluye `sh`, `bash` y `curl`, así que los scripts de notificación habituales funcionan sin cambios.

## 5. Elementos que requieren intervención manual

- **Inicio de sesión de terceros**: el acceso con GitHub/QQ/Google de Lucky pasa por el servidor de relé oficial de Lucky; Gately usa en su lugar aplicaciones OAuth que registras tú mismo. Al importar, selecciona también `lucky_thirdPartyAuthManager.lkcf` (tabla de identidades), `lucky_reverseproxy.lkcf` y `lucky_base.lkcf`:
  - Las identidades de terceros a las que hacen referencia los sitios y el panel de administración se migran como lista de permitidos según la tabla de identidades (GitHub con ID numérico, Google con sub, QQ con openid; el nombre de usuario y el correo se guardan en las notas).
  - El método de inicio de sesión del panel de administración está en «Ajustes → Cuenta y seguridad → Inicio de sesión de terceros», y el de los sitios en «Servicios web → Ajustes globales»; solo surte efecto cuando rellenas tu propio Client ID/Secret y la dirección de callback y lo activas, ya que la importación no lo activa automáticamente. Los identificadores del método de inicio de sesión deben ser `github`, `google` y `qq` para que coincidan con las identidades migradas.
  - El openid de QQ es distinto en cada aplicación de QQ Connect: al cambiar a tu propia aplicación, el primer inicio de sesión será rechazado y se mostrará el nuevo identificador de cuenta; añádelo a la lista de permitidos.
  - Los sitios que solo dependen del inicio de sesión de terceros y para los que no se encuentra ninguna identidad migrable quedan desactivados tras importar.
- **Certificados de ZeroSSL**: se registra una nueva cuenta ACME con las credenciales EAB de Lucky; si ZeroSSL las rechaza, genera de nuevo la EAB en el panel de ZeroSSL.
- **Instancias de frp**: Gately solo tiene un frpc y un frps; las instancias sobrantes de Lucky aparecen en el informe, así que muévelas a otra máquina o fusiónalas en una sola instancia. Un frpc de Gately ya configurado para conectarse a otros servidores, o un frps ya activado, no se sobrescribe. La cuenta del panel de frps la gestiona Lucky; si necesitas el panel tras migrar, vuelve a definir el puerto y la contraseña.
- **Referencias a almacenamiento**: los usuarios de FTP / WebDAV de Lucky pueden montar almacenamientos de la «gestión de almacenamiento». Al importar, selecciona también `lucky_storagemanagement.lkcf` (o importa directamente el zip del sitio completo); los almacenamientos locales se resuelven a los directorios correspondientes. WebDAV, Alibaba Cloud Drive y los montajes de rclone aún no son compatibles y se indican en el informe.
- **Funciones de Lucky aún no implementadas**: DLNA, montaje de unidades en la nube con rclone, Alibaba Cloud Drive, la base de datos de IP (`lucky_ipdb.lkcf`) y otras aún no se migran y se omiten al importar.
- **Servidores oficiales de Lucky**: Gately no se conecta a direcciones como `66666.host` o `lucky666.cn`; tras migrar, la consulta de IP pública usa servicios públicos de terceros (se puede cambiar en las tareas DDNS).

## 6. Volver atrás

Gately no modifica ningún archivo de Lucky. Para volver atrás, desactiva las reglas correspondientes en Gately y vuelve a activarlas en Lucky.
