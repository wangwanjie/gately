# Certificados SSL

Emite y renueva certificados automáticamente para que los usen los [Servicios web](/es/docs/modules/web), o expórtalos a otro software.

Cuando varios certificados cubren el mismo dominio, se usa el que vence más tarde; el orden y los grupos de la lista de certificados solo afectan a cómo se muestra.

## Capacidades

- Emisión y renovación automáticas con **ACME DNS-01**, reutilizando los proveedores DNS de [DNS dinámico](/es/docs/modules/ddns), sin necesidad de abrir los puertos 80 / 443;
- **ACME HTTP-01**: no necesita proveedor DNS, pero el puerto 80 de cada dominio debe ser accesible desde Internet; no admite comodines;
- Certificados comodín y EAB (por ejemplo, ZeroSSL);
- **Certificados de corta duración y para direcciones IP**: con el perfil de corta duración de Let's Encrypt (validez de unos 6 días) se pueden emitir certificados para direcciones IP públicas;
- Subida de un certificado existente o lectura desde archivos;
- Exportación a un directorio: cuando cambia un certificado, los archivos se escriben en el directorio indicado y se ejecuta un script, para que otros servicios puedan recargarlo;
- Despliegue automático: cuando cambia un certificado, se copia a varios directorios locales, se envía por SSH a otros hosts y se ejecuta un comando, o se reinician los contenedores Docker que lo usan.

## Validación DNS-01

Elige DNS-01 en el «Método» del certificado y selecciona un proveedor de DNS. Durante la emisión, Gately añade el registro TXT `_acme-challenge` mediante la API del proveedor y lo elimina al terminar la validación.

El «Dominio raíz» es la zona DNS que contiene el registro. Si se deja vacío, Gately busca la zona autoritativa del registro de validación con una consulta SOA, de modo que se detectan correctamente un subdominio delegado a otro proveedor de DNS (por ejemplo, `web.example.com` alojado aparte de `example.com`) y los sufijos de varios niveles como `example.com.cn`. Si la consulta falla, la zona se deduce del nombre de dominio y el motivo queda registrado en el log. También puedes indicar el dominio raíz manualmente, por ejemplo `web.example.com`.

## Red de emisión

La opción «Red» del certificado limita la red de salida usada durante la emisión: «Automática» deja la elección al sistema, y «Solo IPv4» o «Solo IPv6» hace que el acceso a la CA, las llamadas a la API del proveedor DNS y la comprobación de propagación del registro TXT usen únicamente esa familia de direcciones. Si el host tiene dirección IPv6 pero IPv6 no funciona realmente, la comprobación de propagación en modo automático puede agotar el tiempo de espera al consultar los servidores DNS autoritativos; en ese caso, elige «Solo IPv4».

## Validación HTTP-01

Elige HTTP-01 en el «Método» del certificado. Durante la emisión, la CA accede a `http://<dominio>/.well-known/acme-challenge/<token>`:

- Si un [servicio web](/es/docs/modules/web) ya escucha en el puerto 80, responde él directamente sin afectar a los sitios existentes;
- Si el puerto 80 está libre, Gately escucha temporalmente en él durante la emisión y lo cierra en cuanto termina;
- Si el router reenvía el puerto público 80 a otro puerto de este equipo, indica ese puerto en «Puerto temporal».

Muchas conexiones domésticas (por ejemplo, en China continental) bloquean el puerto 80 entrante; en ese caso, usa DNS-01.

## Certificados de corta duración y para direcciones IP

«Perfil de certificado» corresponde al profile de ACME:

- **Predeterminado**: lo decide la CA; en Let's Encrypt son 90 días;
- **Corta duración (certificado IP)**: el perfil `shortlived` de Let's Encrypt, con una validez de unos 6 días. Gately lo renueva automáticamente antes de que caduque según «Horas de antelación para certificados de corta duración» en «Ajustes de renovación» (24 horas por defecto) y, si falla, lo reintenta cada hora;
- También puedes escribir otro nombre de perfil que admita la CA.

Para solicitar un certificado de dirección IP, escribe una dirección IP pública (IPv4 o IPv6, por ejemplo `203.0.113.10` o `2001:db8::1`) entre los dominios; un mismo certificado puede incluir direcciones IP y dominios. Requisitos:

- Usar Let's Encrypt y elegir el perfil de corta duración y HTTP-01 en «Método». La CA accede a `http://<IP>/.well-known/acme-challenge/<token>`, así que el puerto 80 de esa IP debe ser accesible desde Internet. Al escribir una IP, el editor muestra un aviso: basta con pulsar «Configurar automáticamente»;
- Las direcciones privadas, las de loopback y `100.64.0.0/10` no pueden superar la validación de una CA pública; una CA personalizada (como un servidor ACME propio en la red local) no tiene estas restricciones;
- ZeroSSL y Google Trust Services no emiten certificados de dirección IP por ACME.

Al acceder por HTTPS mediante una dirección IP, el navegador no envía nombre de dominio (SNI), así que Gately elige el certificado IP que corresponde a la dirección local de la conexión; tras NAT o reenvío de puertos, cuando la dirección local difiere de la IP pública del certificado, se usa el certificado IP que vence más tarde.

Al cambiar el perfil de un certificado ya emitido entre predeterminado y corta duración, se vuelve a emitir de inmediato con el nuevo perfil. En los certificados de corta duración, el aviso de caducidad solo se envía cuando el tiempo restante baja de la mitad de las horas de antelación, lo que indica que la renovación ha fallado varias veces.

## Mapeo de certificados

Cuando un certificado está activado, Gately escribe `.crt`, `.key`, `.pem` y `_issuerCertificate.crt` en el directorio de mapeo y ejecuta el script de cambio.

## Despliegue automático

En el «Despliegue automático» del certificado puedes añadir varios destinos de despliegue, que se ejecutan automáticamente cada vez que el certificado se emite, se renueva o se actualiza:

- **Copiar a directorio local**: escribe el certificado en un directorio de esta máquina y ejecuta allí el comando "Ejecutar tras copiar". Puedes añadir varios directorios, cada uno con un comando distinto. Si los archivos de un directorio se borran o cambian, la siguiente comprobación los vuelve a copiar. Si Gately se ejecuta en Docker, el directorio y el comando están dentro del contenedor;
- **Subir a host SSH**: elige una conexión SSH ya existente en el [Terminal web](/es/docs/modules/terminal) y se reutilizan su dirección, su método de autenticación y su proxy. Los archivos del certificado (`<nota>.crt`, `.key`, `.pem` y `_issuerCertificate.crt`, igual que en el mapeo de certificados, por lo que la nota es obligatoria) se escriben en el «Directorio remoto» y después se ejecuta en el host remoto el comando de «Ejecutar tras subir», por ejemplo `systemctl reload nginx`; el comando puede usar `$certName`, `$certDir`, `$certFile` y `$keyFile`. Cada archivo se escribe primero en un archivo temporal con los permisos ya ajustados y luego se renombra para sustituir al anterior; la clave privada queda con permisos 0600;
- **Reiniciar contenedores locales**: cuando cambia el certificado, los contenedores Docker elegidos se reinician uno tras otro; útil para servicios en contenedores que montan el directorio de mapeo y necesitan reiniciarse para cargar el certificado nuevo.

"Copiar a directorio local" y "Subir a host SSH" admiten nombres de archivo para el certificado (cadena completa) y la clave privada, como `fullchain.pem` y `privkey.pem`. Con ambos indicados solo se escriben esos dos archivos y la nota puede quedar vacía; si se dejan vacíos se usan los nombres del mapeo de certificados. `$certFile` y `$keyFile` apuntan a los archivos realmente escritos.

El despliegue automático nunca confía en un host desconocido: la conexión SSH elegida debe haberse abierto una vez desde el Terminal web para registrar la clave del host antes de que se envíe nada. Si un despliegue falla, se reintenta cada hora y se emite el evento «Certificado: despliegue fallido», al que puedes suscribirte en el [Centro de notificaciones](/es/docs/modules/notify). El botón «Desplegar ahora» de la tarjeta del certificado ignora el intervalo de reintento, despliega de inmediato en todos los destinos activados y muestra el resultado de cada uno.

Al editar un certificado, los despliegues cuyos ajustes de destino no han cambiado no se repiten; si cambias el directorio remoto, el comando o los contenedores, se vuelve a desplegar una vez.

## Nota de seguridad

Las claves privadas se guardan en `ssl.json` dentro del directorio de configuración. Guarda ese archivo con cuidado y, si lo pones bajo control de versiones, asegúrate de que el repositorio sea privado.
