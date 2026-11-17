# Inicio rápido

Despliega, inicia sesión y crea tu primera regla en cinco minutos.

## 1. Arrancar

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  ghcr.io/wangwanjie/gately:latest
```

Para gestionar Docker desde Gately, añade `-v /var/run/docker.sock:/var/run/docker.sock` al comando. `--cap-add NET_ADMIN` permite que Prevención de intrusiones escriba los bloqueos en el cortafuegos del anfitrión. El reenvío de puertos, STUN y los servicios web escuchan en puertos del host, por lo que se recomienda `--network host`.

Si GHCR te resulta lento, usa en su lugar `vanjayak/gately:latest` de Docker Hub; es la misma imagen.

::: tip Usuarios de fnOS
Te recomendamos instalar la aplicación nativa: se abre desde el icono del escritorio a través de la pasarela unificada de fnOS y puede iniciar la sesión de los administradores de fnOS con su sesión de fnOS. Consulta [Aplicación nativa para fnOS](/es/docs/guide/install#fnos).
:::

## 2. Iniciar sesión

Abre `http://<ip-del-dispositivo>:16680` en un navegador. El usuario es `admin` y la contraseña predeterminada es `gately.cn`. Esta contraseña es pública: al iniciar sesión aparece un diálogo de cambio de contraseña, y para continuar hay que elegir una nueva de al menos 10 caracteres y difícil de adivinar.

La barra lateral de la interfaz de administración lista todas las páginas por grupos. Pulsa Ctrl+K (⌘K en macOS) o el cuadro de búsqueda de la parte superior de la barra lateral para abrir la búsqueda global: lleva por nombre a una página o a una pestaña dentro de ella, y también busca dominios de sitios, reglas de reenvío de puertos, registros DDNS y dominios de certificados. En el móvil, pulsa el icono de búsqueda de la barra superior.

## 3. Aplicar un refuerzo básico

Nada más iniciar sesión, ve a «Ajustes» y:

1. Activa la autenticación de dos factores (TOTP) o registra una passkey;
2. Configura una ruta de acceso segura, el acceso solo desde la red local o HTTPS para la interfaz de administración según lo necesites;
3. Revisa la puntuación de seguridad en la parte superior de «Ajustes → Cuenta y seguridad» y resuelve los puntos que restan.

Consulta la [Guía de refuerzo](/es/docs/guide/hardening) para más recomendaciones.

## 4. Añadir tu primera regla

Elige el módulo que se ajuste a tu necesidad:

| Necesidad | Módulo |
| --- | --- |
| La IP pública de tu casa cambia constantemente y quieres un nombre de dominio estable | [DNS dinámico](/es/docs/modules/ddns) |
| Tu operador no te da un puerto público fijo y quieres acceder desde fuera a los servicios de casa | [Traversal STUN](/es/docs/modules/stun) |
| Reenviar un puerto a otro dispositivo de tu red local | [Reenvío de puertos](/es/docs/modules/portforward) |
| Acceder a varios servicios de la red local con un solo dominio y HTTPS | [Certificados SSL](/es/docs/modules/ssl) + [Servicios web](/es/docs/modules/web) |
| Conectar redes en ubicaciones distintas | [Tailscale / frp](/es/docs/modules/tunnels) |
