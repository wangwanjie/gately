# API de administración de Gately

La API de administración de Gately permite la gestión por lotes mediante scripts y automatización.

Autenticación: cree un token en la página «Tokens de API» del panel de administración e incluya en la cabecera de la solicitud `Authorization: Bearer gtl_xxxx`.

Permisos: cada token solo tiene los permisos marcados al crearlo, con el formato `<módulo>:read` o `<módulo>:write`; el permiso de escritura incluye el de lectura. Los endpoints de cuenta, verificación en dos pasos, administración de tokens, y los de copia de seguridad, restauración, reinicio y ajustes del sistema solo pueden usarse con una sesión iniciada.

El cuerpo de la solicitud usa `Content-Type: application/json`; los errores se devuelven siempre como `{"error": "descripción"}`.

Los módulos desactivados en «Ajustes → Módulos» no ofrecen API: sus endpoints devuelven 403 y dejan de aparecer en la documentación de la API de la instancia en ejecución. Los módulos no incluidos en una compilación personalizada (build.gately.cn) devuelven 404 en sus endpoints, indicando que el módulo no está incluido en esta compilación, y tampoco aparecen en la documentación de la API de la instancia en ejecución.

Sitio web: https://gately.cn

La descripción legible por máquinas está en `openapi.json` (OpenAPI 3.0), en el mismo directorio.

## Inicio rápido

```bash
# Ver el estado del sistema (requiere status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# Listar todas las tareas DDNS (requiere ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Sincronizar ahora una tarea DDNS (requiere ddns:write; key se toma de la lista devuelta en el paso anterior)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## Permisos

| Dominio de permisos | Nombre | Permisos disponibles |
| --- | --- | --- |
| `portforward` | Reenvío de puertos | `portforward:read`、`portforward:write` |
| `protoroute` | Enrutamiento por protocolo | `protoroute:read`、`protoroute:write` |
| `ddns` | DNS dinámico | `ddns:read`、`ddns:write` |
| `stun` | Traversal STUN | `stun:read`、`stun:write` |
| `tailscale` | Red Tailscale | `tailscale:read`、`tailscale:write` |
| `netbird` | Red NetBird | `netbird:read`、`netbird:write` |
| `easytier` | Red EasyTier | `easytier:read`、`easytier:write` |
| `frp` | Traversal de red interna frp | `frp:read`、`frp:write` |
| `cloudflared` | Túnel de Cloudflare | `cloudflared:read`、`cloudflared:write` |
| `ssl` | Certificado SSL | `ssl:read`、`ssl:write` |
| `waf` | Firewall WAF | `waf:read`、`waf:write` |
| `webservice` | Servicio web | `webservice:read`、`webservice:write` |
| `guard` | Prevención de intrusiones | `guard:read`、`guard:write` |
| `knock` | Port knocking | `knock:read`、`knock:write` |
| `wol` | Wake-on-LAN | `wol:read`、`wol:write` |
| `webterminal` | Terminal web | `webterminal:read`、`webterminal:write` |
| `cron` | Tarea programada | `cron:read`、`cron:write` |
| `docker` | Administración de Docker | `docker:read`、`docker:write` |
| `webdav` | Servicio WebDAV | `webdav:read`、`webdav:write` |
| `ftp` | Servicio FTP | `ftp:read`、`ftp:write` |
| `smb` | Servicio SMB | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | Montajes de almacenamiento | `storage:read`、`storage:write` |
| `notify` | Centro de notificaciones | `notify:read`、`notify:write` |
| `perf` | Rendimiento | `perf:read`、`perf:write` |
| `nettools` | Diagnóstico de red | `nettools:read`、`nettools:write` |
| `backup` | Copia de seguridad de la configuración | `backup:read`、`backup:write` |
| `status` | Estado del sistema | `status:read` |
| `logs` | Registros del sistema | `logs:read` |
| `metrics` | Métricas (Prometheus) | `metrics:read` |
| `import` | Importación de configuración de Lucky | `import:write` |
| `fileusers` | Usuarios compartidos de los servicios de archivos | `fileusers:read`、`fileusers:write` |

Comodines: `*:read` equivale a solo lectura en todos los módulos; `*:write` o `*` equivale a lectura y escritura en todos los módulos.

## Reenvío de puertos

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | Ver los registros de ejecución del módulo |
| GET | `/api/portforward/logs/sources` | `portforward:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/portforward/logs/export` | `portforward:read` | Exportar registros del módulo |
| DELETE | `/api/portforward/logs` | `portforward:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/portforward/rules` | `portforward:read` | Listar Regla de reenvío de puertos |
| POST | `/api/portforward/rules` | `portforward:write` | Añadir Regla de reenvío de puertos |
| POST | `/api/portforward/rules/{key}/copy` | `portforward:write` | Duplicar Regla de reenvío de puertos |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | Modificar Regla de reenvío de puertos |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | Eliminar Regla de reenvío de puertos |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | Activar o desactivar Regla de reenvío de puertos |
| PUT | `/api/portforward/rules-order` | `portforward:write` | Reordenar Regla de reenvío de puertos |
| PUT | `/api/portforward/rules-enable` | `portforward:write` | Activar o desactivar en lote: Regla de reenvío de puertos |
| POST | `/api/portforward/rules-delete` | `portforward:write` | Eliminar en lote: Regla de reenvío de puertos |
| PUT | `/api/portforward/rules-group` | `portforward:write` | Asignar grupo a varios elementos: Regla de reenvío de puertos |
| GET | `/api/portforward/settings` | `portforward:read` | Leer los ajustes globales de reenvío de puertos |
| PUT | `/api/portforward/settings` | `portforward:write` | Guardar los ajustes globales de reenvío de puertos |

## Enrutamiento por protocolo

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | Ver los registros de ejecución del módulo |
| GET | `/api/protoroute/logs/sources` | `protoroute:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/protoroute/logs/export` | `protoroute:read` | Exportar registros del módulo |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/protoroute/rules` | `protoroute:read` | Listar regla de enrutamiento por protocolo |
| POST | `/api/protoroute/rules` | `protoroute:write` | Añadir regla de enrutamiento por protocolo |
| POST | `/api/protoroute/rules/{key}/copy` | `protoroute:write` | Duplicar regla de enrutamiento por protocolo |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | Modificar regla de enrutamiento por protocolo |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | Eliminar regla de enrutamiento por protocolo |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | Activar o desactivar regla de enrutamiento por protocolo |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | Reordenar regla de enrutamiento por protocolo |
| PUT | `/api/protoroute/rules-enable` | `protoroute:write` | Activar o desactivar en lote: regla de enrutamiento por protocolo |
| POST | `/api/protoroute/rules-delete` | `protoroute:write` | Eliminar en lote: regla de enrutamiento por protocolo |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | Asignar grupo a varios elementos: regla de enrutamiento por protocolo |
| GET | `/api/protoroute/settings` | `protoroute:read` | Leer los ajustes del enrutamiento por protocolo |
| PUT | `/api/protoroute/settings` | `protoroute:write` | Guardar los ajustes del enrutamiento por protocolo |

## DNS dinámico

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | Ver los registros de ejecución del módulo |
| GET | `/api/ddns/logs/sources` | `ddns:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/ddns/logs/export` | `ddns:read` | Exportar registros del módulo |
| DELETE | `/api/ddns/logs` | `ddns:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/ddns/tasks` | `ddns:read` | Listar Tarea DDNS |
| POST | `/api/ddns/tasks` | `ddns:write` | Añadir Tarea DDNS |
| POST | `/api/ddns/tasks/{key}/copy` | `ddns:write` | Duplicar Tarea DDNS |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | Modificar Tarea DDNS |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | Eliminar Tarea DDNS |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | Activar o desactivar Tarea DDNS |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | Reordenar Tarea DDNS |
| PUT | `/api/ddns/tasks-enable` | `ddns:write` | Activar o desactivar en lote: Tarea DDNS |
| POST | `/api/ddns/tasks-delete` | `ddns:write` | Eliminar en lote: Tarea DDNS |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | Asignar grupo a varios elementos: Tarea DDNS |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | Ver secretos en texto plano |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | Sincronizar ahora una tarea DDNS |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | Eliminar un registro de una tarea DDNS |
| POST | `/api/ddns/tasks/{key}/remote-delete` | `ddns:write` | Eliminar en el proveedor DNS los registros de una tarea DDNS |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | Reordenar y agrupar los registros DNS de una tarea |
| GET | `/api/ddns/providers` | `ddns:read` | Listar los proveedores DNS compatibles y los parámetros que requieren |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | Listar las tarjetas de red de este equipo y sus direcciones |
| POST | `/api/ddns/iptest` | `ddns:write` | Probar la obtención de la IP pública |
| GET | `/api/ddns/defaults` | `ddns:read` | Leer los valores predeterminados de DDNS |
| GET | `/api/ddns/settings` | `ddns:read` | Leer los ajustes globales de DDNS |
| PUT | `/api/ddns/settings` | `ddns:write` | Guardar los ajustes globales de DDNS |

## Traversal STUN

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | Ver los registros de ejecución del módulo |
| GET | `/api/stun/logs/sources` | `stun:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/stun/logs/export` | `stun:read` | Exportar registros del módulo |
| DELETE | `/api/stun/logs` | `stun:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/stun/rules` | `stun:read` | Listar Regla STUN |
| POST | `/api/stun/rules` | `stun:write` | Añadir Regla STUN |
| POST | `/api/stun/rules/{key}/copy` | `stun:write` | Duplicar Regla STUN |
| PUT | `/api/stun/rules/{key}` | `stun:write` | Modificar Regla STUN |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | Eliminar Regla STUN |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | Activar o desactivar Regla STUN |
| PUT | `/api/stun/rules-order` | `stun:write` | Reordenar Regla STUN |
| PUT | `/api/stun/rules-enable` | `stun:write` | Activar o desactivar en lote: Regla STUN |
| POST | `/api/stun/rules-delete` | `stun:write` | Eliminar en lote: Regla STUN |
| PUT | `/api/stun/rules-group` | `stun:write` | Asignar grupo a varios elementos: Regla STUN |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | Reiniciar una regla STUN |
| GET | `/api/stun/netinterfaces` | `stun:read` | Listar las tarjetas de red de este equipo y sus direcciones |
| GET | `/api/stun/settings` | `stun:read` | Leer los ajustes globales de STUN |
| PUT | `/api/stun/settings` | `stun:write` | Guardar los ajustes globales de STUN |

## Red Tailscale

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | Ver los registros de ejecución del módulo |
| GET | `/api/tailscale/logs/sources` | `tailscale:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/tailscale/logs/export` | `tailscale:read` | Exportar registros del módulo |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/tailscale/instances` | `tailscale:read` | Listar instancia de Tailscale |
| POST | `/api/tailscale/instances` | `tailscale:write` | Añadir una instancia de Tailscale |
| POST | `/api/tailscale/instances/{key}/copy` | `tailscale:write` | Duplicar instancia de Tailscale |
| PUT | `/api/tailscale/instances/{key}` | `tailscale:write` | Modificar instancia de Tailscale |
| DELETE | `/api/tailscale/instances/{key}` | `tailscale:write` | Eliminar instancia de Tailscale |
| PUT | `/api/tailscale/instances/{key}/enable` | `tailscale:write` | Activar o desactivar instancia de Tailscale |
| PUT | `/api/tailscale/instances-order` | `tailscale:write` | Reordenar instancia de Tailscale |
| PUT | `/api/tailscale/instances-enable` | `tailscale:write` | Activar o desactivar en lote: instancia de Tailscale |
| POST | `/api/tailscale/instances-delete` | `tailscale:write` | Eliminar en lote: instancia de Tailscale |
| PUT | `/api/tailscale/instances-group` | `tailscale:write` | Asignar grupo a varios elementos: instancia de Tailscale |
| GET | `/api/tailscale/instances/{key}/secrets` | `tailscale:write` | Ver secretos en texto plano |
| GET | `/api/tailscale/status` | `tailscale:read` | Ver el estado de conexión de todas las instancias de Tailscale |
| GET | `/api/tailscale/instances/{key}/status` | `tailscale:read` | Ver el estado de conexión de una instancia de Tailscale |
| POST | `/api/tailscale/instances/{key}/restart` | `tailscale:write` | Reiniciar una instancia de Tailscale |
| POST | `/api/tailscale/instances/{key}/logout` | `tailscale:write` | Cerrar la sesión de una instancia de Tailscale |
| GET | `/api/tailscale/settings` | `tailscale:read` | Leer la configuración del programa Tailscale |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Guardar la configuración del programa Tailscale |
| GET | `/api/tailscale/install` | `tailscale:read` | Ver la información de instalación y el progreso de un programa gestionado |
| POST | `/api/tailscale/install` | `tailscale:write` | Instalar o actualizar en línea un programa gestionado |
| POST | `/api/tailscale/install/check` | `tailscale:write` | Buscar actualizaciones de un programa gestionado |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | Cambiar a una versión anterior guardada |
| DELETE | `/api/tailscale/install` | `tailscale:write` | Desinstalar un programa gestionado |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | Eliminar una versión anterior guardada |
| POST | `/api/tailscale/reset` | `tailscale:write` | Restaurar los ajustes predeterminados del módulo |

## Red NetBird

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/netbird/logs` | `netbird:read` | Ver los registros de ejecución del módulo |
| GET | `/api/netbird/logs/sources` | `netbird:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/netbird/logs/export` | `netbird:read` | Exportar registros del módulo |
| DELETE | `/api/netbird/logs` | `netbird:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/netbird/instances` | `netbird:read` | Listar Instancia de NetBird |
| POST | `/api/netbird/instances` | `netbird:write` | Añadir una instancia de NetBird |
| POST | `/api/netbird/instances/{key}/copy` | `netbird:write` | Duplicar Instancia de NetBird |
| PUT | `/api/netbird/instances/{key}` | `netbird:write` | Modificar Instancia de NetBird |
| DELETE | `/api/netbird/instances/{key}` | `netbird:write` | Eliminar Instancia de NetBird |
| PUT | `/api/netbird/instances/{key}/enable` | `netbird:write` | Activar o desactivar Instancia de NetBird |
| PUT | `/api/netbird/instances-order` | `netbird:write` | Reordenar Instancia de NetBird |
| PUT | `/api/netbird/instances-enable` | `netbird:write` | Activar o desactivar en lote: Instancia de NetBird |
| POST | `/api/netbird/instances-delete` | `netbird:write` | Eliminar en lote: Instancia de NetBird |
| PUT | `/api/netbird/instances-group` | `netbird:write` | Asignar grupo a varios elementos: Instancia de NetBird |
| GET | `/api/netbird/instances/{key}/secrets` | `netbird:write` | Ver secretos en texto plano |
| GET | `/api/netbird/status` | `netbird:read` | Ver el estado de conexión de todas las instancias de NetBird |
| GET | `/api/netbird/instances/{key}/status` | `netbird:read` | Ver el estado de conexión de una instancia de NetBird |
| POST | `/api/netbird/instances/{key}/restart` | `netbird:write` | Reiniciar una instancia de NetBird |
| POST | `/api/netbird/instances/{key}/logout` | `netbird:write` | Cerrar sesión en una instancia de NetBird |
| GET | `/api/netbird/settings` | `netbird:read` | Leer los ajustes del programa NetBird |
| PUT | `/api/netbird/settings` | `netbird:write` | Guardar los ajustes del programa NetBird |
| GET | `/api/netbird/install` | `netbird:read` | Ver la información de instalación y el progreso de un programa gestionado |
| POST | `/api/netbird/install` | `netbird:write` | Instalar o actualizar en línea un programa gestionado |
| POST | `/api/netbird/install/check` | `netbird:write` | Buscar actualizaciones de un programa gestionado |
| POST | `/api/netbird/install/rollback` | `netbird:write` | Cambiar a una versión anterior guardada |
| DELETE | `/api/netbird/install` | `netbird:write` | Desinstalar un programa gestionado |
| DELETE | `/api/netbird/install/backups/{id}` | `netbird:write` | Eliminar una versión anterior guardada |
| POST | `/api/netbird/reset` | `netbird:write` | Restaurar los ajustes predeterminados del módulo |

## Red EasyTier

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/easytier/logs` | `easytier:read` | Ver los registros de ejecución del módulo |
| GET | `/api/easytier/logs/sources` | `easytier:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/easytier/logs/export` | `easytier:read` | Exportar registros del módulo |
| DELETE | `/api/easytier/logs` | `easytier:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/easytier/instances` | `easytier:read` | Listar Instancia de EasyTier |
| POST | `/api/easytier/instances` | `easytier:write` | Añadir una instancia de EasyTier |
| POST | `/api/easytier/instances/{key}/copy` | `easytier:write` | Duplicar Instancia de EasyTier |
| PUT | `/api/easytier/instances/{key}` | `easytier:write` | Modificar Instancia de EasyTier |
| DELETE | `/api/easytier/instances/{key}` | `easytier:write` | Eliminar Instancia de EasyTier |
| PUT | `/api/easytier/instances/{key}/enable` | `easytier:write` | Activar o desactivar Instancia de EasyTier |
| PUT | `/api/easytier/instances-order` | `easytier:write` | Reordenar Instancia de EasyTier |
| PUT | `/api/easytier/instances-enable` | `easytier:write` | Activar o desactivar en lote: Instancia de EasyTier |
| POST | `/api/easytier/instances-delete` | `easytier:write` | Eliminar en lote: Instancia de EasyTier |
| PUT | `/api/easytier/instances-group` | `easytier:write` | Asignar grupo a varios elementos: Instancia de EasyTier |
| GET | `/api/easytier/instances/{key}/secrets` | `easytier:write` | Ver secretos en texto plano |
| GET | `/api/easytier/status` | `easytier:read` | Ver el estado de ejecución de todas las instancias de EasyTier |
| GET | `/api/easytier/instances/{key}/status` | `easytier:read` | Ver el estado de ejecución de una instancia de EasyTier |
| POST | `/api/easytier/instances/{key}/restart` | `easytier:write` | Reiniciar una instancia de EasyTier |
| GET | `/api/easytier/settings` | `easytier:read` | Leer la configuración del programa EasyTier |
| PUT | `/api/easytier/settings` | `easytier:write` | Guardar la configuración del programa EasyTier |
| GET | `/api/easytier/install` | `easytier:read` | Ver la información de instalación y el progreso de un programa gestionado |
| POST | `/api/easytier/install` | `easytier:write` | Instalar o actualizar en línea un programa gestionado |
| POST | `/api/easytier/install/check` | `easytier:write` | Buscar actualizaciones de un programa gestionado |
| POST | `/api/easytier/install/rollback` | `easytier:write` | Cambiar a una versión anterior guardada |
| DELETE | `/api/easytier/install` | `easytier:write` | Desinstalar un programa gestionado |
| DELETE | `/api/easytier/install/backups/{id}` | `easytier:write` | Eliminar una versión anterior guardada |
| POST | `/api/easytier/reset` | `easytier:write` | Restaurar los ajustes predeterminados del módulo |

## Traversal de red interna frp

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | Ver los registros de ejecución del módulo |
| GET | `/api/frp/logs/sources` | `frp:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/frp/logs/export` | `frp:read` | Exportar registros del módulo |
| DELETE | `/api/frp/logs` | `frp:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/frp/clients` | `frp:read` | Listar Cliente frp |
| POST | `/api/frp/clients` | `frp:write` | Añadir Cliente frp |
| POST | `/api/frp/clients/{key}/copy` | `frp:write` | Duplicar Cliente frp |
| PUT | `/api/frp/clients/{key}` | `frp:write` | Modificar Cliente frp |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | Eliminar Cliente frp |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | Activar o desactivar Cliente frp |
| PUT | `/api/frp/clients-order` | `frp:write` | Reordenar Cliente frp |
| PUT | `/api/frp/clients-enable` | `frp:write` | Activar o desactivar en lote: Cliente frp |
| POST | `/api/frp/clients-delete` | `frp:write` | Eliminar en lote: Cliente frp |
| PUT | `/api/frp/clients-group` | `frp:write` | Asignar grupo a varios elementos: Cliente frp |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | Ver secretos en texto plano |
| GET | `/api/frp/servers` | `frp:read` | Listar Servidor frp |
| POST | `/api/frp/servers` | `frp:write` | Añadir Servidor frp |
| POST | `/api/frp/servers/{key}/copy` | `frp:write` | Duplicar Servidor frp |
| PUT | `/api/frp/servers/{key}` | `frp:write` | Modificar Servidor frp |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | Eliminar Servidor frp |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | Activar o desactivar Servidor frp |
| PUT | `/api/frp/servers-order` | `frp:write` | Reordenar Servidor frp |
| PUT | `/api/frp/servers-enable` | `frp:write` | Activar o desactivar en lote: Servidor frp |
| POST | `/api/frp/servers-delete` | `frp:write` | Eliminar en lote: Servidor frp |
| PUT | `/api/frp/servers-group` | `frp:write` | Asignar grupo a varios elementos: Servidor frp |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | Ver secretos en texto plano |
| GET | `/api/frp/proxies` | `frp:read` | Listar proxies de frp |
| POST | `/api/frp/proxies` | `frp:write` | Añadir proxies de frp |
| POST | `/api/frp/proxies/{key}/copy` | `frp:write` | Duplicar proxies de frp |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | Modificar proxies de frp |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | Eliminar proxies de frp |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | Activar o desactivar proxies de frp |
| PUT | `/api/frp/proxies-order` | `frp:write` | Reordenar proxies de frp |
| PUT | `/api/frp/proxies-enable` | `frp:write` | Activar o desactivar en lote: proxies de frp |
| POST | `/api/frp/proxies-delete` | `frp:write` | Eliminar en lote: proxies de frp |
| PUT | `/api/frp/proxies-group` | `frp:write` | Asignar grupo a varios elementos: proxies de frp |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | Ver secretos en texto plano |
| POST | `/api/frp/import` | `frp:write` | Importar configuraciones existentes de frpc / frps (admite ini antiguo y toml nuevo); con apply en false solo se muestra una vista previa |
| GET | `/api/frp/install` | `frp:read` | Ver la información de instalación y el progreso de un programa gestionado |
| POST | `/api/frp/install` | `frp:write` | Instalar o actualizar en línea un programa gestionado |
| POST | `/api/frp/install/check` | `frp:write` | Buscar actualizaciones de un programa gestionado |
| POST | `/api/frp/install/rollback` | `frp:write` | Cambiar a una versión anterior guardada |
| DELETE | `/api/frp/install` | `frp:write` | Desinstalar un programa gestionado |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | Eliminar una versión anterior guardada |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | Reiniciar instancia de frpc |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | Reiniciar instancia de frps |
| GET | `/api/frp/settings` | `frp:read` | Obtener rutas y versiones de los programas |
| PUT | `/api/frp/settings` | `frp:write` | Guardar rutas de los programas |
| POST | `/api/frp/reset` | `frp:write` | Restaurar los ajustes predeterminados del módulo |

## Túnel de Cloudflare

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | Ver los registros de ejecución del módulo |
| GET | `/api/cloudflared/logs/sources` | `cloudflared:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/cloudflared/logs/export` | `cloudflared:read` | Exportar registros del módulo |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | Listar Túnel de Cloudflare |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Añadir Túnel de Cloudflare |
| POST | `/api/cloudflared/tunnels/{key}/copy` | `cloudflared:write` | Duplicar Túnel de Cloudflare |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Modificar Túnel de Cloudflare |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Eliminar Túnel de Cloudflare |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Activar o desactivar Túnel de Cloudflare |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Reordenar Túnel de Cloudflare |
| PUT | `/api/cloudflared/tunnels-enable` | `cloudflared:write` | Activar o desactivar en lote: Túnel de Cloudflare |
| POST | `/api/cloudflared/tunnels-delete` | `cloudflared:write` | Eliminar en lote: Túnel de Cloudflare |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Asignar grupo a varios elementos: Túnel de Cloudflare |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | Ver secretos en texto plano |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Reiniciar un túnel de Cloudflare |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | Leer las rutas de un túnel con token |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | Guardar las rutas de un túnel con token |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Leer los ajustes del túnel de Cloudflare |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Guardar los ajustes del túnel de Cloudflare |
| GET | `/api/cloudflared/install` | `cloudflared:read` | Ver la información de instalación y el progreso de un programa gestionado |
| POST | `/api/cloudflared/install` | `cloudflared:write` | Instalar o actualizar en línea un programa gestionado |
| POST | `/api/cloudflared/install/check` | `cloudflared:write` | Buscar actualizaciones de un programa gestionado |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | Cambiar a una versión anterior guardada |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | Desinstalar un programa gestionado |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | Eliminar una versión anterior guardada |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | Restaurar los ajustes predeterminados del módulo |

## Certificado SSL

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | Ver los registros de ejecución del módulo |
| GET | `/api/ssl/logs/sources` | `ssl:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/ssl/logs/export` | `ssl:read` | Exportar registros del módulo |
| DELETE | `/api/ssl/logs` | `ssl:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/ssl/certs` | `ssl:read` | Listar Certificado |
| POST | `/api/ssl/certs` | `ssl:write` | Añadir Certificado |
| POST | `/api/ssl/certs/{key}/copy` | `ssl:write` | Duplicar Certificado |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | Modificar Certificado |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | Eliminar Certificado |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | Activar o desactivar Certificado |
| PUT | `/api/ssl/certs-order` | `ssl:write` | Reordenar Certificado |
| PUT | `/api/ssl/certs-enable` | `ssl:write` | Activar o desactivar en lote: Certificado |
| POST | `/api/ssl/certs-delete` | `ssl:write` | Eliminar en lote: Certificado |
| PUT | `/api/ssl/certs-group` | `ssl:write` | Asignar grupo a varios elementos: Certificado |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | Ver secretos en texto plano |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | Emitir o renovar un certificado ahora |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | Descargar certificado y clave privada |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | Desplegar un certificado ahora |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | Listar destinos de despliegue disponibles |
| GET | `/api/ssl/providers` | `ssl:read` | Listar los proveedores compatibles con validación DNS |
| GET | `/api/ssl/settings` | `ssl:read` | Leer los ajustes globales de certificados |
| PUT | `/api/ssl/settings` | `ssl:write` | Guardar los ajustes globales de certificados |

## Firewall WAF

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | Ver los registros de ejecución del módulo |
| GET | `/api/waf/logs/sources` | `waf:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/waf/logs/export` | `waf:read` | Exportar registros del módulo |
| DELETE | `/api/waf/logs` | `waf:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/waf/instances` | `waf:read` | Listar Instancia WAF |
| POST | `/api/waf/instances` | `waf:write` | Añadir Instancia WAF |
| POST | `/api/waf/instances/{key}/copy` | `waf:write` | Duplicar Instancia WAF |
| PUT | `/api/waf/instances/{key}` | `waf:write` | Modificar Instancia WAF |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | Eliminar Instancia WAF |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | Activar o desactivar Instancia WAF |
| PUT | `/api/waf/instances-order` | `waf:write` | Reordenar Instancia WAF |
| PUT | `/api/waf/instances-enable` | `waf:write` | Activar o desactivar en lote: Instancia WAF |
| POST | `/api/waf/instances-delete` | `waf:write` | Eliminar en lote: Instancia WAF |
| PUT | `/api/waf/instances-group` | `waf:write` | Asignar grupo a varios elementos: Instancia WAF |
| POST | `/api/waf/safeline/test` | `waf:write` | Probar el motor de detección SafeLine |
| GET | `/api/waf/crs` | `waf:read` | Listar archivos de reglas OWASP CRS |
| GET | `/api/waf/crs/{file}` | `waf:read` | Leer un archivo de reglas CRS |

## Servicio web

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | Ver los registros de ejecución del módulo |
| GET | `/api/webservice/logs/sources` | `webservice:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/webservice/logs/export` | `webservice:read` | Exportar registros del módulo |
| DELETE | `/api/webservice/logs` | `webservice:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/webservice/listeners` | `webservice:read` | Listar Escucha del servicio web |
| POST | `/api/webservice/listeners` | `webservice:write` | Añadir Escucha del servicio web |
| POST | `/api/webservice/listeners/{key}/copy` | `webservice:write` | Duplicar Escucha del servicio web |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Modificar Escucha del servicio web |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Eliminar Escucha del servicio web |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Activar o desactivar Escucha del servicio web |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Reordenar Escucha del servicio web |
| PUT | `/api/webservice/listeners-enable` | `webservice:write` | Activar o desactivar en lote: Escucha del servicio web |
| POST | `/api/webservice/listeners-delete` | `webservice:write` | Eliminar en lote: Escucha del servicio web |
| GET | `/api/webservice/iplists` | `webservice:read` | Listar las listas de IP suscritas |
| POST | `/api/webservice/iplists` | `webservice:write` | Añadir Lista de IP suscrita |
| POST | `/api/webservice/iplists/{key}/copy` | `webservice:write` | Duplicar Lista de IP suscrita |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | Modificar Lista de IP suscrita |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | Eliminar Lista de IP suscrita |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | Activar o desactivar Lista de IP suscrita |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | Reordenar Lista de IP suscrita |
| PUT | `/api/webservice/iplists-enable` | `webservice:write` | Activar o desactivar en lote: Lista de IP suscrita |
| POST | `/api/webservice/iplists-delete` | `webservice:write` | Eliminar en lote: Lista de IP suscrita |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | Actualizar ahora una lista de IP suscrita |
| GET | `/api/webservice/errorpages` | `webservice:read` | Lista de páginas de error personalizadas (sin el contenido de las páginas) |
| POST | `/api/webservice/errorpages` | `webservice:write` | Añadir una página de error personalizada: la página subida se depura antes de guardarse; en una página de archivo se comprueba que el archivo esté disponible |
| GET | `/api/webservice/errorpages/{key}` | `webservice:read` | Leer una página de error personalizada (incluido el contenido de la página) |
| PUT | `/api/webservice/errorpages/{key}` | `webservice:write` | Modificar una página de error personalizada |
| DELETE | `/api/webservice/errorpages/{key}` | `webservice:write` | Eliminar una página de error personalizada (se rechaza si algún sitio aún la usa) |
| POST | `/api/webservice/errorpages/check` | `webservice:write` | Comprueba la seguridad y la usabilidad de una página y genera una vista previa sin guardar |
| GET | `/api/webservice/errorpages/styles` | `webservice:read` | Vista previa del estilo predeterminado y de todos los estilos integrados; ?code= indica el código de estado (por defecto 502) |
| GET | `/api/webservice/errorpages/{key}/preview` | `webservice:read` | Vista previa de una página de error personalizada; ?code= indica el código de estado (por defecto 502) |
| GET | `/api/webservice/panels` | `webservice:read` | Listar Paneles de navegación |
| POST | `/api/webservice/panels` | `webservice:write` | Añadir Paneles de navegación |
| POST | `/api/webservice/panels/{key}/copy` | `webservice:write` | Duplicar Paneles de navegación |
| PUT | `/api/webservice/panels/{key}` | `webservice:write` | Modificar Paneles de navegación |
| DELETE | `/api/webservice/panels/{key}` | `webservice:write` | Eliminar Paneles de navegación |
| PUT | `/api/webservice/panels/{key}/enable` | `webservice:write` | Activar o desactivar Paneles de navegación |
| PUT | `/api/webservice/panels-order` | `webservice:write` | Reordenar Paneles de navegación |
| PUT | `/api/webservice/panels-enable` | `webservice:write` | Activar o desactivar en lote: Paneles de navegación |
| POST | `/api/webservice/panels-delete` | `webservice:write` | Eliminar en lote: Paneles de navegación |
| POST | `/api/webservice/panels/preview` | `webservice:write` | Vista previa del panel de navegación |
| GET | `/api/webservice/panels/icons` | `webservice:read` | Iconos integrados de los paneles de navegación |
| GET | `/api/webservice/users` | `webservice:read` | Leer usuarios compartidos y proveedor de identidad OIDC |
| PUT | `/api/webservice/users` | `webservice:write` | Guardar usuarios compartidos y proveedor de identidad OIDC, configurar sitios en bloque |
| POST | `/api/webservice/sites/batch` | `webservice:write` | Operaciones masivas de sitios: activar, desactivar, eliminar, añadir, quitar o reemplazar dominios, cambiar algunas opciones o mover a otro listener; solo se guarda si todos los sitios pasan la validación y no se cambia nada si alguno falla |
| GET | `/api/webservice/cache` | `webservice:read` | Número de entradas y tamaño de la caché de respuestas del sitio (?listener=&site=) |
| POST | `/api/webservice/cache/purge` | `webservice:write` | Vacía la caché de respuestas del sitio: deja path vacío para vaciarla entera; si no, solo las entradas cuya ruta coincida (misma sintaxis que las rutas de las reglas de caché, admite *) |
| POST | `/api/webservice/import/nginx` | `webservice:write` | Importar configuración de nginx |
| GET | `/api/webservice/settings` | `webservice:read` | Leer los ajustes globales del servicio web |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | Ver en texto plano los secretos TOTP de los usuarios compartidos (por nombre de usuario) |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | Ver en texto plano los Client Secret de los inicios de sesión de terceros (por identificador del método) |
| GET | `/api/webservice/settings/secrets/marker` | `webservice:write` | Ver la clave de la marca de autenticación en texto plano (authMarkerKey) |
| POST | `/api/webservice/totp-secret` | `webservice:write` | Generar una nueva clave TOTP para un usuario compartido (no se guarda; se aplica al guardar la configuración) |
| PUT | `/api/webservice/settings` | `webservice:write` | Guardar los ajustes globales del servicio web |
| GET | `/api/webservice/stats` | `webservice:read` | Estadísticas por minuto del sitio (?listener=&site=&minutes=) |
| GET | `/api/webservice/metrics` | `webservice:read` | Métricas en formato de texto de Prometheus |
| GET | `/api/webservice/topology` | `webservice:read` | Instantánea de la topología de Servicios web |
| GET | `/api/webservice/visitors` | `webservice:read` | Lista de auditoría de visitantes |
| GET | `/api/webservice/visitors/detail` | `webservice:read` | Registro de auditoría de un visitante |
| POST | `/api/webservice/visitors/delete` | `webservice:write` | Eliminar registros de auditoría de visitantes |
| GET | `/api/webservice/visitors/settings` | `webservice:read` | Leer la configuración y el estado de almacenamiento de la auditoría de visitantes |
| PUT | `/api/webservice/visitors/settings` | `webservice:write` | Guardar la configuración de auditoría de visitantes |
| GET | `/api/webservice/topology/replay` | `webservice:read` | Resumen de la reproducción de topología |
| GET | `/api/webservice/topology/replay/frame` | `webservice:read` | Instantánea de reproducción de topología en un momento |
| PUT | `/api/webservice/topology/replay/settings` | `webservice:write` | Cambiar los ajustes de reproducción de topología |
| GET | `/api/webservice/connections` | `webservice:read` | Solicitudes en curso y solicitudes largas terminadas recientemente de un sitio (?listener=&site=&q=) |
| POST | `/api/webservice/connections/close` | `webservice:write` | Cerrar una solicitud en curso de un sitio y, opcionalmente, bloquear su dirección de origen (banSeconds -1 significa permanente) |
| GET | `/api/webservice/bans` | `webservice:read` | IP bloqueadas actualmente |
| POST | `/api/webservice/bans` | `webservice:write` | Bloquear manualmente una IP (seconds: 0 significa 24 horas, -1 significa permanente) |
| DELETE | `/api/webservice/bans` | `webservice:write` | Quitar un bloqueo (el body es {"key": dirección}) |
| GET | `/api/webservice/geoip` | `webservice:read` | Estado de la base de datos GeoIP |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | Subir la base de datos GeoIP (campo multipart file, .mmdb) |
| POST | `/api/webservice/geoip/download` | `webservice:write` | Descargar la base de datos GeoIP (.mmdb) desde una dirección |
| DELETE | `/api/webservice/geoip` | `webservice:write` | Quitar la base de datos GeoIP |

## Prevención de intrusiones

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | Ver los registros de ejecución del módulo |
| GET | `/api/guard/logs/sources` | `guard:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/guard/logs/export` | `guard:read` | Exportar registros del módulo |
| DELETE | `/api/guard/logs` | `guard:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/guard/jails` | `guard:read` | Listar regla de prevención de intrusiones |
| POST | `/api/guard/jails` | `guard:write` | Añadir regla de prevención de intrusiones |
| POST | `/api/guard/jails/{key}/copy` | `guard:write` | Duplicar regla de prevención de intrusiones |
| PUT | `/api/guard/jails/{key}` | `guard:write` | Modificar regla de prevención de intrusiones |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | Eliminar regla de prevención de intrusiones |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | Activar o desactivar regla de prevención de intrusiones |
| PUT | `/api/guard/jails-order` | `guard:write` | Reordenar regla de prevención de intrusiones |
| PUT | `/api/guard/jails-enable` | `guard:write` | Activar o desactivar en lote: regla de prevención de intrusiones |
| POST | `/api/guard/jails-delete` | `guard:write` | Eliminar en lote: regla de prevención de intrusiones |
| PUT | `/api/guard/jails-group` | `guard:write` | Asignar grupo a varios elementos: regla de prevención de intrusiones |
| GET | `/api/guard/presets` | `guard:read` | Plantillas de reglas para servicios habituales |
| POST | `/api/guard/jails/test` | `guard:write` | Probar una regla de registro con ejemplos |
| GET | `/api/guard/status` | `guard:read` | Estado de sincronización del cortafuegos y entorno |
| GET | `/api/guard/settings` | `guard:read` | Ajustes globales |
| PUT | `/api/guard/settings` | `guard:write` | Modificar la configuración global |
| GET | `/api/guard/bans` | `guard:read` | IP bloqueadas actualmente |
| POST | `/api/guard/bans` | `guard:write` | Bloquear manualmente una IP o red (seconds: 0 usa la duración predeterminada, -1 significa permanente) |
| DELETE | `/api/guard/bans` | `guard:write` | Quitar un bloqueo (el body es {"key": dirección}) |
| GET | `/api/guard/firewall/rules` | `guard:read` | Reglas que Gately escribió en el cortafuegos del sistema (texto) |

## Port knocking

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/knock/logs` | `knock:read` | Ver los registros de ejecución del módulo |
| GET | `/api/knock/logs/sources` | `knock:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/knock/logs/export` | `knock:read` | Exportar registros del módulo |
| DELETE | `/api/knock/logs` | `knock:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/knock/rules` | `knock:read` | Listar Reglas de port knocking |
| POST | `/api/knock/rules` | `knock:write` | Añadir Reglas de port knocking |
| POST | `/api/knock/rules/{key}/copy` | `knock:write` | Duplicar Reglas de port knocking |
| PUT | `/api/knock/rules/{key}` | `knock:write` | Modificar Reglas de port knocking |
| DELETE | `/api/knock/rules/{key}` | `knock:write` | Eliminar Reglas de port knocking |
| PUT | `/api/knock/rules/{key}/enable` | `knock:write` | Activar o desactivar Reglas de port knocking |
| PUT | `/api/knock/rules-order` | `knock:write` | Reordenar Reglas de port knocking |
| PUT | `/api/knock/rules-enable` | `knock:write` | Activar o desactivar en lote: Reglas de port knocking |
| POST | `/api/knock/rules-delete` | `knock:write` | Eliminar en lote: Reglas de port knocking |
| PUT | `/api/knock/rules-group` | `knock:write` | Asignar grupo a varios elementos: Reglas de port knocking |
| GET | `/api/knock/rules/{key}/secrets` | `knock:write` | Ver secretos en texto plano |
| GET | `/api/knock/settings` | `knock:read` | Ajustes globales |
| PUT | `/api/knock/settings` | `knock:write` | Modificar la configuración global |
| GET | `/api/knock/status` | `knock:read` | Escucha, estado del bloqueo por firewall y autorizaciones vigentes |
| DELETE | `/api/knock/grants` | `knock:write` | Revocar autorizaciones (el body es {"addr": dirección, "rule": clave de regla}; deje rule vacío para revocar todas las autorizaciones de la dirección) |
| GET | `/api/knock/firewall/rules` | `knock:read` | Reglas de bloqueo que Gately escribió en el firewall del sistema (texto) |

## Wake-on-LAN

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | Ver los registros de ejecución del módulo |
| GET | `/api/wol/logs/sources` | `wol:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/wol/logs/export` | `wol:read` | Exportar registros del módulo |
| DELETE | `/api/wol/logs` | `wol:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/wol/devices` | `wol:read` | Listar Activar un dispositivo |
| POST | `/api/wol/devices` | `wol:write,webterminal:write` | Añadir Activar un dispositivo |
| POST | `/api/wol/devices/{key}/copy` | `wol:write,webterminal:write` | Duplicar Activar un dispositivo |
| PUT | `/api/wol/devices/{key}` | `wol:write,webterminal:write` | Modificar Activar un dispositivo |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | Eliminar Activar un dispositivo |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | Activar o desactivar Activar un dispositivo |
| PUT | `/api/wol/devices-order` | `wol:write` | Reordenar Activar un dispositivo |
| PUT | `/api/wol/devices-enable` | `wol:write` | Activar o desactivar en lote: Activar un dispositivo |
| POST | `/api/wol/devices-delete` | `wol:write` | Eliminar en lote: Activar un dispositivo |
| PUT | `/api/wol/devices-group` | `wol:write` | Asignar grupo a varios elementos: Activar un dispositivo |
| GET | `/api/wol/devices/{key}/secrets` | `wol:write` | Ver secretos en texto plano |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | Enviar un paquete mágico de activación al dispositivo |
| POST | `/api/wol/devices/{key}/shutdown` | `wol:write,webterminal:write` | Apagado remoto (el método de agente también permite reiniciar y suspender) |
| GET | `/api/wol/agents` | `wol:read` | Información de conexión de los agentes, indexada por la key del dispositivo |
| POST | `/api/wol/devices/{key}/agent-token` | `wol:write` | Regenerar el token del agente; los agentes conectados se desconectarán de inmediato |
| GET | `/api/wol/agent-script/{name}` | `wol:read` | Descargar el script del agente (gately-agent.sh o gately-agent.ps1) |
| GET | `/api/wol/agent-scripts` | `wol:read` | SHA-256 de los scripts del agente, por nombre de archivo; los comandos de instalación lo usan para verificar el script descargado |
| GET | `/api/wol/status` | `wol:read` | Ver el estado en línea del dispositivo |
| GET | `/api/wol/ssh-targets` | `wol:read,webterminal:read` | Listar las conexiones SSH utilizables para apagado remoto |
| GET | `/api/wol/bemfa` | `wol:read` | Configuración y estado de conexión de Bemfa |
| GET | `/api/wol/bemfa/secrets` | `wol:write` | Ver la clave privada de Bemfa |
| PUT | `/api/wol/bemfa` | `wol:write` | Modificar la configuración de Bemfa |
| POST | `/api/wol/public/agent/poll` | Sin inicio de sesión | Sondeo largo de comandos del agente de Wake-on-LAN |
| POST | `/api/wol/public/agent/ack` | Sin inicio de sesión | El agente de Wake-on-LAN confirma un comando o informa de su resultado |
| GET | `/api/wol/public/agent/{name}` | Sin inicio de sesión | Descargar un script del agente de Wake-on-LAN |
| POST | `/api/wol/public/agent/{name}` | Sin inicio de sesión | Descargar un script del agente de Wake-on-LAN |

## Terminal web

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | Ver los registros de ejecución del módulo |
| GET | `/api/webterminal/logs/sources` | `webterminal:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/webterminal/logs/export` | `webterminal:read` | Exportar registros del módulo |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/webterminal/connections` | `webterminal:read` | Listar Conexión de terminal |
| POST | `/api/webterminal/connections` | `webterminal:write` | Añadir Conexión de terminal |
| POST | `/api/webterminal/connections/{key}/copy` | `webterminal:write` | Duplicar Conexión de terminal |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | Modificar Conexión de terminal |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | Eliminar Conexión de terminal |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | Activar o desactivar Conexión de terminal |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | Reordenar Conexión de terminal |
| PUT | `/api/webterminal/connections-enable` | `webterminal:write` | Activar o desactivar en lote: Conexión de terminal |
| POST | `/api/webterminal/connections-delete` | `webterminal:write` | Eliminar en lote: Conexión de terminal |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | Asignar grupo a varios elementos: Conexión de terminal |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | Ver secretos en texto plano |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | Borrar la clave de host registrada |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | Probar una conexión guardada |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | Probar parámetros de conexión temporales |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | Accesos rápidos de conexión |
| GET | `/api/webterminal/credentials` | `webterminal:read` | Listar Credencial predefinida de terminal |
| POST | `/api/webterminal/credentials` | `webterminal:write` | Añadir Credencial predefinida de terminal |
| POST | `/api/webterminal/credentials/{key}/copy` | `webterminal:write` | Duplicar Credencial predefinida de terminal |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | Modificar Credencial predefinida de terminal |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | Eliminar Credencial predefinida de terminal |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | Activar o desactivar Credencial predefinida de terminal |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | Reordenar Credencial predefinida de terminal |
| PUT | `/api/webterminal/credentials-enable` | `webterminal:write` | Activar o desactivar en lote: Credencial predefinida de terminal |
| POST | `/api/webterminal/credentials-delete` | `webterminal:write` | Eliminar en lote: Credencial predefinida de terminal |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | Ver secretos en texto plano |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | Abrir una sesión de terminal (WebSocket) |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | Unirse a una sesión de terminal existente (WebSocket) |
| GET | `/api/webterminal/sessions` | `webterminal:read` | Listar sesiones de terminal |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | Ver los detalles de una sesión de terminal |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | Modificar la nota de la sesión de terminal |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | Ver las estadísticas de sesiones de terminal |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | Cerrar la sesión de terminal |
| GET | `/api/webterminal/security` | Solo sesión iniciada | Ver el estado de los ajustes de seguridad del terminal |
| POST | `/api/webterminal/security/agreement` | Solo sesión iniciada | Confirmar el contrato de uso del terminal |
| POST | `/api/webterminal/security/2fa/setup` | Solo sesión iniciada | Generar la clave de verificación en dos pasos del terminal |
| POST | `/api/webterminal/security/2fa/enable` | Solo sesión iniciada | Activar la verificación en dos pasos del terminal |
| POST | `/api/webterminal/security/2fa/disable` | Solo sesión iniciada | Desactivar la verificación en dos pasos del terminal |
| POST | `/api/webterminal/security/verify2fa` | Solo sesión iniciada | Verificar el código de verificación en dos pasos del terminal |
| GET | `/api/webterminal/security/check2fa` | Solo sesión iniciada | Comprobar si se superó la verificación en dos pasos del terminal |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | Listar el directorio remoto |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | Leer un archivo remoto |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | Escribir un archivo remoto |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | Subir un archivo al host remoto |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | Crear un directorio remoto |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | Crear un archivo vacío remoto |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | Renombrar o mover un archivo remoto |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | Copiar un archivo remoto |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | Modificar los permisos de un archivo remoto |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | Eliminar un archivo remoto |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | Comprimir archivos en el host remoto |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | Descomprimir archivos en el host remoto |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | Previsualizar el contenido de un archivo comprimido remoto |
| GET | `/api/webterminal/settings` | `webterminal:read` | Leer los ajustes del terminal web |
| PUT | `/api/webterminal/settings` | `webterminal:write` | Guardar los ajustes del terminal web |
| GET | `/api/webterminal/shells` | `webterminal:read` | Listar los shells disponibles en este equipo |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | Leer los comandos rápidos globales |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | Guardar los comandos rápidos globales |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | Leer la disposición de paneles divididos |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | Guardar la disposición de paneles divididos |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | Borrar la disposición de paneles divididos |

## Tarea programada

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | Ver los registros de ejecución del módulo |
| GET | `/api/cron/logs/sources` | `cron:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/cron/logs/export` | `cron:read` | Exportar registros del módulo |
| DELETE | `/api/cron/logs` | `cron:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/cron/tasks` | `cron:read` | Listar Tarea programada |
| POST | `/api/cron/tasks` | `cron:write` | Añadir Tarea programada |
| POST | `/api/cron/tasks/{key}/copy` | `cron:write` | Duplicar Tarea programada |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | Modificar Tarea programada |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | Eliminar Tarea programada |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | Activar o desactivar Tarea programada |
| PUT | `/api/cron/tasks-order` | `cron:write` | Reordenar Tarea programada |
| PUT | `/api/cron/tasks-enable` | `cron:write` | Activar o desactivar en lote: Tarea programada |
| POST | `/api/cron/tasks-delete` | `cron:write` | Eliminar en lote: Tarea programada |
| PUT | `/api/cron/tasks-group` | `cron:write` | Asignar grupo a varios elementos: Tarea programada |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | Ejecutar ahora una tarea programada |
| POST | `/api/cron/parse` | `cron:write` | Analizar y previsualizar una expresión Cron |

## Administración de Docker

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | Ver los registros de ejecución del módulo |
| GET | `/api/docker/logs/sources` | `docker:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/docker/logs/export` | `docker:read` | Exportar registros del módulo |
| DELETE | `/api/docker/logs` | `docker:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/docker/tasks` | `docker:read` | Listar tareas en segundo plano |
| GET | `/api/docker/tasks/{id}` | `docker:read` | Ver el progreso y la salida de una tarea en segundo plano |
| POST | `/api/docker/tasks/{id}/cancel` | `docker:write` | Cancelar una tarea en segundo plano en curso |
| DELETE | `/api/docker/tasks` | `docker:write` | Limpiar las tareas en segundo plano finalizadas |
| GET | `/api/docker/images` | `docker:read` | Listar imágenes |
| POST | `/api/docker/images/pull` | `docker:write` | Descargar una imagen |
| POST | `/api/docker/images/push` | `docker:write` | Subir una imagen al registro |
| POST | `/api/docker/images/search` | `docker:write` | Buscar imágenes |
| POST | `/api/docker/images/load` | `docker:write` | Importar un archivo de imagen |
| POST | `/api/docker/images/build` | `docker:write` | Compilar una imagen |
| GET | `/api/docker/images/{id}` | `docker:read` | Ver los detalles de la imagen |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | Etiquetar la imagen |
| DELETE | `/api/docker/images/{id}` | `docker:write` | Eliminar una imagen |
| GET | `/api/docker/images/{id}/export` | `docker:write` | Exportar una imagen |
| GET | `/api/docker/networks` | `docker:read` | Listar redes |
| POST | `/api/docker/networks` | `docker:write` | Crear una red |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | Eliminar una red |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | Conectar el contenedor a la red |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | Desconectar el contenedor de la red |
| GET | `/api/docker/volumes` | `docker:read` | Listar volúmenes |
| POST | `/api/docker/volumes` | `docker:write` | Crear un volumen |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | Eliminar un volumen |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | Exportar un volumen |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | Importar datos a un volumen |
| GET | `/api/docker/compose/projects` | `docker:read` | Listar proyectos Compose |
| POST | `/api/docker/compose/projects` | `docker:write` | Crear un proyecto Compose |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | Iniciar un proyecto Compose |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | Descargar las imágenes de un proyecto Compose |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | Ejecutar una operación en un proyecto Compose |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | Leer el archivo de configuración de Compose |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | Guardar el archivo de configuración de Compose |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | Ver los registros de un proyecto Compose |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | Eliminar un proyecto Compose |
| GET | `/api/docker/info` | `docker:read` | Ver la información del motor Docker |
| GET | `/api/docker/disk-usage` | `docker:read` | Ver el uso de disco de Docker |
| POST | `/api/docker/prune` | `docker:write` | Limpiar recursos de Docker sin uso |
| GET | `/api/docker/settings` | `docker:read` | Leer los ajustes del módulo Docker |
| PUT | `/api/docker/settings` | `docker:write` | Guardar los ajustes del módulo Docker |
| GET | `/api/docker/settings/secrets` | `docker:write` | Ver en texto plano las contraseñas de los registros (claves `registry:<dirección del registro>`) |
| GET | `/api/docker/containers` | `docker:read` | Listar contenedores |
| POST | `/api/docker/containers` | `docker:write` | Crear un contenedor |
| POST | `/api/docker/containers/check-all` | `docker:write` | Comprobar actualizaciones de las imágenes de todos los contenedores |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | Actualizar contenedores por lotes |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | Comprobar si la imagen tiene una versión nueva |
| GET | `/api/docker/containers/{id}` | `docker:read` | Ver los detalles del contenedor |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | Ver en texto plano las variables de entorno y el comando de inicio del contenedor |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | Eliminar un contenedor |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | Modificar la configuración del contenedor y recrearlo |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | Renombrar un contenedor |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | Confirmar el contenedor como imagen |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | Comprobar si la imagen de un contenedor tiene actualizaciones |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | Actualizar la imagen del contenedor y recrearlo |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | Ejecutar una operación en un contenedor |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | Ver los registros del contenedor |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | Ver el uso de recursos del contenedor |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | Ver los procesos dentro del contenedor |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | Listar el directorio dentro del contenedor |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | Leer un archivo dentro del contenedor |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | Descargar un archivo dentro del contenedor |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | Escribir un archivo dentro del contenedor |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | Crear un directorio dentro del contenedor |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | Renombrar un archivo dentro del contenedor |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | Subir un archivo al contenedor |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | Eliminar un archivo dentro del contenedor |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | Abrir un terminal dentro del contenedor (WebSocket) |

## Servicio WebDAV

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | Ver los registros de ejecución del módulo |
| GET | `/api/webdav/logs/sources` | `webdav:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/webdav/logs/export` | `webdav:read` | Exportar registros del módulo |
| DELETE | `/api/webdav/logs` | `webdav:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/webdav/settings` | `webdav:read` | Leer los ajustes de WebDAV |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | Ver en texto plano las contraseñas de los usuarios locales |
| GET | `/api/webdav/shared-users` | `webdav:read` | Leer los usuarios compartidos de los servicios de archivos (endpoint antiguo) |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | Guardar los usuarios compartidos de los servicios de archivos (endpoint antiguo) |
| PUT | `/api/webdav/settings` | `webdav:write` | Guardar los ajustes de WebDAV |

## Servicio FTP

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | Ver los registros de ejecución del módulo |
| GET | `/api/ftp/logs/sources` | `ftp:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/ftp/logs/export` | `ftp:read` | Exportar registros del módulo |
| DELETE | `/api/ftp/logs` | `ftp:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/ftp/settings` | `ftp:read` | Leer los ajustes de FTP |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | Ver en texto plano las contraseñas de los usuarios locales |
| GET | `/api/ftp/shared-users` | `ftp:read` | Leer los usuarios compartidos de los servicios de archivos (endpoint antiguo) |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | Guardar los usuarios compartidos de los servicios de archivos (endpoint antiguo) |
| PUT | `/api/ftp/settings` | `ftp:write` | Guardar los ajustes de FTP |

## Servicio SMB

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | Ver los registros de ejecución del módulo |
| GET | `/api/smb/logs/sources` | `smb:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/smb/logs/export` | `smb:read` | Exportar registros del módulo |
| DELETE | `/api/smb/logs` | `smb:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/smb/settings` | `smb:read` | Obtener la configuración de SMB |
| GET | `/api/smb/settings/secrets` | `smb:write` | Ver en texto plano las contraseñas de los usuarios locales |
| GET | `/api/smb/shared-users` | `smb:read` | Leer los usuarios compartidos de los servicios de archivos (endpoint antiguo) |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | Guardar los usuarios compartidos de los servicios de archivos (endpoint antiguo) |
| PUT | `/api/smb/settings` | `smb:write` | Guardar la configuración de SMB |

## File Browser

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | Ver los registros de ejecución del módulo |
| GET | `/api/filebrowser/logs/sources` | `filebrowser:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/filebrowser/logs/export` | `filebrowser:read` | Exportar registros del módulo |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | Listar Instancia de File Browser |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | Añadir Instancia de File Browser |
| POST | `/api/filebrowser/instances/{key}/copy` | `filebrowser:write` | Duplicar Instancia de File Browser |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Modificar Instancia de File Browser |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Eliminar Instancia de File Browser |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | Activar o desactivar Instancia de File Browser |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | Reordenar Instancia de File Browser |
| PUT | `/api/filebrowser/instances-enable` | `filebrowser:write` | Activar o desactivar en lote: Instancia de File Browser |
| POST | `/api/filebrowser/instances-delete` | `filebrowser:write` | Eliminar en lote: Instancia de File Browser |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | Asignar grupo a varios elementos: Instancia de File Browser |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | Ver secretos en texto plano |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | Reiniciar una instancia de File Browser |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | Leer los ajustes del programa File Browser |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | Guardar los ajustes del programa File Browser |
| GET | `/api/filebrowser/install` | `filebrowser:read` | Ver la información de instalación y el progreso de un programa gestionado |
| POST | `/api/filebrowser/install` | `filebrowser:write` | Instalar o actualizar en línea un programa gestionado |
| POST | `/api/filebrowser/install/check` | `filebrowser:write` | Buscar actualizaciones de un programa gestionado |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | Cambiar a una versión anterior guardada |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | Desinstalar un programa gestionado |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | Eliminar una versión anterior guardada |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | Restaurar los ajustes predeterminados del módulo |

## Montajes de almacenamiento

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | Ver los registros de ejecución del módulo |
| GET | `/api/storage/logs/sources` | `storage:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/storage/logs/export` | `storage:read` | Exportar registros del módulo |
| DELETE | `/api/storage/logs` | `storage:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/storage/mounts` | `storage:read` | Listar Montajes de almacenamiento |
| POST | `/api/storage/mounts` | `storage:write` | Añadir Montajes de almacenamiento |
| POST | `/api/storage/mounts/{key}/copy` | `storage:write` | Duplicar Montajes de almacenamiento |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | Modificar Montajes de almacenamiento |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | Eliminar Montajes de almacenamiento |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | Activar o desactivar Montajes de almacenamiento |
| PUT | `/api/storage/mounts-order` | `storage:write` | Reordenar Montajes de almacenamiento |
| PUT | `/api/storage/mounts-enable` | `storage:write` | Activar o desactivar en lote: Montajes de almacenamiento |
| POST | `/api/storage/mounts-delete` | `storage:write` | Eliminar en lote: Montajes de almacenamiento |
| PUT | `/api/storage/mounts-group` | `storage:write` | Asignar grupo a varios elementos: Montajes de almacenamiento |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | Ver secretos en texto plano |
| GET | `/api/storage/files/{key}/list` | `storage:write` | Listar el directorio de almacenamiento |
| GET | `/api/storage/files/{key}/download` | `storage:write` | Descargar un archivo del almacenamiento |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | Subir un archivo al almacenamiento |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | Crear un directorio de almacenamiento |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | Renombrar o mover un archivo de almacenamiento |
| DELETE | `/api/storage/files/{key}` | `storage:write` | Eliminar un archivo de almacenamiento |

## Centro de notificaciones

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | Ver los registros de ejecución del módulo |
| GET | `/api/notify/logs/sources` | `notify:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/notify/logs/export` | `notify:read` | Exportar registros del módulo |
| DELETE | `/api/notify/logs` | `notify:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/notify/channels` | `notify:read` | Listar Canal de notificaciones |
| POST | `/api/notify/channels` | `notify:write` | Añadir Canal de notificaciones |
| POST | `/api/notify/channels/{key}/copy` | `notify:write` | Duplicar Canal de notificaciones |
| PUT | `/api/notify/channels/{key}` | `notify:write` | Modificar Canal de notificaciones |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | Eliminar Canal de notificaciones |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | Activar o desactivar Canal de notificaciones |
| PUT | `/api/notify/channels-order` | `notify:write` | Reordenar Canal de notificaciones |
| PUT | `/api/notify/channels-enable` | `notify:write` | Activar o desactivar en lote: Canal de notificaciones |
| POST | `/api/notify/channels-delete` | `notify:write` | Eliminar en lote: Canal de notificaciones |
| PUT | `/api/notify/channels-group` | `notify:write` | Asignar grupo a varios elementos: Canal de notificaciones |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | Ver secretos en texto plano |
| GET | `/api/notify/rules` | `notify:read` | Listar Regla de notificación |
| POST | `/api/notify/rules` | `notify:write` | Añadir Regla de notificación |
| POST | `/api/notify/rules/{key}/copy` | `notify:write` | Duplicar Regla de notificación |
| PUT | `/api/notify/rules/{key}` | `notify:write` | Modificar Regla de notificación |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | Eliminar Regla de notificación |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | Activar o desactivar Regla de notificación |
| PUT | `/api/notify/rules-order` | `notify:write` | Reordenar Regla de notificación |
| PUT | `/api/notify/rules-enable` | `notify:write` | Activar o desactivar en lote: Regla de notificación |
| POST | `/api/notify/rules-delete` | `notify:write` | Eliminar en lote: Regla de notificación |
| PUT | `/api/notify/rules-group` | `notify:write` | Asignar grupo a varios elementos: Regla de notificación |
| GET | `/api/notify/settings` | `notify:read` | Leer los ajustes de notificación |
| PUT | `/api/notify/settings` | `notify:write` | Guardar los ajustes de notificación |
| GET | `/api/notify/event-types` | `notify:read` | Tipos de evento a los que se puede suscribir |
| GET | `/api/notify/history` | `notify:read` | Registros de entrega recientes |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | Enviar una notificación de prueba al canal |

## Rendimiento

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/perf/logs` | `perf:read` | Ver los registros de ejecución del módulo |
| GET | `/api/perf/logs/sources` | `perf:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/perf/logs/export` | `perf:read` | Exportar registros del módulo |
| DELETE | `/api/perf/logs` | `perf:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/perf/rules` | `perf:read` | Listar Reglas de alerta de rendimiento |
| POST | `/api/perf/rules` | `perf:write` | Añadir Reglas de alerta de rendimiento |
| POST | `/api/perf/rules/{key}/copy` | `perf:write` | Duplicar Reglas de alerta de rendimiento |
| PUT | `/api/perf/rules/{key}` | `perf:write` | Modificar Reglas de alerta de rendimiento |
| DELETE | `/api/perf/rules/{key}` | `perf:write` | Eliminar Reglas de alerta de rendimiento |
| PUT | `/api/perf/rules/{key}/enable` | `perf:write` | Activar o desactivar Reglas de alerta de rendimiento |
| PUT | `/api/perf/rules-order` | `perf:write` | Reordenar Reglas de alerta de rendimiento |
| PUT | `/api/perf/rules-enable` | `perf:write` | Activar o desactivar en lote: Reglas de alerta de rendimiento |
| POST | `/api/perf/rules-delete` | `perf:write` | Eliminar en lote: Reglas de alerta de rendimiento |
| PUT | `/api/perf/rules-group` | `perf:write` | Asignar grupo a varios elementos: Reglas de alerta de rendimiento |
| GET | `/api/perf/metrics` | `perf:read` | Métricas disponibles para alertas |
| GET | `/api/perf/overview` | `perf:read` | Historial del host y del proceso (?range=60s|1h|24h) |
| GET | `/api/perf/sites` | `perf:read` | Métricas de cada sitio en aproximadamente los últimos 60 segundos |
| GET | `/api/perf/alerts` | `perf:read` | Alertas en curso (pendientes y activas) |
| GET | `/api/perf/processes` | `perf:read` | Procesos gestionados (frpc, tailscaled, etc.) con su uso de recursos actual y de los últimos 60 segundos aproximadamente |

## Diagnóstico de red

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | Ver los registros de ejecución del módulo |
| GET | `/api/nettools/logs/sources` | `nettools:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/nettools/logs/export` | `nettools:read` | Exportar registros del módulo |
| DELETE | `/api/nettools/logs` | `nettools:write` | Vaciar los registros de ejecución del módulo |
| POST | `/api/nettools/run` | `nettools:write` | Ejecutar una herramienta de diagnóstico y devolver la tarea (tool: ping / traceroute / tcp / dns / speed / scan) |
| GET | `/api/nettools/tasks` | `nettools:read` | Tareas de diagnóstico recientes |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | Detalles y salida de la tarea |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | Cancelar una tarea en ejecución o eliminar una tarea finalizada |
| GET | `/api/nettools/settings` | `nettools:read` | Ajustes de diagnóstico |
| PUT | `/api/nettools/settings` | `nettools:write` | Modificar la lista de sondeo prohibido |
| GET | `/api/nettools/networks` | `nettools:read` | Segmentos de red de este equipo que pueden usarse para el escaneo de red local |

## Copia de seguridad de la configuración

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | Ver los registros de ejecución del módulo |
| GET | `/api/backup/logs/sources` | `backup:read` | Listar todos los orígenes de los registros del módulo |
| GET | `/api/backup/logs/export` | `backup:read` | Exportar registros del módulo |
| DELETE | `/api/backup/logs` | `backup:write` | Vaciar los registros de ejecución del módulo |
| GET | `/api/backup/targets` | `backup:read` | Listar Destino de copia de seguridad |
| POST | `/api/backup/targets` | `backup:write` | Añadir Destino de copia de seguridad |
| POST | `/api/backup/targets/{key}/copy` | `backup:write` | Duplicar Destino de copia de seguridad |
| PUT | `/api/backup/targets/{key}` | `backup:write` | Modificar Destino de copia de seguridad |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | Eliminar Destino de copia de seguridad |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | Activar o desactivar Destino de copia de seguridad |
| PUT | `/api/backup/targets-order` | `backup:write` | Reordenar Destino de copia de seguridad |
| PUT | `/api/backup/targets-enable` | `backup:write` | Activar o desactivar en lote: Destino de copia de seguridad |
| POST | `/api/backup/targets-delete` | `backup:write` | Eliminar en lote: Destino de copia de seguridad |
| GET | `/api/backup/settings` | `backup:read` | Ajustes de copia de seguridad |
| PUT | `/api/backup/settings` | `backup:write` | Modificar los ajustes de copia de seguridad (enviar el marcador de posición como contraseña para conservarla) |
| GET | `/api/backup/secrets` | `backup:write` | Ver secretos en texto plano |
| POST | `/api/backup/run` | `backup:write` | Hacer una copia de seguridad ahora |
| GET | `/api/backup/status` | `backup:read` | Resultado de la última copia de seguridad y hora de la próxima ejecución |
| GET | `/api/backup/mounts` | `backup:read` | Puntos de montaje de almacenamiento que pueden usarse como destino de copia de seguridad |
| GET | `/api/backup/local` | Solo sesión iniciada | Lista de archivos de copia de seguridad locales |
| GET | `/api/backup/local/{name}` | Solo sesión iniciada | Descargar un archivo de copia de seguridad local |

## Usuarios compartidos de los servicios de archivos

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | Leer los usuarios compartidos de los servicios de archivos |
| PUT | `/api/fileusers` | `fileusers:write` | Guardar los usuarios compartidos de los servicios de archivos |
| GET | `/api/fileusers/secrets` | `fileusers:write` | Ver en texto plano las contraseñas de los usuarios compartidos |

## Sistema

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/session` | Cualquier token válido | Ver la identidad autenticada actual |
| GET | `/api/status` | `status:read` | Ver el estado del sistema y de cada módulo |
| GET | `/api/logs` | `logs:read` | Ver los registros del sistema |
| GET | `/api/logs/sources` | `logs:read` | Listar todos los orígenes de los registros del sistema |
| GET | `/api/logs/export` | `logs:read` | Exportar registros del sistema |
| GET | `/api/system/settings` | Solo sesión iniciada | Leer los ajustes globales |
| PUT | `/api/system/settings` | Solo sesión iniciada | Guardar los ajustes globales |
| GET | `/api/system/settings/secrets` | Solo sesión iniciada | Ver en texto plano los Client Secret del inicio de sesión de terceros |
| GET | `/api/system/info` | `status:read` | Ver la información del sistema |
| GET | `/api/system/update` | `status:read` | Buscar nuevas versiones y avisos de seguridad |
| GET | `/api/system/update/state` | `status:read` | Ver el resultado de la última comprobación de actualizaciones |
| POST | `/api/system/upgrade` | Solo sesión iniciada | Actualizar en línea a la última versión |
| GET | `/api/system/upgrade/status` | Solo sesión iniciada | Ver el progreso de la actualización en línea |
| POST | `/api/system/upgrade/cancel` | Solo sesión iniciada | Cancelar la actualización en línea |
| POST | `/api/system/upgrade/upload` | Solo sesión iniciada | Actualizar con un paquete subido |
| GET | `/api/system/backup` | Solo sesión iniciada | Descargar la copia de seguridad de la configuración |
| GET | `/api/system/diagnostics` | Solo sesión iniciada | Exportar paquete de diagnóstico anonimizado |
| GET | `/api/audit` | Solo sesión iniciada | Consultar el registro de auditoría |
| GET | `/api/metrics` | `metrics:read` | Métricas globales de Prometheus |
| GET | `/api/history` | Solo sesión iniciada | Listar las versiones del historial de configuración |
| GET | `/api/history/{name}/{id}/diff` | Solo sesión iniciada | Comparar una versión del historial con la configuración actual |
| POST | `/api/history/{name}/{id}/restore` | Solo sesión iniciada | Revertir una configuración a una versión del historial |
| POST | `/api/system/restore` | Solo sesión iniciada | Restaurar la configuración desde una copia de seguridad |
| POST | `/api/system/restart` | Solo sesión iniciada | Reiniciar Gately |
| POST | `/api/system/selfcheck` | Solo sesión iniciada | Autodiagnóstico de red y configuración |
| GET | `/api/system/onboarding` | Solo sesión iniciada | Si la configuración inicial está pendiente |
| POST | `/api/system/onboarding/dismiss` | Solo sesión iniciada | Completar u omitir la configuración inicial |
| GET | `/api/openapi.json` | Cualquier token válido | Obtener la descripción OpenAPI de esta API |

## Importación de configuración

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | Importar configuración de Lucky |

## Token de API

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| GET | `/api/tokens` | Solo sesión iniciada | Listar tokens de API |
| GET | `/api/tokens/scopes` | Solo sesión iniciada | Listar los dominios de permisos que se pueden conceder |
| POST | `/api/tokens` | Solo sesión iniciada | Crear un token de API |
| PUT | `/api/tokens/order` | Solo sesión iniciada | Reordenar tokens de API |
| PUT | `/api/tokens/group` | Solo sesión iniciada | Asignar el grupo de varios tokens de API |
| PUT | `/api/tokens/{id}` | Solo sesión iniciada | Modificar el nombre, grupo, permisos, caducidad y estado de un token de API |
| POST | `/api/tokens/{id}/regenerate` | Solo sesión iniciada | Regenerar el texto plano del token |
| DELETE | `/api/tokens/{id}` | Solo sesión iniciada | Eliminar un token de API |

## Cuenta

| Método | Ruta | Permiso requerido | Descripción |
| --- | --- | --- | --- |
| PUT | `/api/account` | Solo sesión iniciada | Modificar el usuario y la contraseña del administrador |
| POST | `/api/account/sessions/revoke` | Solo sesión iniciada | Cerrar sesión en todos los dispositivos |
| PUT | `/api/account/ui-language` | Solo sesión iniciada | Registrar el idioma de la consola de administración |
| GET | `/api/account/passkeys` | Solo sesión iniciada | Listar las Passkeys registradas |
| POST | `/api/account/passkeys/begin` | Solo sesión iniciada | Iniciar el registro de una Passkey |
| POST | `/api/account/passkeys` | Solo sesión iniciada | Finalizar el registro de una Passkey |
| DELETE | `/api/account/passkeys/{id}` | Solo sesión iniciada | Eliminar una Passkey |
| GET | `/api/account/users` | Solo sesión iniciada | Listar cuentas de solo lectura |
| POST | `/api/account/users` | Solo sesión iniciada | Crear una cuenta de solo lectura |
| PUT | `/api/account/users/{name}` | Solo sesión iniciada | Restablecer la contraseña de una cuenta de solo lectura |
| DELETE | `/api/account/users/{name}` | Solo sesión iniciada | Eliminar una cuenta de solo lectura |
| GET | `/api/account/totp` | Solo sesión iniciada | Ver el estado de la verificación en dos pasos |
| POST | `/api/account/totp/setup` | Solo sesión iniciada | Generar la clave de verificación en dos pasos |
| POST | `/api/account/totp/enable` | Solo sesión iniciada | Activar la verificación en dos pasos |
| POST | `/api/account/totp/disable` | Solo sesión iniciada | Desactivar la verificación en dos pasos |

## Estructuras de datos

### backup.LocalFile

| Campo | Tipo |
| --- | --- |
| `modTime` | Hora (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| Campo | Tipo |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| Campo | Tipo |
| --- | --- |
| `lastRun` | Hora (RFC 3339) |
| `manual` | boolean |
| `nextRun` | Hora (RFC 3339) |
| `results` | Lista de `backup.Result` |
| `running` | boolean |

### backup.Target

| Campo | Tipo |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| Campo | Tipo |
| --- | --- |
| `count` | integer |
| `created` | Hora (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Hora (RFC 3339) |

### cloudflared.Tunnel

| Campo | Tipo |
| --- | --- |
| `apiToken` | string |
| `edgeIpVersion` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `mode` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `service` | string |
| `token` | string |

### cron.Task

| Campo | Tipo |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `group` | string |
| `http` | `webhook.Config` |
| `key` | string |
| `name` | string |
| `remark` | string |
| `spec` | string |
| `type` | string |

### crud.batchDeleteRequest

| Campo | Tipo |
| --- | --- |
| `keys` | Lista de string |

### crud.batchEnableRequest

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `keys` | Lista de string |

### crud.enableRequest

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| Campo | Tipo |
| --- | --- |
| `group` | string |
| `keys` | Lista de string |

### crud.orderRequest

| Campo | Tipo |
| --- | --- |
| `keys` | Lista de string |

### ddns.Record

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `line` | string |
| `remark` | string |
| `sub` | string |
| `ttl` | integer |
| `type` | string |
| `value` | string |
| `zone` | string |

### ddns.RemoteDeleteResult

| Campo | Tipo |
| --- | --- |
| `deleted` | integer |
| `domain` | string |
| `error` | string |
| `key` | string |
| `type` | string |

### ddns.Task

| Campo | Tipo |
| --- | --- |
| `credentials` | `dnsprovider.Credentials` |
| `enable` | boolean |
| `forceInterval` | integer |
| `group` | string |
| `httpTimeout` | integer |
| `interval` | integer |
| `ipv4` | `ipsource.Source` |
| `ipv6` | `ipsource.Source` |
| `key` | string |
| `name` | string |
| `provider` | string |
| `records` | Lista de `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| Campo | Tipo |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| Campo | Tipo |
| --- | --- |
| `records` | Lista de `ddns.recordLayout` |

### ddns.remoteDeleteRequest

| Campo | Tipo |
| --- | --- |
| `records` | Lista de string |
| `task` | `ddns.Task` |

### dnsprovider.Credentials

| Campo | Tipo |
| --- | --- |
| `extra` | Mapa de string |
| `id` | string |
| `secret` | string |

### easytier.Instance

| Campo | Tipo |
| --- | --- |
| `devName` | string |
| `dhcp` | boolean |
| `enable` | boolean |
| `enableExitNode` | boolean |
| `env` | string |
| `exitNodes` | string |
| `extraArgs` | string |
| `group` | string |
| `hostname` | string |
| `ipv4` | string |
| `kcpProxy` | boolean |
| `key` | string |
| `latencyFirst` | boolean |
| `listeners` | string |
| `name` | string |
| `networkName` | string |
| `networkSecret` | string |
| `noTun` | boolean |
| `peers` | string |
| `portForwards` | string |
| `proxyNetworks` | string |
| `quicProxy` | boolean |
| `remark` | string |

### easytier.PeerView

| Campo | Tipo |
| --- | --- |
| `cost` | string |
| `direct` | boolean |
| `ip` | string |
| `latencyMs` | number |
| `loss` | string |
| `name` | string |
| `nat` | string |
| `rx` | string |
| `tunnel` | string |
| `tx` | string |
| `version` | string |

### easytier.SelfView

| Campo | Tipo |
| --- | --- |
| `ip` | string |
| `listeners` | Lista de string |
| `name` | string |
| `nat` | string |
| `publicIps` | Lista de string |

### easytier.Settings

| Campo | Tipo |
| --- | --- |
| `corePath` | string |
| `installProxy` | string |

### easytier.SettingsView

| Campo | Tipo |
| --- | --- |
| `core` | string |
| `settings` | `easytier.Settings` |

### easytier.StatusView

| Campo | Tipo |
| --- | --- |
| `cli` | string |
| `core` | string |
| `daemon` | `proc.State` |
| `enable` | boolean |
| `error` | string |
| `key` | string |
| `name` | string |
| `peers` | Lista de `easytier.PeerView` |
| `peersDirect` | integer |
| `self` | `easytier.SelfView` |
| `status` | string |
| `version` | string |

### events.TypeInfo

| Campo | Tipo |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| Campo | Tipo |
| --- | --- |
| `accessUrls` | Lista de string |
| `adminPassword` | string |
| `adminUser` | string |
| `authMethod` | string |
| `baseUrl` | string |
| `disableWebDav` | boolean |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenIp` | string |
| `locale` | string |
| `name` | string |
| `port` | integer |
| `proxyHeader` | string |
| `remark` | string |
| `sources` | Lista de `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| Campo | Tipo |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| Campo | Tipo |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| Campo | Tipo |
| --- | --- |
| `access` | `frp.ClientAccess` |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ClientAccess

| Campo | Tipo |
| --- | --- |
| `httpPort` | integer |
| `httpScheme` | string |
| `httpsPort` | integer |
| `subdomainHost` | string |

### frp.ClientSettings

| Campo | Tipo |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| Campo | Tipo |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| Campo | Tipo |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | Lista de `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | Lista de string |

### frp.Proxy

| Campo | Tipo |
| --- | --- |
| `client` | string |
| `customDomains` | Lista de string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `localIp` | string |
| `localPort` | integer |
| `name` | string |
| `remark` | string |
| `remotePort` | integer |
| `secretKey` | string |
| `subdomain` | string |
| `type` | string |

### frp.Server

| Campo | Tipo |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `enable` | boolean |
| `group` | string |
| `ipDenyList` | Lista de string |
| `ipDenySets` | Lista de string |
| `ipList` | Lista de string |
| `ipMode` | string |
| `ipSets` | Lista de string |
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| Campo | Tipo |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `ipDenyList` | Lista de string |
| `ipDenySets` | Lista de string |
| `ipList` | Lista de string |
| `ipMode` | string |
| `ipSets` | Lista de string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| Campo | Tipo |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| Campo | Tipo |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| Campo | Tipo |
| --- | --- |
| `buildTime` | Hora (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| Campo | Tipo |
| --- | --- |
| `count` | integer |
| `created` | Hora (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Hora (RFC 3339) |

### guard.FirewallStatus

| Campo | Tipo |
| --- | --- |
| `active` | boolean |
| `backend` | string |
| `checkedAt` | Hora (RFC 3339) |
| `dropped` | Mapa de integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | Mapa de integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | Lista de integer |
| `version` | string |

### guard.Jail

| Campo | Tipo |
| --- | --- |
| `anyStatus` | boolean |
| `banTime` | integer |
| `enable` | boolean |
| `filter` | string |
| `findTime` | integer |
| `group` | string |
| `ignore` | string |
| `key` | string |
| `maxRetry` | integer |
| `name` | string |
| `preset` | string |
| `remark` | string |
| `source` | string |
| `target` | string |
| `unknownHost` | boolean |

### guard.Platform

| Campo | Tipo |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| Campo | Tipo |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `ipv6Strict` | boolean |
| `threshold` | integer |

### guard.Settings

| Campo | Tipo |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | Lista de string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| Campo | Tipo |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | Lista de integer |

### guard.TestResult

| Campo | Tipo |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `kind` | string |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| Campo | Tipo |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| Campo | Tipo |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| Campo | Tipo |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| Campo | Tipo |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | Lista de string |

### knock.Counters

| Campo | Tipo |
| --- | --- |
| `accepted` | integer |
| `ignored` | integer |
| `rejected` | integer |

### knock.FirewallStatus

| Campo | Tipo |
| --- | --- |
| `active` | boolean |
| `allowed` | integer |
| `backend` | string |
| `checkedAt` | Hora (RFC 3339) |
| `dropped` | integer |
| `error` | string |
| `needed` | boolean |
| `noIPv6` | boolean |
| `ports` | Lista de string |
| `reason` | string |
| `version` | string |

### knock.GrantView

| Campo | Tipo |
| --- | --- |
| `addr` | string |
| `ports` | Lista de string |
| `rule` | string |
| `ruleName` | string |
| `until` | Hora (RFC 3339) |

### knock.Rule

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `encKey` | string |
| `firewall` | boolean |
| `group` | string |
| `hmacKey` | string |
| `hmacType` | string |
| `key` | string |
| `maxTimeout` | integer |
| `name` | string |
| `ports` | Lista de string |
| `remark` | string |
| `requireAddr` | boolean |
| `timeout` | integer |
| `user` | string |

### knock.Settings

| Campo | Tipo |
| --- | --- |
| `allowLan` | boolean |
| `allowList` | Lista de string |
| `autoFirewall` | boolean |
| `maxAge` | integer |
| `port` | integer |

### knock.Status

| Campo | Tipo |
| --- | --- |
| `counters` | `knock.Counters` |
| `firewall` | `knock.FirewallStatus` |
| `grants` | Lista de `knock.GrantView` |
| `listenError` | string |
| `listening` | boolean |
| `now` | Hora (RFC 3339) |
| `openStatus` | string |
| `os` | string |
| `port` | integer |

### knock.revokeRequest

| Campo | Tipo |
| --- | --- |
| `addr` | string |
| `rule` | string |

### netbird.ConnView

| Campo | Tipo |
| --- | --- |
| `connected` | boolean |
| `error` | string |
| `url` | string |

### netbird.Instance

| Campo | Tipo |
| --- | --- |
| `acceptDns` | boolean |
| `acceptRoutes` | boolean |
| `blockInbound` | boolean |
| `daemonAddr` | string |
| `daemonEnv` | string |
| `daemonExtraArgs` | string |
| `enable` | boolean |
| `extraUpArgs` | string |
| `group` | string |
| `hostname` | string |
| `key` | string |
| `legacyStateDir` | boolean |
| `localForwarding` | boolean |
| `manageDaemon` | boolean |
| `managementUrl` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `serverRoutes` | boolean |
| `setupKey` | string |
| `socks5` | string |
| `stateDir` | string |
| `userspace` | boolean |

### netbird.PeerView

| Campo | Tipo |
| --- | --- |
| `connType` | string |
| `fqdn` | string |
| `ips` | Lista de string |
| `lastSeen` | Hora (RFC 3339) |
| `latencyMs` | number |
| `name` | string |
| `online` | boolean |
| `relay` | string |
| `routes` | Lista de string |
| `rxBytes` | integer |
| `status` | string |
| `txBytes` | integer |

### netbird.SelfView

| Campo | Tipo |
| --- | --- |
| `fqdn` | string |
| `ips` | Lista de string |
| `kernel` | boolean |
| `name` | string |
| `wgPort` | integer |

### netbird.Settings

| Campo | Tipo |
| --- | --- |
| `installProxy` | string |
| `netbirdPath` | string |

### netbird.SettingsView

| Campo | Tipo |
| --- | --- |
| `cli` | string |
| `defaultManagementUrl` | string |
| `settings` | `netbird.Settings` |

### netbird.StatusView

| Campo | Tipo |
| --- | --- |
| `authCode` | string |
| `authUrl` | string |
| `cli` | string |
| `daemon` | `proc.State` |
| `daemonStatus` | string |
| `enable` | boolean |
| `error` | string |
| `key` | string |
| `management` | `netbird.ConnView` |
| `name` | string |
| `peers` | Lista de `netbird.PeerView` |
| `peersConnected` | integer |
| `relaysTotal` | integer |
| `relaysUp` | integer |
| `self` | `netbird.SelfView` |
| `signal` | `netbird.ConnView` |
| `version` | string |

### nettools.Request

| Campo | Tipo |
| --- | --- |
| `cidr` | string |
| `count` | integer |
| `ports` | string |
| `seconds` | integer |
| `server` | string |
| `target` | string |
| `tool` | string |
| `type` | string |
| `url` | string |

### nettools.Settings

| Campo | Tipo |
| --- | --- |
| `deny` | Lista de string |

### nettools.Task

| Campo | Tipo |
| --- | --- |
| `error` | string |
| `finished` | Hora (RFC 3339) |
| `id` | string |
| `lines` | Lista de string |
| `result` | Cualquiera |
| `started` | Hora (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `proxy` | string |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| Campo | Tipo |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | Hora (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| Campo | Tipo |
| --- | --- |
| `channels` | Lista de string |
| `enable` | boolean |
| `events` | Lista de string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### notify.Settings

| Campo | Tipo |
| --- | --- |
| `proxy` | string |
| `serverName` | string |

### notify.settingsView

| Campo | Tipo |
| --- | --- |
| `hostname` | string |
| `proxy` | string |
| `serverName` | string |

### perf.AlertState

| Campo | Tipo |
| --- | --- |
| `firedAt` | integer |
| `level` | string |
| `name` | string |
| `rule` | string |
| `since` | integer |
| `state` | string |
| `subject` | string |
| `target` | string |
| `value` | number |

### perf.MetricDef

| Campo | Tipo |
| --- | --- |
| `id` | string |
| `label` | string |
| `max` | number |
| `scope` | string |
| `unit` | string |
| `window` | boolean |

### perf.Overview

| Campo | Tipo |
| --- | --- |
| `latest` | `perf.Point` |
| `range` | string |
| `series` | Lista de `perf.Point` |
| `step` | integer |

### perf.Point

| Campo | Tipo |
| --- | --- |
| `childCpu` | number |
| `childCpuMax` | number |
| `childRss` | number |
| `cpu` | number |
| `cpuMax` | number |
| `disk` | number |
| `goroutines` | number |
| `load1` | number |
| `mem` | number |
| `procCpu` | number |
| `procCpuMax` | number |
| `procFds` | number |
| `procRss` | number |
| `t` | integer |

### perf.ProcSample

| Campo | Tipo |
| --- | --- |
| `cpu` | number |
| `rss` | number |
| `t` | integer |

### perf.ProcessView

| Campo | Tipo |
| --- | --- |
| `cpu` | number |
| `module` | string |
| `name` | string |
| `pid` | integer |
| `program` | string |
| `restarts` | integer |
| `rss` | integer |
| `samples` | Lista de `perf.ProcSample` |
| `since` | Hora (RFC 3339) |
| `threads` | integer |

### perf.Rule

| Campo | Tipo |
| --- | --- |
| `duration` | integer |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `level` | string |
| `metric` | string |
| `minRequests` | integer |
| `name` | string |
| `op` | string |
| `remark` | string |
| `sites` | Lista de string |
| `threshold` | number |

### perf.SiteView

| Campo | Tipo |
| --- | --- |
| `active` | integer |
| `err5xx` | number |
| `key` | string |
| `latencies` | integer |
| `name` | string |
| `p95` | number |
| `qps` | number |
| `requests` | integer |
| `upstreams` | Lista de `perf.UpstreamView` |

### perf.UpstreamView

| Campo | Tipo |
| --- | --- |
| `conns` | integer |
| `healthy` | boolean |
| `key` | string |
| `route` | string |
| `url` | string |

### portforward.Rule

| Campo | Tipo |
| --- | --- |
| `autoFirewall` | boolean |
| `enable` | boolean |
| `group` | string |
| `ipDenyList` | Lista de string |
| `ipDenySets` | Lista de string |
| `ipList` | Lista de string |
| `ipMode` | string |
| `ipSets` | Lista de string |
| `key` | string |
| `knock` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | Lista de string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | Lista de string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### proc.State

| Campo | Tipo |
| --- | --- |
| `error` | string |
| `lastExit` | string |
| `pid` | integer |
| `restarts` | integer |
| `running` | boolean |
| `since` | Hora (RFC 3339) |

### protoroute.Route

| Campo | Tipo |
| --- | --- |
| `alpn` | Lista de string |
| `disabled` | boolean |
| `domains` | Lista de string |
| `knock` | string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | Lista de string |
| `targetPort` | integer |

### protoroute.Rule

| Campo | Tipo |
| --- | --- |
| `autoFirewall` | boolean |
| `enable` | boolean |
| `group` | string |
| `ipDenyList` | Lista de string |
| `ipDenySets` | Lista de string |
| `ipList` | Lista de string |
| `ipMode` | string |
| `ipSets` | Lista de string |
| `key` | string |
| `knock` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | Lista de string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | Lista de `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | Lista de string |
| `targetPorts` | string |

### ssl.ACMEConfig

| Campo | Tipo |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `disableCname` | boolean |
| `domains` | Lista de string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | Lista de string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| Campo | Tipo |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | Lista de `ssl.Deploy` |
| `enable` | boolean |
| `group` | string |
| `issuedProfile` | string |
| `issuerPem` | string |
| `key` | string |
| `keyFile` | string |
| `keyPem` | string |
| `mapping` | `ssl.Mapping` |
| `remark` | string |
| `source` | string |
| `updatedAt` | Hora (RFC 3339) |

### ssl.Deploy

| Campo | Tipo |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | Lista de string |
| `deployedAt` | Hora (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| Campo | Tipo |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| Campo | Tipo |
| --- | --- |
| `domain` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `password` | string |
| `path` | string |
| `readOnly` | boolean |
| `remark` | string |
| `skipVerify` | boolean |
| `type` | string |
| `url` | string |
| `username` | string |

### storage.MountInfo

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| Campo | Tipo |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| Campo | Tipo |
| --- | --- |
| `autoFirewall` | boolean |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `ipDenyList` | Lista de string |
| `ipDenySets` | Lista de string |
| `ipList` | Lista de string |
| `ipMode` | string |
| `ipSets` | Lista de string |
| `keepAliveServers` | Lista de string |
| `key` | string |
| `knock` | string |
| `localPort` | integer |
| `maxConnections` | integer |
| `name` | string |
| `natpmp` | boolean |
| `natpmpGateway` | string |
| `protocol` | string |
| `remark` | string |
| `retryInterval` | integer |
| `script` | string |
| `scriptEnable` | boolean |
| `stunServers` | Lista de string |
| `stunTimeout` | integer |
| `targetAddresses` | Lista de string |
| `targetPort` | integer |
| `udpSessionTimeout` | integer |
| `upnp` | boolean |
| `upnpControlUrl` | string |
| `upnpGateway` | string |
| `upnpInternalPort` | integer |
| `upnpLocalHost` | string |
| `useGlobalServers` | boolean |
| `useGlobalWebhook` | boolean |
| `webhook` | `webhook.Config` |
| `webhookOnlyOnChange` | boolean |

### tailscale.Instance

| Campo | Tipo |
| --- | --- |
| `acceptDns` | boolean |
| `acceptRoutes` | boolean |
| `advertiseExitNode` | boolean |
| `advertiseRoutes` | Lista de string |
| `authKey` | string |
| `daemonEnv` | string |
| `daemonExtraArgs` | string |
| `enable` | boolean |
| `extraUpArgs` | string |
| `group` | string |
| `hostname` | string |
| `key` | string |
| `legacyStateDir` | boolean |
| `loginServer` | string |
| `manageDaemon` | boolean |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `socket` | string |
| `socks5` | string |
| `stateDir` | string |
| `userspace` | boolean |

### tailscale.PeerView

| Campo | Tipo |
| --- | --- |
| `active` | boolean |
| `curAddr` | string |
| `dnsName` | string |
| `exitNode` | boolean |
| `exitNodeOption` | boolean |
| `ips` | Lista de string |
| `lastSeen` | Hora (RFC 3339) |
| `name` | string |
| `online` | boolean |
| `os` | string |
| `relay` | string |
| `routes` | Lista de string |
| `rxBytes` | integer |
| `txBytes` | integer |

### tailscale.Settings

| Campo | Tipo |
| --- | --- |
| `installProxy` | string |
| `tailscalePath` | string |
| `tailscaledPath` | string |

### tailscale.SettingsView

| Campo | Tipo |
| --- | --- |
| `cli` | string |
| `settings` | `tailscale.Settings` |

### tailscale.StatusView

| Campo | Tipo |
| --- | --- |
| `authUrl` | string |
| `backendState` | string |
| `cli` | string |
| `daemon` | `proc.State` |
| `enable` | boolean |
| `error` | string |
| `health` | Lista de string |
| `key` | string |
| `magicDnsSuffix` | string |
| `name` | string |
| `peers` | Lista de `tailscale.PeerView` |
| `self` | `tailscale.PeerView` |
| `tailnet` | string |
| `version` | string |

### thirdauth.Provider

| Campo | Tipo |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| Campo | Tipo |
| --- | --- |
| `bodyLimitAction` | string |
| `bodyLimitMB` | integer |
| `enable` | boolean |
| `group` | string |
| `inboundThreshold` | integer |
| `key` | string |
| `mode` | string |
| `name` | string |
| `outboundThreshold` | integer |
| `paranoia` | integer |
| `remark` | string |
| `ruleSets` | Lista de `waf.RuleSet` |
| `safeline` | `waf.SafeLine` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### waf.SafeLine

| Campo | Tipo |
| --- | --- |
| `addr` | string |
| `bodyLimitKB` | integer |
| `enable` | boolean |
| `failClosed` | boolean |
| `timeoutMs` | integer |

### webhook.Config

| Campo | Tipo |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | Lista de string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `proxy` | string |
| `successContent` | Lista de string |
| `timeout` | integer |
| `url` | string |

### webpage.CheckReport

| Campo | Tipo |
| --- | --- |
| `pageTitle` | string |
| `placeholders` | Lista de string |
| `removed` | Lista de string |
| `size` | integer |
| `warnings` | Lista de string |

### webservice.AuthUser

| Campo | Tipo |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | Lista de string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BackendTimeouts

| Campo | Tipo |
| --- | --- |
| `connect` | integer |
| `responseHeader` | integer |
| `tlsHandshake` | integer |

### webservice.BanInfo

| Campo | Tipo |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | Hora (RFC 3339) |

### webservice.CacheConfig

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `rules` | Lista de `webservice.CacheRule` |
| `ttl` | integer |

### webservice.CacheRule

| Campo | Tipo |
| --- | --- |
| `noCache` | boolean |
| `override` | boolean |
| `path` | string |
| `ttl` | integer |

### webservice.CacheStatsView

| Campo | Tipo |
| --- | --- |
| `bytes` | integer |
| `enabled` | boolean |
| `entries` | integer |
| `maxBytes` | integer |

### webservice.ClientTimeouts

| Campo | Tipo |
| --- | --- |
| `readIdle` | integer |
| `writeIdle` | integer |

### webservice.ConnView

| Campo | Tipo |
| --- | --- |
| `down` | integer |
| `durationMs` | integer |
| `end` | Hora (RFC 3339) |
| `host` | string |
| `id` | string |
| `ip` | string |
| `method` | string |
| `path` | string |
| `peer` | string |
| `port` | integer |
| `proto` | string |
| `result` | string |
| `start` | Hora (RFC 3339) |
| `up` | integer |
| `websocket` | boolean |

### webservice.ConnsView

| Campo | Tipo |
| --- | --- |
| `active` | Lista de `webservice.ConnView` |
| `closed` | integer |
| `history` | Lista de `webservice.ConnView` |
| `matched` | integer |
| `total` | integer |

### webservice.ErrorPage

| Campo | Tipo |
| --- | --- |
| `file` | string |
| `html` | string |
| `key` | string |
| `name` | string |
| `remark` | string |
| `source` | string |
| `updatedAt` | Hora (RFC 3339) |

### webservice.ErrorPageCheckRequest

| Campo | Tipo |
| --- | --- |
| `code` | integer |
| `file` | string |
| `html` | string |
| `source` | string |

### webservice.ErrorPageCheckResult

| Campo | Tipo |
| --- | --- |
| `preview` | string |
| `report` | `webpage.CheckReport` |

### webservice.ErrorPageOptions

| Campo | Tipo |
| --- | --- |
| `codes` | string |
| `intercept` | boolean |
| `page` | string |
| `style` | string |

### webservice.ErrorPageSaved

| Campo | Tipo |
| --- | --- |
| `page` | `webservice.ErrorPageView` |
| `report` | `webpage.CheckReport` |

### webservice.ErrorPageStyle

| Campo | Tipo |
| --- | --- |
| `page` | string |
| `style` | string |

### webservice.ErrorPageView

| Campo | Tipo |
| --- | --- |
| `error` | string |
| `file` | string |
| `key` | string |
| `name` | string |
| `remark` | string |
| `size` | integer |
| `source` | string |
| `updatedAt` | Hora (RFC 3339) |
| `usedBy` | Lista de string |

### webservice.ErrorStylePreview

| Campo | Tipo |
| --- | --- |
| `html` | string |
| `id` | string |

### webservice.ForwardAuth

| Campo | Tipo |
| --- | --- |
| `copyHeaders` | Lista de string |
| `loginFromLocation` | boolean |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| Campo | Tipo |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | Hora (RFC 3339) |
| `url` | string |

### webservice.IdentityHeader

| Campo | Tipo |
| --- | --- |
| `field` | string |
| `header` | string |

### webservice.Listener

| Campo | Tipo |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `accessPort` | integer |
| `accessScheme` | string |
| `autoFirewall` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `httpPort` | integer |
| `httpPortMode` | string |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | Lista de `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| Campo | Tipo |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | Lista de string |
| `stripPrefix` | boolean |
| `targets` | Lista de string |

### webservice.OIDCClient

| Campo | Tipo |
| --- | --- |
| `allow` | Lista de string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | Lista de string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| Campo | Tipo |
| --- | --- |
| `clients` | Lista de `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.Panel

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `icon` | string |
| `key` | string |
| `lanCidrs` | Lista de string |
| `links` | Lista de `webservice.PanelLink` |
| `name` | string |
| `remark` | string |
| `skin` | string |
| `subtitle` | string |
| `theme` | string |
| `title` | string |

### webservice.PanelAlt

| Campo | Tipo |
| --- | --- |
| `name` | string |
| `url` | string |

### webservice.PanelIconView

| Campo | Tipo |
| --- | --- |
| `cat` | string |
| `color` | string |
| `id` | string |
| `name` | string |
| `src` | string |
| `srcDark` | string |

### webservice.PanelLink

| Campo | Tipo |
| --- | --- |
| `alts` | Lista de `webservice.PanelAlt` |
| `color` | string |
| `description` | string |
| `group` | string |
| `hide` | boolean |
| `icon` | string |
| `lanUrl` | string |
| `name` | string |
| `sameTab` | boolean |
| `url` | string |

### webservice.PanelPreview

| Campo | Tipo |
| --- | --- |
| `html` | string |

### webservice.PanelPreviewRequest

| Campo | Tipo |
| --- | --- |
| `lan` | boolean |
| `panel` | `webservice.Panel` |

### webservice.PathRule

| Campo | Tipo |
| --- | --- |
| `auth` | string |
| `ipList` | Lista de string |
| `ipMode` | string |
| `ipSets` | Lista de string |
| `prefix` | string |

### webservice.Point

| Campo | Tipo |
| --- | --- |
| `bytes` | integer |
| `p50` | number |
| `p95` | number |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.Rate

| Campo | Tipo |
| --- | --- |
| `bps` | number |
| `qps` | number |
| `qps60` | number |

### webservice.RateLimit

| Campo | Tipo |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.RatePoint

| Campo | Tipo |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `t` | integer |

### webservice.ReplayBucket

| Campo | Tipo |
| --- | --- |
| `active` | integer |
| `bps` | number |
| `ips` | integer |
| `issue` | boolean |
| `qps` | number |
| `t` | integer |

### webservice.ReplayIndex

| Campo | Tipo |
| --- | --- |
| `bucket` | integer |
| `buckets` | Lista de `webservice.ReplayBucket` |
| `end` | integer |
| `frames` | integer |
| `from` | integer |
| `limit` | integer |
| `samples` | integer |
| `settings` | `webservice.ReplaySettings` |
| `start` | integer |
| `step` | integer |
| `to` | integer |
| `used` | integer |

### webservice.ReplayMeta

| Campo | Tipo |
| --- | --- |
| `end` | integer |
| `frameEnd` | integer |
| `frameStart` | integer |
| `next` | integer |
| `prev` | integer |
| `start` | integer |
| `step` | integer |

### webservice.ReplaySettings

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `sizeMB` | integer |

### webservice.Rewrite

| Campo | Tipo |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| Campo | Tipo |
| --- | --- |
| `authMarkerKey` | string |
| `authSecret` | string |
| `authUsers` | Lista de `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | Lista de `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `realIPFrom` | Lista de string |
| `realIPHeaders` | Lista de string |
| `realPortHeader` | string |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | Lista de string |

### webservice.ShareLink

| Campo | Tipo |
| --- | --- |
| `expiresAt` | Hora (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| Campo | Tipo |
| --- | --- |
| `accessLog` | boolean |
| `adminPass` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `clientTimeouts` | `webservice.ClientTimeouts` |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | Lista de string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | Lista de string |
| `enable` | boolean |
| `errorPage` | `webservice.ErrorPageOptions` |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `identityHeaders` | Lista de `webservice.IdentityHeader` |
| `insecureSkipVerify` | boolean |
| `ipDenyList` | Lista de string |
| `ipDenySets` | Lista de string |
| `ipList` | Lista de string |
| `ipMode` | string |
| `ipSets` | Lista de string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | Lista de `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | Lista de string |
| `panel` | string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | Lista de string |
| `responseHeaders` | Lista de string |
| `rewrites` | Lista de `webservice.Rewrite` |
| `root` | string |
| `rules` | Lista de `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | Lista de `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | Lista de string |
| `text` | string |
| `timeouts` | `webservice.BackendTimeouts` |
| `trustAuthUsers` | boolean |
| `trustBackendAuth` | boolean |
| `type` | string |
| `upstreamH2c` | boolean |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | Lista de `webservice.UserRoute` |
| `users` | Lista de string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteBatchOptions

| Campo | Tipo |
| --- | --- |
| `accessLog` | boolean |
| `compress` | boolean |
| `cors` | boolean |
| `denyClose` | boolean |
| `errorPage` | `webservice.ErrorPageStyle` |
| `forwardedHeaders` | boolean |
| `waf` | string |

### webservice.SiteBatchRequest

| Campo | Tipo |
| --- | --- |
| `action` | string |
| `domains` | Lista de string |
| `from` | string |
| `move` | `webservice.SiteMoveTarget` |
| `options` | `webservice.SiteBatchOptions` |
| `sites` | Lista de `webservice.SiteRef` |
| `to` | string |

### webservice.SiteBatchResult

| Campo | Tipo |
| --- | --- |
| `changed` | integer |
| `moved` | Lista de `webservice.SiteMoved` |
| `unchanged` | integer |

### webservice.SiteMoveTarget

| Campo | Tipo |
| --- | --- |
| `after` | string |
| `before` | string |
| `group` | string |
| `listener` | string |

### webservice.SiteMoved

| Campo | Tipo |
| --- | --- |
| `from` | `webservice.SiteRef` |
| `to` | `webservice.SiteRef` |

### webservice.SiteRef

| Campo | Tipo |
| --- | --- |
| `listener` | string |
| `site` | string |

### webservice.SiteSharedUsers

| Campo | Tipo |
| --- | --- |
| `disableAuth` | boolean |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| Campo | Tipo |
| --- | --- |
| `blocked` | Mapa de integer |
| `bytesOut` | integer |
| `series` | Lista de `webservice.Point` |
| `status` | Mapa de integer |
| `topCountries` | Lista de `webservice.TopItem` |
| `topIps` | Lista de `webservice.TopItem` |
| `upstreams` | Lista de `webservice.UpstreamStatus` |

### webservice.TopItem

| Campo | Tipo |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.TopoAction

| Campo | Tipo |
| --- | --- |
| `id` | string |
| `kind` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |

### webservice.TopoIP

| Campo | Tipo |
| --- | --- |
| `active` | integer |
| `activeAt` | Mapa de integer |
| `ip` | string |
| `last` | integer |
| `nodes` | Lista de string |

### webservice.TopoListener

| Campo | Tipo |
| --- | --- |
| `actions` | Lista de `webservice.TopoAction` |
| `active` | integer |
| `enable` | boolean |
| `error` | string |
| `http3` | boolean |
| `httpPort` | integer |
| `httpPortMode` | string |
| `id` | string |
| `key` | string |
| `listenIp` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `proxyProtocol` | boolean |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `running` | boolean |
| `sites` | Lista de `webservice.TopoSite` |
| `tls` | boolean |

### webservice.TopoSite

| Campo | Tipo |
| --- | --- |
| `accessLog` | boolean |
| `active` | integer |
| `blocked` | Mapa de integer |
| `closed` | integer |
| `default` | boolean |
| `domains` | Lista de string |
| `enable` | boolean |
| `errors` | integer |
| `group` | string |
| `guards` | Lista de string |
| `id` | string |
| `key` | string |
| `name` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `running` | boolean |
| `status` | Mapa de integer |
| `target` | string |
| `type` | string |
| `upstreams` | Lista de `webservice.TopoUpstream` |

### webservice.TopoUpstream

| Campo | Tipo |
| --- | --- |
| `backup` | boolean |
| `bytes` | integer |
| `conns` | integer |
| `healthy` | boolean |
| `id` | string |
| `match` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `scope` | string |
| `url` | string |
| `weight` | integer |

### webservice.TopologyView

| Campo | Tipo |
| --- | --- |
| `active` | integer |
| `enabled` | boolean |
| `focus` | string |
| `ipTotal` | integer |
| `ipTruncated` | boolean |
| `ips` | Lista de `webservice.TopoIP` |
| `listeners` | Lista de `webservice.TopoListener` |
| `rate` | `webservice.Rate` |
| `replay` | `webservice.ReplayMeta` |
| `requests` | integer |
| `series` | Lista de `webservice.RatePoint` |
| `seriesStep` | integer |
| `time` | integer |
| `window` | integer |

### webservice.UpstreamStatus

| Campo | Tipo |
| --- | --- |
| `backup` | boolean |
| `bytes` | integer |
| `conns` | integer |
| `healthy` | boolean |
| `match` | string |
| `requests` | integer |
| `scope` | string |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| Campo | Tipo |
| --- | --- |
| `targets` | Lista de string |
| `users` | Lista de string |

### webservice.UserSite

| Campo | Tipo |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | Lista de string |
| `enable` | boolean |
| `forwardAuth` | boolean |
| `group` | string |
| `listener` | string |
| `listenerEnable` | boolean |
| `listenerName` | string |
| `name` | string |
| `otherAuth` | boolean |
| `port` | integer |
| `sharedUsers` | boolean |
| `site` | string |
| `tls` | boolean |
| `type` | string |
| `webAuth` | boolean |

### webservice.UsersRequest

| Campo | Tipo |
| --- | --- |
| `authUsers` | Lista de `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Lista de `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| Campo | Tipo |
| --- | --- |
| `authUsers` | Lista de `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Lista de `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| Campo | Tipo |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.cachePurgeRequest

| Campo | Tipo |
| --- | --- |
| `listener` | string |
| `path` | string |
| `site` | string |

### webservice.cachePurgeResult

| Campo | Tipo |
| --- | --- |
| `bytes` | integer |
| `enabled` | boolean |
| `entries` | integer |
| `maxBytes` | integer |
| `removed` | integer |

### webservice.closeConnRequest

| Campo | Tipo |
| --- | --- |
| `banSeconds` | integer |
| `id` | string |
| `listener` | string |
| `site` | string |

### webservice.closeConnResult

| Campo | Tipo |
| --- | --- |
| `banKey` | string |
| `closed` | integer |

### webservice.geoipDownload

| Campo | Tipo |
| --- | --- |
| `proxy` | string |
| `url` | string |

### webservice.totpRequest

| Campo | Tipo |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| Campo | Tipo |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | Lista de string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| Campo | Tipo |
| --- | --- |
| `authType` | string |
| `enable` | boolean |
| `key` | string |
| `keyFile` | string |
| `name` | string |
| `passphrase` | string |
| `password` | string |
| `privateKey` | string |
| `remark` | string |
| `username` | string |

### webterminal.LocalConfig

| Campo | Tipo |
| --- | --- |
| `env` | Mapa de string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| Campo | Tipo |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| Campo | Tipo |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
| `hostKeyType` | string |
| `keepAlive` | integer |
| `keyFile` | string |
| `passphrase` | string |
| `password` | string |
| `port` | integer |
| `privateKey` | string |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |
| `workDir` | string |

### webterminal.TelnetConfig

| Campo | Tipo |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.AgentInfo

| Campo | Tipo |
| --- | --- |
| `addr` | string |
| `host` | string |
| `lastSeen` | integer |
| `online` | boolean |
| `os` | string |
| `since` | integer |
| `version` | string |

### wol.BemfaSettings

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `uid` | string |

### wol.BemfaStatus

| Campo | Tipo |
| --- | --- |
| `connected` | boolean |
| `error` | string |
| `since` | integer |
| `topics` | integer |

### wol.Device

| Campo | Tipo |
| --- | --- |
| `agentToken` | string |
| `bemfaTopic` | string |
| `broadcastIps` | Lista de string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | Lista de string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |
| `shutdownCommand` | string |
| `shutdownConn` | string |
| `shutdownHttp` | `wol.HTTPAction` |
| `shutdownMode` | string |

### wol.HTTPAction

| Campo | Tipo |
| --- | --- |
| `body` | string |
| `headers` | Lista de string |
| `insecure` | boolean |
| `method` | string |
| `url` | string |

### wol.SSHTarget

| Campo | Tipo |
| --- | --- |
| `host` | string |
| `key` | string |
| `name` | string |
| `trusted` | boolean |

### wol.bemfaView

| Campo | Tipo |
| --- | --- |
| `settings` | `wol.BemfaSettings` |
| `status` | `wol.BemfaStatus` |

### wol.powerRequest

| Campo | Tipo |
| --- | --- |
| `action` | string |

### wol.shutdownResult

| Campo | Tipo |
| --- | --- |
| `output` | string |

