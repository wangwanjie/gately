# Comparativa de rendimiento

Esta página compara Gately con las herramientas habituales a las que puede sustituir, en la misma máquina y con los mismos límites de CPU: proxy inverso, autenticación web, WAF, prevención de intrusiones, Port Knocking, DNS dinámico y el comportamiento en un router OpenWrt. Todas las cifras proceden de mediciones de noviembre de 2026. Los scripts de prueba, las configuraciones y los resultados originales están en el directorio [tools/benchmark](https://github.com/wangwanjie/gately/tree/main/tools/benchmark) del repositorio, de modo que puede reproducirlos.

<script setup>
const H = 'Más es mejor'
const L = 'Menos es mejor'
</script>

## Resumen

- **Proxy inverso**: nginx y HAProxy, escritos en C, tienen el mayor rendimiento: unas 3.5 veces el de Gately en HTTP y unas 2.6 veces en HTTPS. Entre las pasarelas escritas en Go, Gately está en el primer grupo: su rendimiento HTTPS es un 43 % superior al de Caddy, 2.3 veces el de Lucky y un 9 % inferior al de Traefik; su tasa de nuevas conexiones TLS es la más alta de los 7 programas.
- **Sin pérdida de velocidad con muchos sitios**: con 1000 sitios, el rendimiento de Gately baja solo un 6 %, por encima de HAProxy, Traefik y Caddy y solo por detrás de nginx.
- **Autenticación web**: en las peticiones de usuarios que ya iniciaron sesión, Gately rinde 1.9 veces más que nginx + Authelia y 3.7 veces más que Caddy + Authelia, con una tercera parte de su memoria.
- **WAF**: con el mismo OWASP CRS 4.25, Gately procesa las peticiones normales cerca de un 70 % más rápido que nginx + ModSecurity y los resultados de bloqueo son idénticos; al inspeccionar el cuerpo de las peticiones POST, ModSecurity es más rápido y usa mucha menos memoria.
- **Prevención de intrusiones**: con 10,000 IP atacantes, Gately termina de bloquearlas en 12 segundos y fail2ban tarda 51; con el mismo volumen de registros, la CPU es menos de la mitad que la de fail2ban. Al leer archivos de registro, la latencia de un bloqueo individual es unos 0.2 segundos mayor que la de fail2ban.
- **Port Knocking**: la latencia de llamada de Gately y de fwknop es inferior a 10 ms y, una vez abierto el acceso, el tráfico no pasa por el programa de Port Knocking; el knock de fnOS reenvía el tráfico a través de su pasarela en un Linux normal y su rendimiento con conexiones cortas es solo el 3 % del de una conexión directa.
- **Consumo de recursos**: para una sola función, la memoria de Gately es mayor que la de herramientas pequeñas como fail2ban, fwknop o ddns-go; cuando un único Gately se encarga a la vez de proxy inverso, WAF, autenticación, DDNS, prevención de intrusiones y Port Knocking, ocupa unos 91 MiB, frente a unos 204 MiB en total con herramientas independientes. Consulte [Todo en uno](#todo-en-uno).

## Entorno de prueba

| Elemento | Descripción |
| --- | --- |
| Host | Intel Core i5-13500T, 32 GB de memoria, fnOS (KVM) |
| VM de pruebas de carga | Debian 13 (kernel 6.12), 8 vCPU fijadas a 8 hilos de 4 núcleos de rendimiento del host, 8 GB de memoria, Docker 29 |
| Reparto de CPU | Programa probado en las vCPU 0–1, nginx de origen en las vCPU 2–3, herramientas de carga en las vCPU 4–7, sin competir entre sí |
| Router | VM ImmortalWrt 25.12.1, 2 vCPU (dos hilos del mismo núcleo de rendimiento), 1 GB de memoria |
| Herramientas de carga | wrk 4.1, oha 1.17, h2load 1.64 (nghttp2) |
| Certificado | Todos los programas comparten el mismo certificado ECDSA P-256, TLS 1.3 |
| Origen | nginx devuelve 1 KB de texto; 100 MB en la prueba de archivo grande |

Versiones probadas: Gately v0.6.8.1, nginx 1.29.8, HAProxy 3.2.26, Caddy 2.11.9, Traefik 3.7.19, Lucky 3.8.1, Nginx Proxy Manager 2.17.0 (OpenResty 1.31.8), Authelia 4.39.28, ModSecurity 3.0.16 (OWASP CRS 4.25.0), fail2ban 1.1.0, fwknop 2.6.11, el knock de fnOS fn-knock 2.5.11, ddns-go 6.17.9. Salvo fail2ban y fwknop, que usan paquetes de Debian, todo se ejecuta desde imágenes oficiales de Docker con red en modo host.

**Criterios de medición**

- Rendimiento: cada prueba dura 20 segundos, se hacen 3 rondas y se toma la mediana. HTTP y HTTPS usan wrk con 128 conexiones persistentes; las nuevas conexiones TLS usan oha con 64 en paralelo y una conexión nueva por petición; HTTP/2 usa h2load con 64 conexiones y 10 flujos simultáneos cada una.
- Memoria: suma del PSS de todos los procesos del contenedor probado (la memoria compartida entre procesos se reparte proporcionalmente, por lo que no se cuenta dos veces). «En reposo» es el valor 10 segundos después de terminar el arranque y la configuración; «pico» es el máximo durante la prueba de carga.
- CPU: tiempo de CPU del cgroup del contenedor probado, donde el 100 % equivale a una vCPU.
- Arranque: se reinicia el programa con la configuración lista, se mide el tiempo hasta que la primera petición se responde correctamente y se toma la mediana de 3 repeticiones.
- Todos los programas tienen desactivado el registro de acceso. nginx tiene activado el grupo de conexiones persistentes hacia el origen y un límite de peticiones por conexión ampliado (un ajuste habitual); el resto de programas usa su configuración predeterminada.

## Proxy inverso

El dominio `bench.test` se envía por proxy inverso al origen, con HTTP en el puerto 8080 y HTTPS en el 8443 (Nginx Proxy Manager solo puede usar 80/443).

<BenchBars title="Peticiones HTTPS por segundo (conexiones persistentes)" unit="QPS" :better-label="H" :items="[['nginx',85434],['HAProxy',83953],['Traefik',35812],['Nginx Proxy Manager',32816],['Gately',32764],['Caddy',22871],['Lucky',14376]]" />

<BenchBars title="Nuevas conexiones TLS por segundo (un handshake completo por petición)" unit="conex./s" :better-label="H" :items="[['Gately',4636],['Traefik',4482],['Nginx Proxy Manager',4015],['Caddy',3987],['nginx',3796],['Lucky',3621],['HAProxy',3293]]" />

<BenchBars title="Memoria en reposo" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['nginx',15.7],['HAProxy',36.4],['Gately',47.6],['Caddy',48.8],['Traefik',89.0],['Lucky',124.6],['Nginx Proxy Manager',155.7]]" note="Los 47.6 MiB de Gately incluyen todos los módulos funcionales (DDNS, certificados, WAF, prevención de intrusiones, etc. se ejecutan en el mismo proceso); Lucky tiene 2 procesos y Nginx Proxy Manager 22 (nginx, el backend Node.js, etc.)." />

| Programa | HTTP | HTTPS | HTTP/2 | TLS nuevas/s | Archivo de 100 MB | CPU a 5000 QPS | Memoria en reposo | Pico de carga | Por conexión inactiva | Tiempo de reinicio | Imagen |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 35,364 | 32,764 | 21,591 | **4,636** | 4,157 MB/s | 67% | 47.6 MiB | 107 MiB | 21.2 KiB | 0.40 s | 229 MB |
| nginx | **123,343** | **85,434** | 78,110 | 3,796 | 2,161 MB/s | 55% | **15.7 MiB** | **25 MiB** | **0.6 KiB** | **0.32 s** | 93 MB |
| HAProxy | 117,923 | 83,953 | **90,633** | 3,293 | 3,912 MB/s | **52%** | 36.4 MiB | 56 MiB | 1.0 KiB | 0.33 s | 58 MB |
| Caddy | 24,459 | 22,871 | 11,746 | 3,987 | 4,116 MB/s | 68% | 48.8 MiB | 132 MiB | 27.8 KiB | 0.45 s | 92 MB |
| Traefik | 39,112 | 35,812 | 19,606 | 4,482 | **4,311 MB/s** | 66% | 89.0 MiB | 253 MiB | 24.4 KiB | 0.53 s | 250 MB |
| Lucky | 15,150 | 14,376 | 10,261 | 3,621 | 3,963 MB/s | 80% | 124.6 MiB | 193 MiB | 26.2 KiB | 3.47 s | 58 MB |
| Nginx Proxy Manager | 45,257 | 32,816 | 34,710 | 4,015 | 2,150 MB/s | 58% | 155.7 MiB | 215 MiB | preasignada | 1.50 s | 1,959 MB |

- HTTP, HTTPS y HTTP/2 se expresan en peticiones por segundo. «CPU a 5000 QPS» es la CPU que usa el programa con una carga fija de 5000 peticiones por segundo; cuanto menos, más ahorro de energía.
- Por defecto, nginx cierra cada conexión tras 1000 peticiones, mientras que h2load no vuelve a conectarse, así que en la columna HTTP/2 se amplió este límite tanto en nginx como en Nginx Proxy Manager (en Nginx Proxy Manager solo se amplió mediante configuración personalizada en la prueba HTTP/2; el resto de pruebas mantuvo los valores predeterminados).
- En la columna de archivo grande, nginx y Nginx Proxy Manager quedan más bajos porque nginx escribe por defecto en un archivo temporal en disco las respuestas que superan el búfer de memoria.
- «Por conexión inactiva» es la memoria añadida al mantener 10,000 conexiones persistentes inactivas, dividida entre el número de conexiones. Los programas en Go tienen un búfer de lectura y escritura y una goroutine por conexión, unos 20–28 KiB; nginx y HAProxy, menos de 1 KiB. Nginx Proxy Manager preasigna una tabla de conexiones según su configuración, por lo que no se pudo medir ningún aumento.
- La imagen de Gately es mayor porque incluye la CLI de Docker y las herramientas nftables e iptables; el programa en sí es un único archivo de 40 MB y el paquete de descarga ocupa 18 MB.

## 1000 sitios

Cada programa se configura con 1000 sitios de proxy inverso con dominios distintos, y en la prueba de carga cada petición elige un dominio al azar.

<BenchBars title="Peticiones HTTP por segundo con 1000 sitios" unit="QPS" :better-label="H" :items="[['nginx',122958],['Gately',33211],['Traefik',24229],['HAProxy',24057],['Caddy',9819]]" />

| Programa | Peticiones/s | Frente a 1 sitio | Latencia p99 | Reinicio hasta que el último sitio está disponible | Memoria en reposo | Pico de carga |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 33,211 | -6% | 29.8 ms | **0.43 s** | 94 MiB | 463 MiB |
| nginx | **122,958** | **0%** | **1.2 ms** | 0.54 s | 57 MiB | **58 MiB** |
| HAProxy | 24,057 | -80% | 30.0 ms | **0.37 s** | **51 MiB** | **58 MiB** |
| Caddy | 9,819 | -60% | 52.7 ms | 0.80 s | 80 MiB | 361 MiB |
| Traefik | 24,229 | -38% | 18.8 ms | 0.74 s | 115 MiB | 157 MiB |

- HAProxy compara regla por regla con `use_backend ... if { hdr(host) -i dominio }`, por lo que se vuelve más lento cuantos más sitios hay; usar un archivo map lo evita, pero aquí se mantiene la forma habitual de «una regla por sitio».
- El pico de memoria de Gately procede sobre todo de las estadísticas de acceso de cada sitio: en v0.6.8.1 cada sitio con tráfico guarda un bloque entero de estadísticas por minuto de 24 horas y cuantiles de latencia (unos 108 KiB). La versión de desarrollo las guarda en bloques por hora, lo que baja el pico a 225 MiB; consulta [Optimizaciones de la versión de desarrollo](#optimizaciones-de-la-version-de-desarrollo).

## Autenticación web

El sitio exige iniciar sesión, y la prueba mide las peticiones de usuarios autenticados y el propio inicio de sesión. Gately usa la autenticación web y la autenticación básica integradas del sitio; el grupo de comparación es nginx o Caddy con Authelia delante (`auth_request` / `forward_auth`), además del `auth_basic` propio de nginx. Todo va por HTTPS.

<BenchBars title="Peticiones por segundo de usuarios autenticados (con cookie de sesión)" unit="QPS" :better-label="H" :items="[['Gately',26726],['nginx + Authelia',14291],['Lucky',10786],['Caddy + Authelia',7141]]" />

| Configuración | Peticiones autenticadas/s | Sin autenticación | Autenticación básica (bcrypt) | Inicios de sesión correctos/s | Pico de memoria al iniciar sesión | Memoria en reposo |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **26,726** | 32,520 | **28,598** | **38.3** | **70 MiB** | **45 MiB** |
| nginx + Authelia | 14,291 | 83,474 | 40 | 13.4 | 1,082 MiB | 120 MiB |
| Caddy + Authelia | 7,141 | 22,703 | 21,213 | 13.4 | 1,271 MiB | 149 MiB |
| Lucky | 10,786 | 13,841 | 10,107 | ver notas | 209 MiB | 127 MiB |

- Hash de contraseñas: la autenticación básica de Gately y de nginx/Caddy usa bcrypt (cost 10), Authelia usa argon2id (t=3, 64 MiB, p=4) y Lucky guarda en texto plano las contraseñas de los usuarios del sitio. Cada inicio de sesión en Authelia ocupa 64 MiB de memoria, y con 8 inicios de sesión simultáneos la memoria supera 1 GB.
- El `auth_basic` de nginx recalcula bcrypt en cada petición, por lo que solo alcanza 40 peticiones/s; Gately y Caddy guardan en caché el resultado de la verificación. Con apr1 (MD5), nginx alcanza 13,082 peticiones/s.
- Contraseñas incorrectas: Gately rechaza directamente a un origen tras 10 fallos (procesa 16,000 peticiones erróneas por segundo y la CPU deja de calcular hashes); Authelia añade un retraso fijo de aproximadamente 1 segundo a cada fallo. Durante el bloqueo, ambos rechazan también la contraseña correcta de ese origen, y las sesiones ya iniciadas no se ven afectadas.
- Con 8 inicios de sesión simultáneos, Lucky devolvió `create webauth session error` en todos los casos, así que no se pudo medir su rendimiento de inicio de sesión.

## WAF

Los tres usan OWASP CRS 4.25.0, nivel de paranoia 1 y modo de bloqueo. Gately y Caddy usan el mismo motor Coraza, y nginx usa ModSecurity v3.

<BenchBars title="Peticiones GET por segundo con el WAF activado" unit="QPS" :better-label="H" :items="[['Gately',2498],['Caddy + Coraza',2265],['nginx + ModSecurity',1491]]" />

| Programa | GET | GET con 8 parámetros | POST de formulario de 4 KB | Tráfico de ataque | Pico de memoria bajo carga | Memoria en reposo (act./desact.) |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **2,498** | **1,067** | 72 | **1,218** | 754 MiB | 88 / 48 MiB |
| nginx + ModSecurity | 1,491 | 591 | **102** | 1,014 | **73 MiB** | 62 / 24 MiB |
| Caddy + Coraza | 2,265 | 943 | 46 | 1,004 | 792 MiB | 96 / 55 MiB |

| Eficacia de detección | Gately | nginx + ModSecurity | Caddy + Coraza |
| --- | --- | --- | --- |
| Muestras de ataque bloqueadas (557) | 77.2% | 77.2% | 77.2% |
| Falsos positivos en muestras normales (389) | 20.1% | 20.1% | 20.1% |
| Casos de regresión de CRS detectados (2049) | 95.9% | 95.2% | 95.7% |

- Las propias reglas CRS son pesadas: con el WAF activado, el rendimiento de los tres baja a solo el 1 %–10 % del valor con el WAF desactivado, por lo que en los sitios que necesiten WAF conviene activarlo únicamente en las interfaces dinámicas.
- Al inspeccionar el cuerpo de las peticiones POST, el motor Coraza (tanto en Gately como en Caddy) supera los 700 MiB de memoria en su pico, mientras que ModSecurity se mantiene en 73 MiB; conviene tenerlo en cuenta al activar la inspección del cuerpo en dispositivos con poca memoria.
- Las muestras proceden de gotestwaf (MIT), de las pruebas de regresión incluidas en CRS (Apache-2.0) y de peticiones normales escritas a mano; los falsos positivos son sobre todo artículos técnicos con palabras clave de SQL/shell y las interfaces PUT/DELETE (CRS no permite estos métodos por defecto).

## Prevención de intrusiones

Escenario: un servicio web recibe un escaneo y se bloquea la IP tras 5 respuestas 404 en 60 segundos (nftables). fail2ban lee el registro de acceso de nginx con pyinotify; Gately lee el mismo registro con una regla de archivo de registro equivalente, y se mide además otro grupo con la detección de rutas integrada en los servicios web de Gately.

<BenchBars title="Tiempo necesario para bloquear las 10,000 IP atacantes" unit="s" better="lower" :digits="1" :better-label="L" :items="[['Gately',12.3],['fail2ban',51.2]]" />

| Métrica | Gately (lectura de archivo de registro) | Gately (detección integrada en el servicio web) | fail2ban |
| --- | --- | --- | --- |
| Latencia de bloqueo de un solo atacante p50 / p99 | 478 / 996 ms | **10 / 13 ms** | 280 / 864 ms |
| CPU con 1000 líneas/s de registro | **2.9%** | — | 20.4% |
| CPU con 20,000 líneas/s de registro | **35.7%** | — | 83.2% |
| 10,000 IP atacantes: todas bloqueadas | **12.3 s** | — | 51.2 s |
| 10,000 IP atacantes: CPU consumida | **10.1 s de CPU** | — | 60.3 s de CPU |
| Memoria en reposo | 44.1 MiB | 40.7 MiB | **17.6 MiB** |
| Sin bloquear / bloqueos erróneos | 0 / 0 | 0 / 0 | 0 / 0 |

- Gately lee el archivo de registro una vez por segundo, por lo que la latencia de un bloqueo individual está entre 0 y 1 segundo y, de media, es unos 0.2 segundos más lenta que el inotify de fail2ban; la detección integrada en el servicio web cuenta directamente al procesar la petición y no tiene ese retraso.
- fail2ban lanza un proceso `nft` en cada bloqueo, por lo que se forma cola cuando hay muchos atacantes: con 10,000 IP, la última se bloqueó 41 segundos después de terminar la inyección.
- fail2ban en sí ocupa 2 MB, pero depende de un entorno de ejecución de Python de unos 24 MB.

## Port Knocking

Se comparan tres enfoques: el Port Knocking de Gately (autorización de paquete único SPA compatible con fwknop, bloqueo con nftables), fwknop 2.6.11 (también SPA, bloqueo con iptables) y el knock de fnOS fn-knock 2.5.11 (permite la IP de origen tras iniciar sesión en la web, y su pasarela reenvía el tráfico). Gately también ofrece una entrada que permite el acceso tras iniciar sesión en la web, que se mide como un grupo aparte. Lo protegido es un servicio nginx en un puerto TCP, y el cliente usa una dirección de prueba pública, por lo que no le afecta la excepción «la red local no necesita llamar».

<BenchBars title="Peticiones por segundo con conexiones cortas por el puerto protegido tras la apertura" unit="QPS" :better-label="H" :items="[['fwknop',65046],['Gately',64763],['Gately acceso web',63556],['fn-knock',1886]]" note="Conectando directamente al origen sin ningún programa de Port Knocking se obtienen 67,034 peticiones/s." />

| Métrica | Gately (SPA) | Gately (acceso web) | fwknop | fn-knock |
| --- | --- | --- | --- | --- |
| De la llamada a puerto accesible p50 / p99 | 8.4 / 13.4 ms | 58 / 73 ms | 9.0 / 18.1 ms | 84 / 143 ms |
| Puerto protegido sin autorización | Descarte (tiempo agotado) | Descarte (tiempo agotado) | Descarte (tiempo agotado) | Reinicio tras el handshake (detectable) |
| Peticiones/s con conexión persistente / corta tras la apertura | 143,867 / 64,764 | 143,839 / 63,556 | 148,371 / 65,046 | 140,137 / 1,886 |
| CPU del reenvío tras la apertura (conexiones cortas) | 2% | 1% | 0% | 134% |
| Llamadas legítimas con 50,000 paquetes inválidos/s desde un único origen | **10/10** (CPU 26%) | — | 3/8 (CPU 102%) | — |
| Llamadas legítimas con 10,000 paquetes inválidos/s desde orígenes falsificados | 1/7 | — | **10/10** (CPU 54%) | — |
| Memoria en reposo | 48.7 MiB | 48.7 MiB | **2.2 MiB** | 63.2 MiB (3 procesos) |

- En Gately y fwknop, tras la apertura el cortafuegos deja pasar el tráfico directamente y este no pasa por el programa de Port Knocking, por lo que el rendimiento es igual al de una conexión directa; fn-knock reenvía a través de su pasarela en un Linux normal y además consulta una vez al plano de control la autorización en cada conexión nueva, de modo que su rendimiento con conexiones cortas es solo el 3 % del de una conexión directa. En la versión fpk de fnOS, fn-knock tiene además un modo de conexión directa con el cortafuegos; aquí se midió la versión Docker.
- El inicio de sesión de fn-knock exige resolver primero una prueba de trabajo ALTCHA (en esta prueba, p50 de 28 ms para resolverla); sin contar la resolución, 50 ms.
- fwknopd procesa cada paquete en un único hilo; con 50,000 paquetes/s desde un único origen satura un núcleo y las llamadas legítimas empiezan a fallar. Gately limita la tasa por origen, por lo que una inundación desde un único origen casi no consume CPU.
- Problema conocido: cuando un atacante falsifica muchas direcciones de origen distintas, la tabla de límite de tasa por origen de Gately se llena, la cuota de desbordamiento compartida por los orígenes nuevos se agota con la inundación y la mayoría de las llamadas legítimas fallan (en ese momento la CPU de Gately es de solo un 19 % aproximadamente). fwknop no limita la tasa y no se ve afectado hasta que la CPU se satura.

## DNS dinámico

Cada programa se configura con 20 tareas DDNS y un período de comprobación de 30 segundos. La IP pública se consulta en el origen local y las actualizaciones se envían al origen local mediante una «devolución de llamada personalizada», sin tocar ningún proveedor de DNS real. Tras el arranque y la primera sincronización, se toma una muestra durante 5 minutos.

<BenchBars title="Memoria media con 20 tareas DDNS" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['ddns-go',16.9],['Gately',51.6],['Lucky',131.3]]" />

| Programa | Memoria media | Memoria pico | CPU media | Imagen |
| --- | --- | --- | --- | --- |
| ddns-go 6.17.9 | **16.9 MiB** | **18.2 MiB** | **0.04%** | 43 MB |
| Gately | 51.6 MiB | 55.4 MiB | 0.16% | 229 MB |
| Lucky 3.8.1 | 131.3 MiB | 134.9 MiB | 0.48% | 58 MB |

- Los tres solo consultan al proveedor cuando cambia la IP o llega el período de sincronización forzada, por lo que el consumo permanente es pequeño en todos. Si solo se necesita DDNS, ddns-go es el que menos memoria usa; la cifra de Gately incluye todos sus módulos y el mismo proceso puede ejecutar a la vez certificados, proxy inverso y otras funciones.

## Router OpenWrt

Cada programa se ejecuta en una VM de router ImmortalWrt 25.12.1 (2 vCPU, 1 GB de memoria). La máquina de carga accede al router mediante un puente de la red local y el router envía por proxy inverso al origen de la máquina de carga. nginx 1.26.3 y HAProxy 3.2.21 proceden del repositorio de paquetes de ImmortalWrt, Gately usa el paquete apk oficial de OpenWrt y Lucky usa el binario oficial de Linux.

<BenchBars title="Peticiones HTTPS por segundo en el router (conexiones persistentes)" unit="QPS" :better-label="H" :items="[['nginx',40665],['HAProxy',36266],['Gately',15669],['Lucky',8113]]" />

| Programa | HTTP | HTTPS | TLS nuevas/s | CPU a 2000 QPS | Memoria en reposo | Pico de carga | Tamaño del programa |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 16,439 | 15,669 | 2,665 | 27% | 43 MiB | 65 MiB | 40.5 MB |
| nginx | **49,566** | **40,665** | **3,111** | **12%** | **15 MiB** | **22 MiB** | **0.8 MB** |
| HAProxy | 43,046 | 36,266 | 2,337 | 12% | 24 MiB | 29 MiB | 3.7 MB |
| Lucky | 8,642 | 8,113 | 2,021 | 44% | 64 MiB | 81 MiB | 17.9 MB |

- En el router, el cuello de botella está sobre todo en el envío y la recepción de red del kernel (interrupciones software), y ningún programa llega a usar por completo los dos núcleos, por lo que las diferencias son menores que en la máquina de carga. El rendimiento de Gately es aproximadamente el doble que el de Lucky, con una tercera parte menos de memoria.
- El kernel de OpenWrt no tiene smaps, así que aquí la memoria se estima a partir de `/proc/<pid>/statm` (suma de la memoria residente privada de cada proceso más una copia de la parte compartida). Durante la prueba se desactivó temporalmente la protección contra inundaciones SYN que trae el cortafuegos por defecto (unas 25 conexiones nuevas por segundo), porque de lo contrario limitaría la prueba de nuevas conexiones TLS.
- Si solo se necesita proxy inverso, el nginx del repositorio de paquetes es el más pequeño en tamaño y memoria; los 40 MB de Gately incluyen todos los módulos, y en [build.gately.cn](https://build.gately.cn) puede elegir solo los módulos necesarios para generar un paquete de instalación más pequeño. En routers con poca memoria flash, conviene comprobar antes el espacio libre.

## Todo en uno

Un único Gately con todo esto activado a la vez: dos sitios de proxy inverso (HTTP y HTTPS, ambos con WAF), un sitio con autenticación web, 20 tareas DDNS, una regla de registro de prevención de intrusiones y una regla de Port Knocking. El grupo de comparación es una combinación de herramientas independientes que ofrecen las mismas funciones, sumando la memoria en reposo medida antes para cada una.

<BenchBars title="Memoria en reposo total para el mismo conjunto de funciones" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['Gately',90.8],['nginx + ModSecurity + Authelia + ddns-go + fail2ban + fwknop',203.5],['Nginx Proxy Manager + Authelia + ddns-go + fail2ban + fwknop',297.3]]" note="El grupo de Nginx Proxy Manager no incluye WAF." />

| Función | Herramienta independiente | Memoria en reposo | Tamaño de instalación |
| --- | --- | --- | --- |
| Proxy inverso + WAF | nginx + ModSecurity (CRS 4.25) | 61.9 MiB | Imagen de 156 MB |
| Proxy inverso (interfaz gráfica) | Nginx Proxy Manager | 155.7 MiB | Imagen de 1,959 MB |
| Autenticación web | Authelia | 104.4 MiB | Imagen de 110 MB |
| DNS dinámico | ddns-go | 17.4 MiB | Imagen de 43 MB |
| Prevención de intrusiones | fail2ban | 17.6 MiB | 24 MB (con Python) |
| Port Knocking | fwknop | 2.2 MiB | 0.3 MB |
| **Todo** | **Gately** | **90.8 MiB** | **Programa de 40 MB (descarga de 18 MB)** |

- La instancia todo en uno de Gately, con 1000 peticiones/s de tráfico HTTPS a través del WAF, alcanza un pico de memoria de 114 MiB y una CPU del 71 %; las 20 tareas DDNS se sincronizaron correctamente.
- La emisión de certificados (certbot, acme.sh) solo se ejecuta en las renovaciones y no se incluye en la memoria residente.

## Optimizaciones de la versión de desarrollo

A partir de los resultados anteriores, la versión de desarrollo (sin publicar, posterior al commit `3d579389`) introdujo los cambios siguientes y se volvió a medir en el mismo entorno. El proxy inverso usa la compilación del commit `3d579389`; los 1000 sitios y la prevención de intrusiones usan una compilación que incluye los cambios posteriores. La columna de referencia es v0.6.8.1 medida de nuevo el mismo día (ese día solo se repitieron las filas de HTTP y HTTPS; las demás referencias proceden de los datos anteriores).

- Los sitios de un mismo oyente que hacen proxy inverso al mismo backend con la misma configuración de conexión comparten un grupo de conexiones.
- Con al menos 1 GiB de memoria disponible, el valor por defecto del porcentaje de GC pasa de 100 a 200 (se puede cambiar en los ajustes; los dispositivos con poca memoria mantienen 100).
- El proxy inverso copia la petición una vez menos y crea tres valores de contexto menos por petición.
- Las estadísticas por minuto de 24 horas de cada sitio se guardan en bloques por hora que se reservan al usarse, y los campos de los cubos pasan a contadores de 32 bits (un sitio con tráfico durante una sola hora baja de unos 108 KiB a unos 5 KiB, y uno con tráfico todo el día ocupa unos 50 KiB).
- La prevención de intrusiones lee los archivos de registro con inotify, de modo que lee en cuanto se escribe (se mantiene el sondeo cada segundo como respaldo); las escrituras de bloqueos en el firewall se combinan como máximo una vez cada 250 milisegundos.

| Métrica | v0.6.8.1 | Versión de desarrollo | Cambio |
| --- | --- | --- | --- |
| Peticiones HTTP/s (referencia del mismo día) | 35,968 | 38,358 | +6.6% |
| Peticiones HTTPS/s (referencia del mismo día) | 34,072 | 36,079 | +5.9% |
| Conexiones TLS nuevas/s | 4,636 | 4,942 | +6.6% |
| Peticiones HTTP/2/s | 21,591 | 23,788 | +10% |
| 1000 sitios: peticiones/s | 33,211 | 37,423 | +13% |
| 1000 sitios: latencia p99 | 29.8 ms | 12.9 ms | -57% |
| 1000 sitios: pico de memoria durante la prueba | 463 MiB | 225 MiB | -51% |
| Pico de memoria en la prueba HTTP | 64.5 MiB | 81.1 MiB | +26% |
| Prevención de intrusiones: latencia de bloqueo de un solo atacante p50 / p99 | 478 / 996 ms | 9.8 / 129 ms | |
| Prevención de intrusiones: latencia de bloqueo con ataque sostenido p50 / p99 (20,000 líneas/s) | 509 / 1006 ms | 143 / 269 ms | |
| Prevención de intrusiones: CPU a 1000 líneas/s | 2.9% | 8.0% | mayor |
| Prevención de intrusiones: CPU a 20,000 líneas/s | 35.7% | 26.5% | -26% |
| Prevención de intrusiones: bloquear las 10,000 IP atacantes | 12.3 s, 10.1 s de CPU | 10.1 s, 2.5 s de CPU | |

- La mejora del rendimiento procede sobre todo del ajuste de GC, a cambio de un pico de memoria unos diez MiB mayor durante la prueba. El análisis de perfil muestra que casi toda la sobrecarga restante en la ruta del proxy está en la implementación HTTP de la biblioteca estándar de Go y en el GC; la diferencia con nginx y HAProxy corresponde al lenguaje y al entorno de ejecución.
- Con 1000 sitios, el rendimiento sube y la latencia baja porque, al compartir el grupo de conexiones, hay muchas menos conexiones hacia el origen; el pico de memoria se reduce a la mitad gracias al almacenamiento de las estadísticas en bloques.
- La latencia de un bloqueo individual ya es menor que la de fail2ban (280 / 864 ms). Con pocas líneas de registro por segundo, la CPU es mayor que en la versión anterior porque el firewall se escribe como máximo 4 veces por segundo, frente a 1 vez antes; sigue siendo cuatro décimas partes de la de fail2ban (20.4%).

## Reproducir

```bash
git clone https://github.com/wangwanjie/gately && cd gately/tools/benchmark
# Ejecutar como root en una VM Debian 13; requiere Docker, wrk, oha y h2load
bash provision-debian.sh
python3 lib/run.py gately 3     # proxy inverso; sustituya por nginx, haproxy, caddy, traefik, lucky, npm
bash auth/run.sh                # autenticación web
bash waf/run.sh                 # WAF
bash guard/run.sh               # prevención de intrusiones
bash knock/run.sh               # Port Knocking
bash scale/run.sh 1000          # 1000 sitios
bash ddns/run.sh 20             # DNS dinámico
bash bundle/run.sh              # todo en uno
bash router/run.sh              # router (requiere además una máquina OpenWrt)
```

El README de cada directorio describe la configuración, los criterios de medición y las limitaciones conocidas. Los resultados dependen del hardware, de la virtualización y de la configuración, por lo que conviene repetir las pruebas con su propia carga de trabajo.
