# API управления Gately

API управления Gately предназначен для скриптов и массового управления в автоматизации.

Аутентификация: создайте токен на странице «API-токены» панели управления и передавайте в заголовке запроса `Authorization: Bearer gtl_xxxx`.

Права: каждый токен имеет только те права, которые отмечены при создании, в формате `<модуль>:read` или `<модуль>:write`, право на запись включает чтение. Интерфейсы учётных записей, двухфакторной аутентификации, управления токенами, а также резервного копирования, восстановления, перезапуска и системных настроек доступны только в сессии входа.

В теле запроса используется `Content-Type: application/json`, ошибки всегда возвращаются в виде `{"error": "описание"}`.

Модули, отключённые в разделе «Настройки → Модули», не предоставляют API: их эндпоинты возвращают 403 и больше не отображаются в документации API работающего экземпляра.

Сайт: https://gately.cn

Машиночитаемое описание находится в `openapi.json` (OpenAPI 3.0) в том же каталоге.

## Быстрый старт

```bash
# Просмотр состояния системы (требуется status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# Список всех задач DDNS (требуется ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Немедленно синхронизировать задачу DDNS (требуется ddns:write, key берётся из списка, полученного на предыдущем шаге)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## Права

| Область прав | Название | Доступные права |
| --- | --- | --- |
| `portforward` | Проброс портов | `portforward:read`、`portforward:write` |
| `protoroute` | Маршрутизация по протоколу | `protoroute:read`、`protoroute:write` |
| `ddns` | Динамический DNS | `ddns:read`、`ddns:write` |
| `stun` | Проброс STUN | `stun:read`、`stun:write` |
| `tailscale` | Сеть Tailscale | `tailscale:read`、`tailscale:write` |
| `frp` | Проброс внутренней сети frp | `frp:read`、`frp:write` |
| `cloudflared` | Туннель Cloudflare | `cloudflared:read`、`cloudflared:write` |
| `ssl` | SSL-сертификат | `ssl:read`、`ssl:write` |
| `waf` | Брандмауэр WAF | `waf:read`、`waf:write` |
| `webservice` | Веб-служба | `webservice:read`、`webservice:write` |
| `guard` | Защита от вторжений | `guard:read`、`guard:write` |
| `wol` | Wake-on-LAN | `wol:read`、`wol:write` |
| `webterminal` | Веб-терминал | `webterminal:read`、`webterminal:write` |
| `cron` | Запланированная задача | `cron:read`、`cron:write` |
| `docker` | Управление Docker | `docker:read`、`docker:write` |
| `webdav` | WebDAV-сервер | `webdav:read`、`webdav:write` |
| `ftp` | Служба FTP | `ftp:read`、`ftp:write` |
| `smb` | Служба SMB | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | Монтирование хранилищ | `storage:read`、`storage:write` |
| `notify` | Центр уведомлений | `notify:read`、`notify:write` |
| `nettools` | Диагностика сети | `nettools:read`、`nettools:write` |
| `backup` | Резервная копия конфигурации | `backup:read`、`backup:write` |
| `status` | Состояние системы | `status:read` |
| `logs` | Системный журнал | `logs:read` |
| `metrics` | Метрики (Prometheus) | `metrics:read` |
| `import` | Импорт конфигурации Lucky | `import:write` |
| `fileusers` | Общие пользователи файловых служб | `fileusers:read`、`fileusers:write` |

Шаблоны: `*:read` означает доступ только на чтение ко всем модулям, `*:write` или `*` означает чтение и запись для всех модулей.

## Проброс портов

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | Просмотр журнала работы модуля |
| DELETE | `/api/portforward/logs` | `portforward:write` | Очистить журнал работы модуля |
| GET | `/api/portforward/rules` | `portforward:read` | Список: Правило проброса портов |
| POST | `/api/portforward/rules` | `portforward:write` | Добавить: Правило проброса портов |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | Изменить: Правило проброса портов |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | Удалить: Правило проброса портов |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | Включить или отключить: Правило проброса портов |
| PUT | `/api/portforward/rules-order` | `portforward:write` | Изменить порядок: Правило проброса портов |
| PUT | `/api/portforward/rules-group` | `portforward:write` | Задать группу сразу для нескольких: Правило проброса портов |
| GET | `/api/portforward/settings` | `portforward:read` | Чтение глобальных настроек проброса портов |
| PUT | `/api/portforward/settings` | `portforward:write` | Сохранить глобальные настройки проброса портов |

## Маршрутизация по протоколу

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | Просмотр журнала работы модуля |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | Очистить журнал работы модуля |
| GET | `/api/protoroute/rules` | `protoroute:read` | Список: правило маршрутизации по протоколу |
| POST | `/api/protoroute/rules` | `protoroute:write` | Добавить: правило маршрутизации по протоколу |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | Изменить: правило маршрутизации по протоколу |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | Удалить: правило маршрутизации по протоколу |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | Включить или отключить: правило маршрутизации по протоколу |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | Изменить порядок: правило маршрутизации по протоколу |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | Задать группу сразу для нескольких: правило маршрутизации по протоколу |
| GET | `/api/protoroute/settings` | `protoroute:read` | Получить настройки маршрутизации по протоколу |
| PUT | `/api/protoroute/settings` | `protoroute:write` | Сохранить настройки маршрутизации по протоколу |

## Динамический DNS

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | Просмотр журнала работы модуля |
| DELETE | `/api/ddns/logs` | `ddns:write` | Очистить журнал работы модуля |
| GET | `/api/ddns/tasks` | `ddns:read` | Список: Задача DDNS |
| POST | `/api/ddns/tasks` | `ddns:write` | Добавить: Задача DDNS |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | Изменить: Задача DDNS |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | Удалить: Задача DDNS |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | Включить или отключить: Задача DDNS |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | Изменить порядок: Задача DDNS |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | Задать группу сразу для нескольких: Задача DDNS |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | Показать секреты открытым текстом |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | Немедленно синхронизировать задачу DDNS |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | Удалить одну запись из задачи DDNS |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | Изменить порядок и группы DNS-записей задачи |
| GET | `/api/ddns/providers` | `ddns:read` | Список поддерживаемых DNS-провайдеров и необходимых им параметров |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | Список сетевых адаптеров и адресов этой машины |
| POST | `/api/ddns/iptest` | `ddns:write` | Проверить получение публичного IP |
| GET | `/api/ddns/defaults` | `ddns:read` | Чтение значений DDNS по умолчанию |
| GET | `/api/ddns/settings` | `ddns:read` | Чтение глобальных настроек DDNS |
| PUT | `/api/ddns/settings` | `ddns:write` | Сохранить глобальные настройки DDNS |

## Проброс STUN

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | Просмотр журнала работы модуля |
| DELETE | `/api/stun/logs` | `stun:write` | Очистить журнал работы модуля |
| GET | `/api/stun/rules` | `stun:read` | Список: Правило STUN |
| POST | `/api/stun/rules` | `stun:write` | Добавить: Правило STUN |
| PUT | `/api/stun/rules/{key}` | `stun:write` | Изменить: Правило STUN |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | Удалить: Правило STUN |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | Включить или отключить: Правило STUN |
| PUT | `/api/stun/rules-order` | `stun:write` | Изменить порядок: Правило STUN |
| PUT | `/api/stun/rules-group` | `stun:write` | Задать группу сразу для нескольких: Правило STUN |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | Перезапустить правило STUN |
| GET | `/api/stun/netinterfaces` | `stun:read` | Список сетевых адаптеров и адресов этой машины |
| GET | `/api/stun/settings` | `stun:read` | Чтение глобальных настроек STUN |
| PUT | `/api/stun/settings` | `stun:write` | Сохранить глобальные настройки STUN |

## Сеть Tailscale

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | Просмотр журнала работы модуля |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | Очистить журнал работы модуля |
| GET | `/api/tailscale/settings` | `tailscale:read` | Чтение настроек Tailscale |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Сохранить настройки Tailscale |
| GET | `/api/tailscale/secrets` | `tailscale:write` | Показать секреты открытым текстом |
| GET | `/api/tailscale/status` | `tailscale:read` | Просмотр состояния подключения Tailscale |
| POST | `/api/tailscale/restart` | `tailscale:write` | Перезапустить Tailscale |
| POST | `/api/tailscale/logout` | `tailscale:write` | Выйти из Tailscale |
| GET | `/api/tailscale/install` | `tailscale:read` | Просмотр сведений об установке и хода установки управляемой программы |
| POST | `/api/tailscale/install` | `tailscale:write` | Установить или обновить управляемую программу онлайн |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | Переключиться на сохранённую предыдущую версию |
| DELETE | `/api/tailscale/install` | `tailscale:write` | Удалить управляемую программу |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | Удалить сохранённую предыдущую версию |
| POST | `/api/tailscale/reset` | `tailscale:write` | Сбросить настройки модуля по умолчанию |

## Проброс внутренней сети frp

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | Просмотр журнала работы модуля |
| DELETE | `/api/frp/logs` | `frp:write` | Очистить журнал работы модуля |
| GET | `/api/frp/clients` | `frp:read` | Список: Клиент frp |
| POST | `/api/frp/clients` | `frp:write` | Добавить: Клиент frp |
| PUT | `/api/frp/clients/{key}` | `frp:write` | Изменить: Клиент frp |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | Удалить: Клиент frp |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | Включить или отключить: Клиент frp |
| PUT | `/api/frp/clients-order` | `frp:write` | Изменить порядок: Клиент frp |
| PUT | `/api/frp/clients-group` | `frp:write` | Задать группу сразу для нескольких: Клиент frp |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | Показать секреты открытым текстом |
| GET | `/api/frp/servers` | `frp:read` | Список: Сервер frp |
| POST | `/api/frp/servers` | `frp:write` | Добавить: Сервер frp |
| PUT | `/api/frp/servers/{key}` | `frp:write` | Изменить: Сервер frp |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | Удалить: Сервер frp |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | Включить или отключить: Сервер frp |
| PUT | `/api/frp/servers-order` | `frp:write` | Изменить порядок: Сервер frp |
| PUT | `/api/frp/servers-group` | `frp:write` | Задать группу сразу для нескольких: Сервер frp |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | Показать секреты открытым текстом |
| GET | `/api/frp/proxies` | `frp:read` | Список: прокси frp |
| POST | `/api/frp/proxies` | `frp:write` | Добавить: прокси frp |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | Изменить: прокси frp |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | Удалить: прокси frp |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | Включить или отключить: прокси frp |
| PUT | `/api/frp/proxies-order` | `frp:write` | Изменить порядок: прокси frp |
| PUT | `/api/frp/proxies-group` | `frp:write` | Задать группу сразу для нескольких: прокси frp |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | Показать секреты открытым текстом |
| POST | `/api/frp/import` | `frp:write` | Импорт существующей конфигурации frpc / frps (поддерживаются старый ini и новый toml); если apply равен false, выполняется только предпросмотр |
| GET | `/api/frp/install` | `frp:read` | Просмотр сведений об установке и хода установки управляемой программы |
| POST | `/api/frp/install` | `frp:write` | Установить или обновить управляемую программу онлайн |
| POST | `/api/frp/install/rollback` | `frp:write` | Переключиться на сохранённую предыдущую версию |
| DELETE | `/api/frp/install` | `frp:write` | Удалить управляемую программу |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | Удалить сохранённую предыдущую версию |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | Перезапустить экземпляр frpc |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | Перезапустить экземпляр frps |
| GET | `/api/frp/settings` | `frp:read` | Получить пути и версии программ |
| PUT | `/api/frp/settings` | `frp:write` | Сохранить пути к программам |
| POST | `/api/frp/reset` | `frp:write` | Сбросить настройки модуля по умолчанию |

## Туннель Cloudflare

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | Просмотр журнала работы модуля |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | Очистить журнал работы модуля |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | Список: Туннель Cloudflare |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Добавить: Туннель Cloudflare |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Изменить: Туннель Cloudflare |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Удалить: Туннель Cloudflare |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Включить или отключить: Туннель Cloudflare |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Изменить порядок: Туннель Cloudflare |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Задать группу сразу для нескольких: Туннель Cloudflare |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | Показать секреты открытым текстом |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Перезапустить туннель Cloudflare |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | Прочитать маршруты туннеля с токеном |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | Сохранить маршруты туннеля с токеном |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Чтение настроек туннеля Cloudflare |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Сохранить настройки туннеля Cloudflare |
| GET | `/api/cloudflared/install` | `cloudflared:read` | Просмотр сведений об установке и хода установки управляемой программы |
| POST | `/api/cloudflared/install` | `cloudflared:write` | Установить или обновить управляемую программу онлайн |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | Переключиться на сохранённую предыдущую версию |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | Удалить управляемую программу |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | Удалить сохранённую предыдущую версию |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | Сбросить настройки модуля по умолчанию |

## SSL-сертификат

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | Просмотр журнала работы модуля |
| DELETE | `/api/ssl/logs` | `ssl:write` | Очистить журнал работы модуля |
| GET | `/api/ssl/certs` | `ssl:read` | Список: Сертификат |
| POST | `/api/ssl/certs` | `ssl:write` | Добавить: Сертификат |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | Изменить: Сертификат |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | Удалить: Сертификат |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | Включить или отключить: Сертификат |
| PUT | `/api/ssl/certs-order` | `ssl:write` | Изменить порядок: Сертификат |
| PUT | `/api/ssl/certs-group` | `ssl:write` | Задать группу сразу для нескольких: Сертификат |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | Показать секреты открытым текстом |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | Немедленно выпустить или продлить сертификат |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | Скачать сертификат и закрытый ключ |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | Развернуть сертификат сейчас |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | Список доступных целей развёртывания |
| GET | `/api/ssl/providers` | `ssl:read` | Список поддерживаемых провайдеров DNS-проверки |
| GET | `/api/ssl/settings` | `ssl:read` | Чтение глобальных настроек сертификатов |
| PUT | `/api/ssl/settings` | `ssl:write` | Сохранить глобальные настройки сертификатов |

## Брандмауэр WAF

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | Просмотр журнала работы модуля |
| DELETE | `/api/waf/logs` | `waf:write` | Очистить журнал работы модуля |
| GET | `/api/waf/instances` | `waf:read` | Список: Экземпляр WAF |
| POST | `/api/waf/instances` | `waf:write` | Добавить: Экземпляр WAF |
| PUT | `/api/waf/instances/{key}` | `waf:write` | Изменить: Экземпляр WAF |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | Удалить: Экземпляр WAF |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | Включить или отключить: Экземпляр WAF |
| PUT | `/api/waf/instances-order` | `waf:write` | Изменить порядок: Экземпляр WAF |
| PUT | `/api/waf/instances-group` | `waf:write` | Задать группу сразу для нескольких: Экземпляр WAF |
| GET | `/api/waf/crs` | `waf:read` | Список файлов правил OWASP CRS |
| GET | `/api/waf/crs/{file}` | `waf:read` | Прочитать один файл правил CRS |

## Веб-служба

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | Просмотр журнала работы модуля |
| DELETE | `/api/webservice/logs` | `webservice:write` | Очистить журнал работы модуля |
| GET | `/api/webservice/listeners` | `webservice:read` | Список: Прослушивание веб-службы |
| POST | `/api/webservice/listeners` | `webservice:write` | Добавить: Прослушивание веб-службы |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Изменить: Прослушивание веб-службы |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Удалить: Прослушивание веб-службы |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Включить или отключить: Прослушивание веб-службы |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Изменить порядок: Прослушивание веб-службы |
| GET | `/api/webservice/iplists` | `webservice:read` | Список подписных списков IP |
| POST | `/api/webservice/iplists` | `webservice:write` | Добавить: Подписной список IP |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | Изменить: Подписной список IP |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | Удалить: Подписной список IP |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | Включить или отключить: Подписной список IP |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | Изменить порядок: Подписной список IP |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | Обновить подписной список IP сейчас |
| GET | `/api/webservice/users` | `webservice:read` | Получить общих пользователей и поставщика удостоверений OIDC |
| PUT | `/api/webservice/users` | `webservice:write` | Сохранить общих пользователей и поставщика удостоверений OIDC, массово настроить сайты |
| POST | `/api/webservice/import/nginx` | `webservice:write` | Импорт конфигурации nginx |
| GET | `/api/webservice/settings` | `webservice:read` | Чтение глобальных настроек веб-службы |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | Показать секреты TOTP общих пользователей открытым текстом (по имени пользователя) |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | Показать Client Secret сторонних входов открытым текстом (по идентификатору способа входа) |
| POST | `/api/webservice/totp-secret` | `webservice:write` | Создать новый ключ TOTP для общего пользователя (не сохраняется; действует после сохранения настроек) |
| PUT | `/api/webservice/settings` | `webservice:write` | Сохранить глобальные настройки веб-службы |
| GET | `/api/webservice/stats` | `webservice:read` | Поминутная статистика сайта (?listener=&site=&minutes=) |
| GET | `/api/webservice/metrics` | `webservice:read` | Метрики в текстовом формате Prometheus |
| GET | `/api/webservice/bans` | `webservice:read` | Заблокированные в данный момент IP |
| POST | `/api/webservice/bans` | `webservice:write` | Заблокировать IP вручную |
| DELETE | `/api/webservice/bans` | `webservice:write` | Снять блокировку (body: {"key": адрес}) |
| GET | `/api/webservice/geoip` | `webservice:read` | Состояние базы данных GeoIP |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | Загрузить базу данных GeoIP (поле multipart file, .mmdb) |
| POST | `/api/webservice/geoip/download` | `webservice:write` | Скачать базу данных GeoIP (.mmdb) по адресу |
| DELETE | `/api/webservice/geoip` | `webservice:write` | Удалить базу данных GeoIP |

## Защита от вторжений

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | Просмотр журнала работы модуля |
| DELETE | `/api/guard/logs` | `guard:write` | Очистить журнал работы модуля |
| GET | `/api/guard/jails` | `guard:read` | Список: правило защиты от вторжений |
| POST | `/api/guard/jails` | `guard:write` | Добавить: правило защиты от вторжений |
| PUT | `/api/guard/jails/{key}` | `guard:write` | Изменить: правило защиты от вторжений |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | Удалить: правило защиты от вторжений |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | Включить или отключить: правило защиты от вторжений |
| PUT | `/api/guard/jails-order` | `guard:write` | Изменить порядок: правило защиты от вторжений |
| PUT | `/api/guard/jails-group` | `guard:write` | Задать группу сразу для нескольких: правило защиты от вторжений |
| GET | `/api/guard/presets` | `guard:read` | Шаблоны правил для распространённых сервисов |
| POST | `/api/guard/jails/test` | `guard:write` | Проверить фильтры на примерах строк журнала |
| GET | `/api/guard/status` | `guard:read` | Состояние синхронизации с брандмауэром и окружение |
| GET | `/api/guard/settings` | `guard:read` | Глобальные настройки |
| PUT | `/api/guard/settings` | `guard:write` | Изменить общие настройки |
| GET | `/api/guard/bans` | `guard:read` | Заблокированные сейчас IP |
| POST | `/api/guard/bans` | `guard:write` | Заблокировать IP или сеть вручную |
| DELETE | `/api/guard/bans` | `guard:write` | Снять блокировку (body: {"key": адрес}) |
| GET | `/api/guard/firewall/rules` | `guard:read` | Правила, записанные Gately в nftables (текст) |

## Wake-on-LAN

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | Просмотр журнала работы модуля |
| DELETE | `/api/wol/logs` | `wol:write` | Очистить журнал работы модуля |
| GET | `/api/wol/devices` | `wol:read` | Список: Разбудить устройство |
| POST | `/api/wol/devices` | `wol:write` | Добавить: Разбудить устройство |
| PUT | `/api/wol/devices/{key}` | `wol:write` | Изменить: Разбудить устройство |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | Удалить: Разбудить устройство |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | Включить или отключить: Разбудить устройство |
| PUT | `/api/wol/devices-order` | `wol:write` | Изменить порядок: Разбудить устройство |
| PUT | `/api/wol/devices-group` | `wol:write` | Задать группу сразу для нескольких: Разбудить устройство |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | Отправить на устройство magic-пакет пробуждения |
| GET | `/api/wol/status` | `wol:read` | Просмотр состояния устройства в сети |

## Веб-терминал

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | Просмотр журнала работы модуля |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | Очистить журнал работы модуля |
| GET | `/api/webterminal/connections` | `webterminal:read` | Список: Подключение терминала |
| POST | `/api/webterminal/connections` | `webterminal:write` | Добавить: Подключение терминала |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | Изменить: Подключение терминала |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | Удалить: Подключение терминала |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | Включить или отключить: Подключение терминала |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | Изменить порядок: Подключение терминала |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | Задать группу сразу для нескольких: Подключение терминала |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | Показать секреты открытым текстом |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | Удалить записанные ключи хоста |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | Проверить сохранённое подключение |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | Проверить временные параметры подключения |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | Быстрый вход для настройки подключения |
| GET | `/api/webterminal/credentials` | `webterminal:read` | Список: Элемент |
| POST | `/api/webterminal/credentials` | `webterminal:write` | Добавить: Элемент |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | Изменить: Элемент |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | Удалить: Элемент |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | Включить или отключить: Элемент |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | Изменить порядок: Элемент |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | Показать секреты открытым текстом |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | Открыть сессию терминала (WebSocket) |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | Подключиться к существующей сессии терминала (WebSocket) |
| GET | `/api/webterminal/sessions` | `webterminal:read` | Список сессий терминала |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | Просмотр сведений о сессии терминала |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | Изменить заметку сессии терминала |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | Просмотр статистики сессий терминала |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | Закрыть сессию терминала |
| GET | `/api/webterminal/security` | Только сессия входа | Просмотр состояния настроек безопасности терминала |
| POST | `/api/webterminal/security/agreement` | Только сессия входа | Подтвердить соглашение об использовании терминала |
| POST | `/api/webterminal/security/2fa/setup` | Только сессия входа | Сгенерировать ключ двухфакторной аутентификации терминала |
| POST | `/api/webterminal/security/2fa/enable` | Только сессия входа | Включить двухфакторную аутентификацию терминала |
| POST | `/api/webterminal/security/2fa/disable` | Только сессия входа | Отключить двухфакторную аутентификацию терминала |
| POST | `/api/webterminal/security/verify2fa` | Только сессия входа | Проверить код двухфакторной аутентификации терминала |
| GET | `/api/webterminal/security/check2fa` | Только сессия входа | Проверить, пройдена ли двухфакторная аутентификация терминала |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | Список удалённых каталогов |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | Чтение удалённого файла |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | Записать удалённый файл |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | Загрузить файл на удалённый хост |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | Создать удалённый каталог |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | Создать пустой удалённый файл |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | Переименовать или переместить удалённый файл |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | Копировать удалённый файл |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | Изменить права удалённого файла |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | Удалить удалённый файл |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | Архивировать файлы на удалённом хосте |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | Распаковать файлы на удалённом хосте |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | Предпросмотр содержимого удалённого архива |
| GET | `/api/webterminal/settings` | `webterminal:read` | Чтение настроек веб-терминала |
| PUT | `/api/webterminal/settings` | `webterminal:write` | Сохранить настройки веб-терминала |
| GET | `/api/webterminal/shells` | `webterminal:read` | Список доступных на этой машине оболочек (Shell) |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | Чтение глобальных быстрых команд |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | Сохранить глобальные быстрые команды |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | Чтение раскладки разделённых панелей |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | Сохранить раскладку разделённых панелей |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | Сбросить раскладку разделённых панелей |

## Запланированная задача

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | Просмотр журнала работы модуля |
| DELETE | `/api/cron/logs` | `cron:write` | Очистить журнал работы модуля |
| GET | `/api/cron/tasks` | `cron:read` | Список: Запланированная задача |
| POST | `/api/cron/tasks` | `cron:write` | Добавить: Запланированная задача |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | Изменить: Запланированная задача |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | Удалить: Запланированная задача |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | Включить или отключить: Запланированная задача |
| PUT | `/api/cron/tasks-order` | `cron:write` | Изменить порядок: Запланированная задача |
| PUT | `/api/cron/tasks-group` | `cron:write` | Задать группу сразу для нескольких: Запланированная задача |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | Немедленно выполнить запланированную задачу |
| POST | `/api/cron/parse` | `cron:write` | Разобрать выражение Cron и показать предпросмотр |

## Управление Docker

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | Просмотр журнала работы модуля |
| DELETE | `/api/docker/logs` | `docker:write` | Очистить журнал работы модуля |
| GET | `/api/docker/tasks` | `docker:read` | Список фоновых задач |
| GET | `/api/docker/tasks/{id}` | `docker:read` | Просмотр хода выполнения и вывода фоновой задачи |
| DELETE | `/api/docker/tasks` | `docker:write` | Очистить завершённые фоновые задачи |
| GET | `/api/docker/images` | `docker:read` | Список образов |
| POST | `/api/docker/images/pull` | `docker:write` | Загрузить образ |
| POST | `/api/docker/images/push` | `docker:write` | Отправить образ |
| POST | `/api/docker/images/search` | `docker:write` | Поиск образов |
| POST | `/api/docker/images/load` | `docker:write` | Импортировать файл образа |
| POST | `/api/docker/images/build` | `docker:write` | Собрать образ |
| GET | `/api/docker/images/{id}` | `docker:read` | Просмотр сведений об образе |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | Присвоить тег образу |
| DELETE | `/api/docker/images/{id}` | `docker:write` | Удалить образ |
| GET | `/api/docker/images/{id}/export` | `docker:write` | Экспортировать образ |
| GET | `/api/docker/networks` | `docker:read` | Список сетей |
| POST | `/api/docker/networks` | `docker:write` | Создать сеть |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | Удалить сеть |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | Подключить контейнер к сети |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | Отключить контейнер от сети |
| GET | `/api/docker/volumes` | `docker:read` | Список томов |
| POST | `/api/docker/volumes` | `docker:write` | Создать том данных |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | Удалить том данных |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | Экспортировать том данных |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | Импортировать данные в том |
| GET | `/api/docker/compose/projects` | `docker:read` | Список проектов Compose |
| POST | `/api/docker/compose/projects` | `docker:write` | Создать проект Compose |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | Запустить проект Compose |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | Загрузить образы проекта Compose |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | Выполнить операцию над проектом Compose |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | Чтение файла конфигурации Compose |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | Сохранить файл конфигурации Compose |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | Просмотр журнала проекта Compose |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | Удалить проект Compose |
| GET | `/api/docker/info` | `docker:read` | Просмотр информации о движке Docker |
| GET | `/api/docker/disk-usage` | `docker:read` | Просмотр использования диска Docker |
| POST | `/api/docker/prune` | `docker:write` | Очистить неиспользуемые ресурсы Docker |
| GET | `/api/docker/settings` | `docker:read` | Чтение настроек модуля Docker |
| PUT | `/api/docker/settings` | `docker:write` | Сохранить настройки модуля Docker |
| GET | `/api/docker/settings/secrets` | `docker:write` | Показать пароли реестров открытым текстом (ключи `registry:<адрес реестра>`) |
| GET | `/api/docker/containers` | `docker:read` | Список контейнеров |
| POST | `/api/docker/containers` | `docker:write` | Создать контейнер |
| POST | `/api/docker/containers/check-all` | `docker:write` | Проверить обновления образов всех контейнеров |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | Пакетное обновление контейнеров |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | Проверить, есть ли новая версия образа |
| GET | `/api/docker/containers/{id}` | `docker:read` | Просмотр сведений о контейнере |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | Просмотр переменных окружения и команды запуска контейнера в открытом виде |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | Удалить контейнер |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | Изменить конфигурацию контейнера и пересоздать его |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | Переименовать контейнер |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | Сохранить контейнер как образ |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | Проверить, есть ли обновления образа контейнера |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | Обновить образ контейнера и пересоздать контейнер |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | Выполнить операцию над контейнером |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | Просмотр журнала контейнера |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | Просмотр использования ресурсов контейнером |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | Просмотр процессов внутри контейнера |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | Список каталогов внутри контейнера |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | Чтение файла внутри контейнера |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | Скачать файл из контейнера |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | Записать файл внутри контейнера |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | Создать каталог внутри контейнера |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | Переименовать файл внутри контейнера |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | Загрузить файл в контейнер |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | Удалить файл внутри контейнера |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | Открыть терминал внутри контейнера (WebSocket) |

## WebDAV-сервер

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | Просмотр журнала работы модуля |
| DELETE | `/api/webdav/logs` | `webdav:write` | Очистить журнал работы модуля |
| GET | `/api/webdav/settings` | `webdav:read` | Чтение настроек WebDAV |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | Показать пароли локальных пользователей открытым текстом |
| GET | `/api/webdav/shared-users` | `webdav:read` | Получить общих пользователей файловых служб (старый метод) |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | Сохранить общих пользователей файловых служб (старый метод) |
| PUT | `/api/webdav/settings` | `webdav:write` | Сохранить настройки WebDAV |

## Служба FTP

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | Просмотр журнала работы модуля |
| DELETE | `/api/ftp/logs` | `ftp:write` | Очистить журнал работы модуля |
| GET | `/api/ftp/settings` | `ftp:read` | Чтение настроек FTP |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | Показать пароли локальных пользователей открытым текстом |
| GET | `/api/ftp/shared-users` | `ftp:read` | Получить общих пользователей файловых служб (старый метод) |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | Сохранить общих пользователей файловых служб (старый метод) |
| PUT | `/api/ftp/settings` | `ftp:write` | Сохранить настройки FTP |

## Служба SMB

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | Просмотр журнала работы модуля |
| DELETE | `/api/smb/logs` | `smb:write` | Очистить журнал работы модуля |
| GET | `/api/smb/settings` | `smb:read` | Получить настройки SMB |
| GET | `/api/smb/settings/secrets` | `smb:write` | Показать пароли локальных пользователей открытым текстом |
| GET | `/api/smb/shared-users` | `smb:read` | Получить общих пользователей файловых служб (старый метод) |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | Сохранить общих пользователей файловых служб (старый метод) |
| PUT | `/api/smb/settings` | `smb:write` | Сохранить настройки SMB |

## File Browser

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | Просмотр журнала работы модуля |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | Очистить журнал работы модуля |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | Список: Экземпляр File Browser |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | Добавить: Экземпляр File Browser |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Изменить: Экземпляр File Browser |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Удалить: Экземпляр File Browser |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | Включить или отключить: Экземпляр File Browser |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | Изменить порядок: Экземпляр File Browser |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | Задать группу сразу для нескольких: Экземпляр File Browser |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | Показать секреты открытым текстом |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | Перезапустить экземпляр File Browser |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | Получить настройки программы File Browser |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | Сохранить настройки программы File Browser |
| GET | `/api/filebrowser/install` | `filebrowser:read` | Просмотр сведений об установке и хода установки управляемой программы |
| POST | `/api/filebrowser/install` | `filebrowser:write` | Установить или обновить управляемую программу онлайн |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | Переключиться на сохранённую предыдущую версию |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | Удалить управляемую программу |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | Удалить сохранённую предыдущую версию |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | Сбросить настройки модуля по умолчанию |

## Монтирование хранилищ

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | Просмотр журнала работы модуля |
| DELETE | `/api/storage/logs` | `storage:write` | Очистить журнал работы модуля |
| GET | `/api/storage/mounts` | `storage:read` | Список: Монтирование хранилищ |
| POST | `/api/storage/mounts` | `storage:write` | Добавить: Монтирование хранилищ |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | Изменить: Монтирование хранилищ |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | Удалить: Монтирование хранилищ |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | Включить или отключить: Монтирование хранилищ |
| PUT | `/api/storage/mounts-order` | `storage:write` | Изменить порядок: Монтирование хранилищ |
| PUT | `/api/storage/mounts-group` | `storage:write` | Задать группу сразу для нескольких: Монтирование хранилищ |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | Показать секреты открытым текстом |
| GET | `/api/storage/files/{key}/list` | `storage:write` | Список каталогов хранилища |
| GET | `/api/storage/files/{key}/download` | `storage:write` | Скачать файл из хранилища |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | Загрузить файл в хранилище |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | Создать каталог хранилища |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | Переименовать или переместить файл хранилища |
| DELETE | `/api/storage/files/{key}` | `storage:write` | Удалить файл хранилища |

## Центр уведомлений

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | Просмотр журнала работы модуля |
| DELETE | `/api/notify/logs` | `notify:write` | Очистить журнал работы модуля |
| GET | `/api/notify/channels` | `notify:read` | Список: Каналы уведомлений |
| POST | `/api/notify/channels` | `notify:write` | Добавить: Каналы уведомлений |
| PUT | `/api/notify/channels/{key}` | `notify:write` | Изменить: Каналы уведомлений |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | Удалить: Каналы уведомлений |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | Включить или отключить: Каналы уведомлений |
| PUT | `/api/notify/channels-order` | `notify:write` | Изменить порядок: Каналы уведомлений |
| PUT | `/api/notify/channels-group` | `notify:write` | Задать группу сразу для нескольких: Каналы уведомлений |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | Показать секреты открытым текстом |
| GET | `/api/notify/rules` | `notify:read` | Список: Правила уведомлений |
| POST | `/api/notify/rules` | `notify:write` | Добавить: Правила уведомлений |
| PUT | `/api/notify/rules/{key}` | `notify:write` | Изменить: Правила уведомлений |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | Удалить: Правила уведомлений |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | Включить или отключить: Правила уведомлений |
| PUT | `/api/notify/rules-order` | `notify:write` | Изменить порядок: Правила уведомлений |
| PUT | `/api/notify/rules-group` | `notify:write` | Задать группу сразу для нескольких: Правила уведомлений |
| GET | `/api/notify/event-types` | `notify:read` | Типы событий, на которые можно подписаться |
| GET | `/api/notify/history` | `notify:read` | Последние записи доставки |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | Отправить тестовое уведомление в канал |

## Диагностика сети

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | Просмотр журнала работы модуля |
| DELETE | `/api/nettools/logs` | `nettools:write` | Очистить журнал работы модуля |
| POST | `/api/nettools/run` | `nettools:write` | Запустить инструмент диагностики, возвращает задачу (tool: ping / traceroute / tcp / dns / speed / scan) |
| GET | `/api/nettools/tasks` | `nettools:read` | Последние задачи диагностики |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | Сведения о задаче и вывод |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | Отменить выполняемую задачу или удалить завершённую |
| GET | `/api/nettools/settings` | `nettools:read` | Настройки диагностики |
| PUT | `/api/nettools/settings` | `nettools:write` | Изменить список запрещённых для сканирования адресов |
| GET | `/api/nettools/networks` | `nettools:read` | Подсети этой машины, доступные для сканирования локальной сети |

## Резервная копия конфигурации

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | Просмотр журнала работы модуля |
| DELETE | `/api/backup/logs` | `backup:write` | Очистить журнал работы модуля |
| GET | `/api/backup/targets` | `backup:read` | Список: Назначение резервных копий |
| POST | `/api/backup/targets` | `backup:write` | Добавить: Назначение резервных копий |
| PUT | `/api/backup/targets/{key}` | `backup:write` | Изменить: Назначение резервных копий |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | Удалить: Назначение резервных копий |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | Включить или отключить: Назначение резервных копий |
| PUT | `/api/backup/targets-order` | `backup:write` | Изменить порядок: Назначение резервных копий |
| GET | `/api/backup/settings` | `backup:read` | Настройки резервного копирования |
| PUT | `/api/backup/settings` | `backup:write` | Изменить настройки резервного копирования (передайте заполнитель вместо пароля, чтобы оставить его без изменений) |
| GET | `/api/backup/secrets` | `backup:write` | Показать секреты открытым текстом |
| POST | `/api/backup/run` | `backup:write` | Выполнить резервное копирование сейчас |
| GET | `/api/backup/status` | `backup:read` | Результат последней резервной копии и время следующего запуска |
| GET | `/api/backup/mounts` | `backup:read` | Точки монтирования хранилищ, доступные как назначение резервных копий |
| GET | `/api/backup/local` | Только сессия входа | Список локальных файлов резервных копий |
| GET | `/api/backup/local/{name}` | Только сессия входа | Скачать локальный файл резервной копии |

## Общие пользователи файловых служб

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | Получить общих пользователей файловых служб |
| PUT | `/api/fileusers` | `fileusers:write` | Сохранить общих пользователей файловых служб |
| GET | `/api/fileusers/secrets` | `fileusers:write` | Показать пароли общих пользователей открытым текстом |

## Система

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/session` | Любой действующий токен | Просмотр текущего удостоверения аутентификации |
| GET | `/api/status` | `status:read` | Просмотр состояния системы и модулей |
| GET | `/api/logs` | `logs:read` | Просмотр системного журнала |
| GET | `/api/system/settings` | Только сессия входа | Чтение глобальных настроек |
| PUT | `/api/system/settings` | Только сессия входа | Сохранить глобальные настройки |
| GET | `/api/system/settings/secrets` | Только сессия входа | Показать Client Secret стороннего входа открытым текстом |
| GET | `/api/system/info` | `status:read` | Просмотр информации о системе |
| GET | `/api/system/update` | `status:read` | Проверить наличие новой версии |
| POST | `/api/system/upgrade` | Только сессия входа | Обновить онлайн до последней версии |
| GET | `/api/system/upgrade/status` | Только сессия входа | Показать ход онлайн-обновления |
| POST | `/api/system/upgrade/cancel` | Только сессия входа | Отменить онлайн-обновление |
| POST | `/api/system/upgrade/upload` | Только сессия входа | Обновить из загруженного пакета |
| GET | `/api/system/backup` | Только сессия входа | Скачать резервную копию конфигурации |
| GET | `/api/system/diagnostics` | Только сессия входа | Экспорт обезличенного диагностического пакета |
| GET | `/api/audit` | Только сессия входа | Запросить журнал аудита |
| GET | `/api/metrics` | `metrics:read` | Глобальные метрики Prometheus |
| GET | `/api/history` | Только сессия входа | Список версий истории конфигурации |
| GET | `/api/history/{name}/{id}/diff` | Только сессия входа | Сравнить версию из истории с текущей конфигурацией |
| POST | `/api/history/{name}/{id}/restore` | Только сессия входа | Откатить конфигурацию к версии из истории |
| POST | `/api/system/restore` | Только сессия входа | Восстановить конфигурацию из резервной копии |
| POST | `/api/system/restart` | Только сессия входа | Перезапустить Gately |
| POST | `/api/system/selfcheck` | Только сессия входа | Самопроверка сети и конфигурации |
| GET | `/api/system/onboarding` | Только сессия входа | Требуется ли начальная настройка |
| POST | `/api/system/onboarding/dismiss` | Только сессия входа | Завершить или пропустить начальную настройку |
| GET | `/api/openapi.json` | Любой действующий токен | Получить описание OpenAPI этого API |

## Импорт конфигурации

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | Импорт конфигурации Lucky |

## API-токен

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| GET | `/api/tokens` | Только сессия входа | Список API-токенов |
| GET | `/api/tokens/scopes` | Только сессия входа | Список областей прав, доступных для назначения |
| POST | `/api/tokens` | Только сессия входа | Создать API-токен |
| PUT | `/api/tokens/order` | Только сессия входа | Изменить порядок API-токенов |
| PUT | `/api/tokens/group` | Только сессия входа | Задать группу нескольким API-токенам |
| PUT | `/api/tokens/{id}` | Только сессия входа | Изменить имя, группу, права, срок действия и состояние API-токена |
| POST | `/api/tokens/{id}/regenerate` | Только сессия входа | Перегенерировать токен в открытом виде |
| DELETE | `/api/tokens/{id}` | Только сессия входа | Удалить API-токен |

## Учётная запись

| Метод | Путь | Необходимое право | Описание |
| --- | --- | --- | --- |
| PUT | `/api/account` | Только сессия входа | Изменить имя пользователя и пароль администратора |
| PUT | `/api/account/ui-language` | Только сессия входа | Сохранить язык панели управления |
| GET | `/api/account/passkeys` | Только сессия входа | Список зарегистрированных Passkey |
| POST | `/api/account/passkeys/begin` | Только сессия входа | Начать регистрацию Passkey |
| POST | `/api/account/passkeys` | Только сессия входа | Завершить регистрацию Passkey |
| DELETE | `/api/account/passkeys/{id}` | Только сессия входа | Удалить Passkey |
| GET | `/api/account/users` | Только сессия входа | Список учётных записей только для чтения |
| POST | `/api/account/users` | Только сессия входа | Создать учётную запись только для чтения |
| PUT | `/api/account/users/{name}` | Только сессия входа | Сбросить пароль учётной записи только для чтения |
| DELETE | `/api/account/users/{name}` | Только сессия входа | Удалить учётную запись только для чтения |
| GET | `/api/account/totp` | Только сессия входа | Просмотр состояния двухфакторной аутентификации |
| POST | `/api/account/totp/setup` | Только сессия входа | Сгенерировать ключ двухфакторной аутентификации |
| POST | `/api/account/totp/enable` | Только сессия входа | Включить двухфакторную аутентификацию |
| POST | `/api/account/totp/disable` | Только сессия входа | Отключить двухфакторную аутентификацию |

## Структуры данных

### backup.LocalFile

| Поле | Тип |
| --- | --- |
| `modTime` | Время (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| Поле | Тип |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| Поле | Тип |
| --- | --- |
| `lastRun` | Время (RFC 3339) |
| `manual` | boolean |
| `nextRun` | Время (RFC 3339) |
| `results` | Массив из `backup.Result` |
| `running` | boolean |

### backup.Target

| Поле | Тип |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| Поле | Тип |
| --- | --- |
| `count` | integer |
| `created` | Время (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Время (RFC 3339) |

### cloudflared.Tunnel

| Поле | Тип |
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

| Поле | Тип |
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

### crud.enableRequest

| Поле | Тип |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| Поле | Тип |
| --- | --- |
| `group` | string |
| `keys` | Массив из string |

### crud.orderRequest

| Поле | Тип |
| --- | --- |
| `keys` | Массив из string |

### ddns.Record

| Поле | Тип |
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

### ddns.Task

| Поле | Тип |
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
| `records` | Массив из `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| Поле | Тип |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| Поле | Тип |
| --- | --- |
| `records` | Массив из `ddns.recordLayout` |

### dnsprovider.Credentials

| Поле | Тип |
| --- | --- |
| `extra` | Словарь из string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| Поле | Тип |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| Поле | Тип |
| --- | --- |
| `accessUrls` | Массив из string |
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
| `sources` | Массив из `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| Поле | Тип |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| Поле | Тип |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| Поле | Тип |
| --- | --- |
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

### frp.ClientSettings

| Поле | Тип |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| Поле | Тип |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| Поле | Тип |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | Массив из `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | Массив из string |

### frp.Proxy

| Поле | Тип |
| --- | --- |
| `client` | string |
| `customDomains` | Массив из string |
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

| Поле | Тип |
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
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| Поле | Тип |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| Поле | Тип |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| Поле | Тип |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| Поле | Тип |
| --- | --- |
| `buildTime` | Время (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| Поле | Тип |
| --- | --- |
| `count` | integer |
| `created` | Время (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Время (RFC 3339) |

### guard.FirewallStatus

| Поле | Тип |
| --- | --- |
| `active` | boolean |
| `checkedAt` | Время (RFC 3339) |
| `dropped` | Словарь из integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | Словарь из integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | Массив из integer |
| `version` | string |

### guard.Jail

| Поле | Тип |
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

| Поле | Тип |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| Поле | Тип |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `threshold` | integer |

### guard.Settings

| Поле | Тип |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | Массив из string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| Поле | Тип |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | Массив из integer |

### guard.TestResult

| Поле | Тип |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| Поле | Тип |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| Поле | Тип |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| Поле | Тип |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| Поле | Тип |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | Массив из string |

### nettools.Request

| Поле | Тип |
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

| Поле | Тип |
| --- | --- |
| `deny` | Массив из string |

### nettools.Task

| Поле | Тип |
| --- | --- |
| `error` | string |
| `finished` | Время (RFC 3339) |
| `id` | string |
| `lines` | Массив из string |
| `result` | Любое |
| `started` | Время (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| Поле | Тип |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | Время (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| Поле | Тип |
| --- | --- |
| `channels` | Массив из string |
| `enable` | boolean |
| `events` | Массив из string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### portforward.Rule

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | Массив из string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | Массив из string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### protoroute.Route

| Поле | Тип |
| --- | --- |
| `alpn` | Массив из string |
| `domains` | Массив из string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | Массив из string |
| `targetPort` | integer |

### protoroute.Rule

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | Массив из string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | Массив из `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | Массив из string |
| `targetPorts` | string |

### ssl.ACMEConfig

| Поле | Тип |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `domains` | Массив из string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | Массив из string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| Поле | Тип |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | Массив из `ssl.Deploy` |
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
| `updatedAt` | Время (RFC 3339) |

### ssl.Deploy

| Поле | Тип |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | Массив из string |
| `deployedAt` | Время (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| Поле | Тип |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| Поле | Тип |
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

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| Поле | Тип |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| Поле | Тип |
| --- | --- |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `keepAliveServers` | Массив из string |
| `key` | string |
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
| `stunServers` | Массив из string |
| `stunTimeout` | integer |
| `targetAddresses` | Массив из string |
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

### thirdauth.Provider

| Поле | Тип |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| Поле | Тип |
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
| `ruleSets` | Массив из `waf.RuleSet` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### webhook.Config

| Поле | Тип |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | Массив из string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `successContent` | Массив из string |
| `timeout` | integer |
| `url` | string |

### webservice.AuthUser

| Поле | Тип |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | Массив из string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BanInfo

| Поле | Тип |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | Время (RFC 3339) |

### webservice.CacheConfig

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `ttl` | integer |

### webservice.ForwardAuth

| Поле | Тип |
| --- | --- |
| `copyHeaders` | Массив из string |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| Поле | Тип |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | Время (RFC 3339) |
| `url` | string |

### webservice.Listener

| Поле | Тип |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | Массив из `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| Поле | Тип |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | Массив из string |
| `targets` | Массив из string |

### webservice.OIDCClient

| Поле | Тип |
| --- | --- |
| `allow` | Массив из string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | Массив из string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| Поле | Тип |
| --- | --- |
| `clients` | Массив из `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.PathRule

| Поле | Тип |
| --- | --- |
| `auth` | string |
| `ipList` | Массив из string |
| `ipMode` | string |
| `ipSets` | Массив из string |
| `prefix` | string |

### webservice.Point

| Поле | Тип |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.RateLimit

| Поле | Тип |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.Rewrite

| Поле | Тип |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| Поле | Тип |
| --- | --- |
| `authSecret` | string |
| `authUsers` | Массив из `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | Массив из `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | Массив из string |

### webservice.ShareLink

| Поле | Тип |
| --- | --- |
| `expiresAt` | Время (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| Поле | Тип |
| --- | --- |
| `accessLog` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | Массив из string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | Массив из string |
| `enable` | boolean |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `insecureSkipVerify` | boolean |
| `ipList` | Массив из string |
| `ipMode` | string |
| `ipSets` | Массив из string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | Массив из `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | Массив из string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | Массив из string |
| `responseHeaders` | Массив из string |
| `rewrites` | Массив из `webservice.Rewrite` |
| `root` | string |
| `rules` | Массив из `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | Массив из `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | Массив из string |
| `text` | string |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | Массив из `webservice.UserRoute` |
| `users` | Массив из string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteSharedUsers

| Поле | Тип |
| --- | --- |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| Поле | Тип |
| --- | --- |
| `bytesOut` | integer |
| `series` | Массив из `webservice.Point` |
| `status` | Словарь из integer |
| `topCountries` | Массив из `webservice.TopItem` |
| `topIps` | Массив из `webservice.TopItem` |
| `upstreams` | Массив из `webservice.UpstreamStatus` |

### webservice.TopItem

| Поле | Тип |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.UpstreamStatus

| Поле | Тип |
| --- | --- |
| `backup` | boolean |
| `conns` | integer |
| `healthy` | boolean |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| Поле | Тип |
| --- | --- |
| `targets` | Массив из string |
| `users` | Массив из string |

### webservice.UserSite

| Поле | Тип |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | Массив из string |
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

| Поле | Тип |
| --- | --- |
| `authUsers` | Массив из `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Массив из `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| Поле | Тип |
| --- | --- |
| `authUsers` | Массив из `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Массив из `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| Поле | Тип |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.geoipDownload

| Поле | Тип |
| --- | --- |
| `url` | string |

### webservice.totpRequest

| Поле | Тип |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| Поле | Тип |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| Поле | Тип |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | Массив из string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| Поле | Тип |
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

| Поле | Тип |
| --- | --- |
| `env` | Словарь из string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| Поле | Тип |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| Поле | Тип |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
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

| Поле | Тип |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.Device

| Поле | Тип |
| --- | --- |
| `broadcastIps` | Массив из string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | Массив из string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |

