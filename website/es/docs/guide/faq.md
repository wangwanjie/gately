# Preguntas frecuentes

## ¿Necesita conectarse a los servidores del proyecto?

No. Gately no se conecta a ningún servidor del proyecto y no envía información del dispositivo; por defecto consulta de forma anónima en GitHub las versiones y los avisos de seguridad, algo que se puede desactivar, consulta [Comprobación automática de actualizaciones](/es/docs/guide/install#update-check). Las direcciones salientes, como la consulta de IP pública, usan servicios públicos de terceros y se pueden cambiar en las tareas correspondientes.

## ¿Cuáles son el usuario y la contraseña predeterminados?

El usuario predeterminado es `admin` y la contraseña predeterminada, `gately.cn`. Esta contraseña es pública: cámbiala en «Ajustes → Cuenta y seguridad» nada más iniciar sesión; hasta entonces, la interfaz de administración muestra un aviso en la parte superior. Si al desplegar definiste la variable de entorno `GATELY_INITIAL_PASSWORD`, la contraseña es la que indicaste.

## ¿Qué hago si olvido la contraseña del administrador?

Ejecuta `gately reset-password` en el servidor (con Docker, `docker exec -it gately gately reset-password`) y escribe `yes` para confirmar. La contraseña vuelve a ser la predeterminada, `gately.cn`, y una instancia en ejecución aplica el cambio en pocos segundos. Consulta [Restablecer la contraseña y la autenticación de dos factores](/es/docs/guide/install#reset).

## ¿Qué hago si pierdo el autenticador?

Ejecuta `gately reset-totp` en el servidor (con Docker, `docker exec -it gately gately reset-totp`) y confirma. La autenticación de dos factores se desactiva; inicia sesión con la contraseña y vuelve a activarla.

## ¿Es compatible con fnOS?

Sí. Cada versión incluye paquetes sin conexión para fnOS (`.fpk`) que se instalan manualmente desde el Centro de aplicaciones. Gately se integra en la pasarela unificada de fnOS, de modo que se abre desde el icono del escritorio y también funciona a través de tu dominio remoto de fnOS. Activa «Iniciar sesión con fnOS» en «Ajustes → Control de acceso» y los administradores de fnOS con la sesión iniciada entran directamente, sin contraseña ni código de verificación en dos pasos. Consulta [Aplicación nativa para fnOS](/es/docs/guide/install#fnos).

## ¿Es compatible con OpenWrt / iStoreOS?

Sí. Cada versión incluye paquetes para OpenWrt: el `.ipk` para OpenWrt 24.10 y anteriores (incluido iStoreOS), que se instala con `opkg install`, y el `.apk` para OpenWrt 25.12 y posteriores, que se instala con `apk add --allow-untrusted`. El servicio arranca justo después de la instalación y guarda su configuración en `/etc/gately`; para actualizar, instala el paquete nuevo. El binario ocupa unos 32 MB, así que conviene un router software con almacenamiento de sobra. Consulta [OpenWrt](/es/docs/guide/install#openwrt).

## ¿Por qué se recomienda `--network host`?

El reenvío de puertos, STUN y los servicios web escuchan directamente en puertos del host. Con una red bridge tienes que publicar tú mismo cada puerto en uso.

## ¿De dónde salen los programas de Tailscale, frp y cloudflared?

Estos módulos supervisan los programas oficiales externos, que Gately no incluye. La tarjeta «Instalación en línea» de cada página de módulo descarga la última versión o una versión concreta, admite un proxy de descarga propio y permite volver a la versión anterior si la actualización da problemas, además de desinstalar el programa o restaurar los valores predeterminados; consulta [Instalación, actualización y reversión en línea](/es/docs/modules/tunnels#install). La instalación en línea de Tailscale solo funciona en Linux. También puedes colocar los programas tú mismo; consulta [Configuración y parámetros de ejecución](/es/docs/guide/configuration) para saber dónde.

## Mi regla STUN está activada, pero sigo sin poder conectar desde fuera

Si la interfaz WAN de tu router está detrás de un NAT de operador (100.64.0.0/10), los equipos externos no pueden alcanzar el puerto traspasado. Es una limitación de la propia red. Usa en su lugar [Tailscale / frp / Cloudflare Tunnel](/es/docs/modules/tunnels).

## ¿Cómo informo de problemas o contribuyo?

Abre una incidencia en [GitHub Issues](https://github.com/wangwanjie/gately/issues). Las convenciones de desarrollo están en [Cómo contribuir](/es/docs/guide/contributing).
