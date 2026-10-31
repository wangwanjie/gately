# API de administração do Gately

A API de administração do Gately serve para gerenciamento em lote por scripts e automação.

Autenticação: crie um token na página "Tokens de API" da administração e envie no cabeçalho da requisição `Authorization: Bearer gtl_xxxx`.

Permissões: cada token possui apenas as permissões marcadas na criação, no formato `<módulo>:read` ou `<módulo>:write`; a permissão de escrita inclui a de leitura. As interfaces de conta, verificação em duas etapas, gerenciamento de tokens, backup, restauração, reinício e configurações do sistema só podem ser usadas com sessão de login.

O corpo da requisição usa `Content-Type: application/json`; os erros são sempre retornados como `{"error": "descrição"}`.

Módulos desativados em "Configurações → Módulos" não oferecem API: seus endpoints retornam 403 e deixam de aparecer na documentação da API da instância em execução.

Site oficial: https://gately.cn

A descrição legível por máquina está em `openapi.json` (OpenAPI 3.0) no mesmo diretório.

## Início rápido

```bash
# Ver o status do sistema (requer status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# Listar todas as tarefas DDNS (requer ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Sincronizar uma tarefa DDNS imediatamente (requer ddns:write; a key vem da lista retornada no passo anterior)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## Permissões

| Domínio de permissão | Nome | Permissões disponíveis |
| --- | --- | --- |
| `portforward` | Encaminhamento de portas | `portforward:read`、`portforward:write` |
| `protoroute` | Roteamento por protocolo | `protoroute:read`、`protoroute:write` |
| `ddns` | DNS dinâmico | `ddns:read`、`ddns:write` |
| `stun` | Travessia STUN | `stun:read`、`stun:write` |
| `tailscale` | Rede Tailscale | `tailscale:read`、`tailscale:write` |
| `frp` | Travessia de rede interna frp | `frp:read`、`frp:write` |
| `cloudflared` | Túnel Cloudflare | `cloudflared:read`、`cloudflared:write` |
| `ssl` | Certificado SSL | `ssl:read`、`ssl:write` |
| `waf` | Firewall WAF | `waf:read`、`waf:write` |
| `webservice` | Serviço web | `webservice:read`、`webservice:write` |
| `guard` | Prevenção de intrusão | `guard:read`、`guard:write` |
| `wol` | Wake-on-LAN | `wol:read`、`wol:write` |
| `webterminal` | Terminal web | `webterminal:read`、`webterminal:write` |
| `cron` | Tarefa agendada | `cron:read`、`cron:write` |
| `docker` | Gerenciamento do Docker | `docker:read`、`docker:write` |
| `webdav` | Serviço WebDAV | `webdav:read`、`webdav:write` |
| `ftp` | Serviço FTP | `ftp:read`、`ftp:write` |
| `smb` | Serviço SMB | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | Montagens de armazenamento | `storage:read`、`storage:write` |
| `notify` | Central de notificações | `notify:read`、`notify:write` |
| `nettools` | Diagnóstico de rede | `nettools:read`、`nettools:write` |
| `backup` | Backup de configuração | `backup:read`、`backup:write` |
| `status` | Estado do sistema | `status:read` |
| `logs` | Log do sistema | `logs:read` |
| `metrics` | Métricas (Prometheus) | `metrics:read` |
| `import` | Importação de configuração do Lucky | `import:write` |
| `fileusers` | Usuários compartilhados dos serviços de arquivos | `fileusers:read`、`fileusers:write` |

Curingas: `*:read` significa somente leitura em todos os módulos; `*:write` ou `*` significa leitura e escrita em todos os módulos.

## Encaminhamento de portas

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | Ver logs de execução do módulo |
| DELETE | `/api/portforward/logs` | `portforward:write` | Limpar logs de execução do módulo |
| GET | `/api/portforward/rules` | `portforward:read` | Listar Regra de encaminhamento de portas |
| POST | `/api/portforward/rules` | `portforward:write` | Adicionar Regra de encaminhamento de portas |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | Modificar Regra de encaminhamento de portas |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | Excluir Regra de encaminhamento de portas |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | Ativar ou desativar Regra de encaminhamento de portas |
| PUT | `/api/portforward/rules-order` | `portforward:write` | Reordenar Regra de encaminhamento de portas |
| PUT | `/api/portforward/rules-group` | `portforward:write` | Definir o grupo de vários itens: Regra de encaminhamento de portas |
| GET | `/api/portforward/settings` | `portforward:read` | Ler configurações globais do encaminhamento de portas |
| PUT | `/api/portforward/settings` | `portforward:write` | Salvar configurações globais do encaminhamento de portas |

## Roteamento por protocolo

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | Ver logs de execução do módulo |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | Limpar logs de execução do módulo |
| GET | `/api/protoroute/rules` | `protoroute:read` | Listar regra de roteamento por protocolo |
| POST | `/api/protoroute/rules` | `protoroute:write` | Adicionar regra de roteamento por protocolo |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | Modificar regra de roteamento por protocolo |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | Excluir regra de roteamento por protocolo |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | Ativar ou desativar regra de roteamento por protocolo |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | Reordenar regra de roteamento por protocolo |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | Definir o grupo de vários itens: regra de roteamento por protocolo |
| GET | `/api/protoroute/settings` | `protoroute:read` | Ler as configurações do roteamento por protocolo |
| PUT | `/api/protoroute/settings` | `protoroute:write` | Salvar as configurações do roteamento por protocolo |

## DNS dinâmico

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | Ver logs de execução do módulo |
| DELETE | `/api/ddns/logs` | `ddns:write` | Limpar logs de execução do módulo |
| GET | `/api/ddns/tasks` | `ddns:read` | Listar Tarefa DDNS |
| POST | `/api/ddns/tasks` | `ddns:write` | Adicionar Tarefa DDNS |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | Modificar Tarefa DDNS |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | Excluir Tarefa DDNS |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | Ativar ou desativar Tarefa DDNS |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | Reordenar Tarefa DDNS |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | Definir o grupo de vários itens: Tarefa DDNS |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | Ver segredos em texto puro |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | Sincronizar uma tarefa DDNS imediatamente |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | Excluir um registro de uma tarefa DDNS |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | Reordenar e agrupar os registros DNS de uma tarefa |
| GET | `/api/ddns/providers` | `ddns:read` | Listar provedores de DNS suportados e seus parâmetros necessários |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | Listar placas de rede e endereços desta máquina |
| POST | `/api/ddns/iptest` | `ddns:write` | Testar a obtenção do IP público |
| GET | `/api/ddns/defaults` | `ddns:read` | Ler valores padrão do DDNS |
| GET | `/api/ddns/settings` | `ddns:read` | Ler configurações globais do DDNS |
| PUT | `/api/ddns/settings` | `ddns:write` | Salvar configurações globais do DDNS |

## Travessia STUN

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | Ver logs de execução do módulo |
| DELETE | `/api/stun/logs` | `stun:write` | Limpar logs de execução do módulo |
| GET | `/api/stun/rules` | `stun:read` | Listar Regra STUN |
| POST | `/api/stun/rules` | `stun:write` | Adicionar Regra STUN |
| PUT | `/api/stun/rules/{key}` | `stun:write` | Modificar Regra STUN |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | Excluir Regra STUN |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | Ativar ou desativar Regra STUN |
| PUT | `/api/stun/rules-order` | `stun:write` | Reordenar Regra STUN |
| PUT | `/api/stun/rules-group` | `stun:write` | Definir o grupo de vários itens: Regra STUN |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | Reiniciar uma regra STUN |
| GET | `/api/stun/netinterfaces` | `stun:read` | Listar placas de rede e endereços desta máquina |
| GET | `/api/stun/settings` | `stun:read` | Ler configurações globais do STUN |
| PUT | `/api/stun/settings` | `stun:write` | Salvar configurações globais do STUN |

## Rede Tailscale

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | Ver logs de execução do módulo |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | Limpar logs de execução do módulo |
| GET | `/api/tailscale/settings` | `tailscale:read` | Ler configurações do Tailscale |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Salvar configurações do Tailscale |
| GET | `/api/tailscale/secrets` | `tailscale:write` | Ver segredos em texto puro |
| GET | `/api/tailscale/status` | `tailscale:read` | Ver o estado da conexão do Tailscale |
| POST | `/api/tailscale/restart` | `tailscale:write` | Reiniciar o Tailscale |
| POST | `/api/tailscale/logout` | `tailscale:write` | Sair do login do Tailscale |
| GET | `/api/tailscale/install` | `tailscale:read` | Ver as informações de instalação e o progresso de um programa gerenciado |
| POST | `/api/tailscale/install` | `tailscale:write` | Instalar ou atualizar online um programa gerenciado |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | Mudar para uma versão anterior salva |
| DELETE | `/api/tailscale/install` | `tailscale:write` | Desinstalar um programa gerenciado |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | Excluir uma versão anterior salva |
| POST | `/api/tailscale/reset` | `tailscale:write` | Restaurar as configurações padrão do módulo |

## Travessia de rede interna frp

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | Ver logs de execução do módulo |
| DELETE | `/api/frp/logs` | `frp:write` | Limpar logs de execução do módulo |
| GET | `/api/frp/clients` | `frp:read` | Listar Cliente frp |
| POST | `/api/frp/clients` | `frp:write` | Adicionar Cliente frp |
| PUT | `/api/frp/clients/{key}` | `frp:write` | Modificar Cliente frp |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | Excluir Cliente frp |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | Ativar ou desativar Cliente frp |
| PUT | `/api/frp/clients-order` | `frp:write` | Reordenar Cliente frp |
| PUT | `/api/frp/clients-group` | `frp:write` | Definir o grupo de vários itens: Cliente frp |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | Ver segredos em texto puro |
| GET | `/api/frp/servers` | `frp:read` | Listar Servidor frp |
| POST | `/api/frp/servers` | `frp:write` | Adicionar Servidor frp |
| PUT | `/api/frp/servers/{key}` | `frp:write` | Modificar Servidor frp |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | Excluir Servidor frp |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | Ativar ou desativar Servidor frp |
| PUT | `/api/frp/servers-order` | `frp:write` | Reordenar Servidor frp |
| PUT | `/api/frp/servers-group` | `frp:write` | Definir o grupo de vários itens: Servidor frp |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | Ver segredos em texto puro |
| GET | `/api/frp/proxies` | `frp:read` | Listar proxies do frp |
| POST | `/api/frp/proxies` | `frp:write` | Adicionar proxies do frp |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | Modificar proxies do frp |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | Excluir proxies do frp |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | Ativar ou desativar proxies do frp |
| PUT | `/api/frp/proxies-order` | `frp:write` | Reordenar proxies do frp |
| PUT | `/api/frp/proxies-group` | `frp:write` | Definir o grupo de vários itens: proxies do frp |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | Ver segredos em texto puro |
| POST | `/api/frp/import` | `frp:write` | Importar configuração existente de frpc / frps (suporta ini antigo e toml novo); com apply igual a false apenas pré-visualiza |
| GET | `/api/frp/install` | `frp:read` | Ver as informações de instalação e o progresso de um programa gerenciado |
| POST | `/api/frp/install` | `frp:write` | Instalar ou atualizar online um programa gerenciado |
| POST | `/api/frp/install/rollback` | `frp:write` | Mudar para uma versão anterior salva |
| DELETE | `/api/frp/install` | `frp:write` | Desinstalar um programa gerenciado |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | Excluir uma versão anterior salva |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | Reiniciar instância do frpc |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | Reiniciar instância do frps |
| GET | `/api/frp/settings` | `frp:read` | Obter caminhos e versões dos programas |
| PUT | `/api/frp/settings` | `frp:write` | Salvar caminhos dos programas |
| POST | `/api/frp/reset` | `frp:write` | Restaurar as configurações padrão do módulo |

## Túnel Cloudflare

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | Ver logs de execução do módulo |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | Limpar logs de execução do módulo |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | Listar Túnel Cloudflare |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Adicionar Túnel Cloudflare |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Modificar Túnel Cloudflare |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Excluir Túnel Cloudflare |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Ativar ou desativar Túnel Cloudflare |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Reordenar Túnel Cloudflare |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Definir o grupo de vários itens: Túnel Cloudflare |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | Ver segredos em texto puro |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Reiniciar um túnel Cloudflare |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | Ler as rotas de um túnel com token |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | Salvar as rotas de um túnel com token |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Ler configurações do túnel Cloudflare |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Salvar configurações do túnel Cloudflare |
| GET | `/api/cloudflared/install` | `cloudflared:read` | Ver as informações de instalação e o progresso de um programa gerenciado |
| POST | `/api/cloudflared/install` | `cloudflared:write` | Instalar ou atualizar online um programa gerenciado |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | Mudar para uma versão anterior salva |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | Desinstalar um programa gerenciado |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | Excluir uma versão anterior salva |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | Restaurar as configurações padrão do módulo |

## Certificado SSL

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | Ver logs de execução do módulo |
| DELETE | `/api/ssl/logs` | `ssl:write` | Limpar logs de execução do módulo |
| GET | `/api/ssl/certs` | `ssl:read` | Listar Certificado |
| POST | `/api/ssl/certs` | `ssl:write` | Adicionar Certificado |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | Modificar Certificado |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | Excluir Certificado |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | Ativar ou desativar Certificado |
| PUT | `/api/ssl/certs-order` | `ssl:write` | Reordenar Certificado |
| PUT | `/api/ssl/certs-group` | `ssl:write` | Definir o grupo de vários itens: Certificado |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | Ver segredos em texto puro |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | Emitir ou renovar o certificado imediatamente |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | Baixar certificado e chave privada |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | Implantar um certificado agora |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | Listar destinos de implantação disponíveis |
| GET | `/api/ssl/providers` | `ssl:read` | Listar provedores de validação DNS suportados |
| GET | `/api/ssl/settings` | `ssl:read` | Ler configurações globais de certificados |
| PUT | `/api/ssl/settings` | `ssl:write` | Salvar configurações globais de certificados |

## Firewall WAF

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | Ver logs de execução do módulo |
| DELETE | `/api/waf/logs` | `waf:write` | Limpar logs de execução do módulo |
| GET | `/api/waf/instances` | `waf:read` | Listar Instância WAF |
| POST | `/api/waf/instances` | `waf:write` | Adicionar Instância WAF |
| PUT | `/api/waf/instances/{key}` | `waf:write` | Modificar Instância WAF |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | Excluir Instância WAF |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | Ativar ou desativar Instância WAF |
| PUT | `/api/waf/instances-order` | `waf:write` | Reordenar Instância WAF |
| PUT | `/api/waf/instances-group` | `waf:write` | Definir o grupo de vários itens: Instância WAF |
| GET | `/api/waf/crs` | `waf:read` | Listar arquivos de regras OWASP CRS |
| GET | `/api/waf/crs/{file}` | `waf:read` | Ler um arquivo de regras CRS |

## Serviço web

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | Ver logs de execução do módulo |
| DELETE | `/api/webservice/logs` | `webservice:write` | Limpar logs de execução do módulo |
| GET | `/api/webservice/listeners` | `webservice:read` | Listar Escuta do serviço web |
| POST | `/api/webservice/listeners` | `webservice:write` | Adicionar Escuta do serviço web |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Modificar Escuta do serviço web |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Excluir Escuta do serviço web |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Ativar ou desativar Escuta do serviço web |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Reordenar Escuta do serviço web |
| GET | `/api/webservice/iplists` | `webservice:read` | Listar as listas de IP assinadas |
| POST | `/api/webservice/iplists` | `webservice:write` | Adicionar Lista de IP assinada |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | Modificar Lista de IP assinada |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | Excluir Lista de IP assinada |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | Ativar ou desativar Lista de IP assinada |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | Reordenar Lista de IP assinada |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | Atualizar agora uma lista de IP assinada |
| GET | `/api/webservice/users` | `webservice:read` | Ler usuários compartilhados e provedor de identidade OIDC |
| PUT | `/api/webservice/users` | `webservice:write` | Salvar usuários compartilhados e provedor de identidade OIDC, configurar sites em lote |
| POST | `/api/webservice/import/nginx` | `webservice:write` | Importar configuração do nginx |
| GET | `/api/webservice/settings` | `webservice:read` | Ler configurações globais do serviço web |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | Ver segredos TOTP dos usuários compartilhados em texto puro (por nome de usuário) |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | Ver Client Secrets dos logins de terceiros em texto puro (por identificador do método) |
| POST | `/api/webservice/totp-secret` | `webservice:write` | Gerar nova chave TOTP para um usuário compartilhado (não salva; vale após salvar as configurações) |
| PUT | `/api/webservice/settings` | `webservice:write` | Salvar configurações globais do serviço web |
| GET | `/api/webservice/stats` | `webservice:read` | Estatísticas por minuto do site (?listener=&site=&minutes=) |
| GET | `/api/webservice/metrics` | `webservice:read` | Métricas em formato de texto do Prometheus |
| GET | `/api/webservice/bans` | `webservice:read` | IPs atualmente bloqueados |
| POST | `/api/webservice/bans` | `webservice:write` | Bloquear IP manualmente |
| DELETE | `/api/webservice/bans` | `webservice:write` | Desbloquear (body é {"key": endereço}) |
| GET | `/api/webservice/geoip` | `webservice:read` | Status do banco de dados GeoIP |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | Enviar banco de dados GeoIP (campo multipart file, .mmdb) |
| POST | `/api/webservice/geoip/download` | `webservice:write` | Baixar o banco de dados GeoIP (.mmdb) a partir de um endereço |
| DELETE | `/api/webservice/geoip` | `webservice:write` | Remover banco de dados GeoIP |

## Prevenção de intrusão

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | Ver logs de execução do módulo |
| DELETE | `/api/guard/logs` | `guard:write` | Limpar logs de execução do módulo |
| GET | `/api/guard/jails` | `guard:read` | Listar regra de prevenção de intrusões |
| POST | `/api/guard/jails` | `guard:write` | Adicionar regra de prevenção de intrusões |
| PUT | `/api/guard/jails/{key}` | `guard:write` | Modificar regra de prevenção de intrusões |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | Excluir regra de prevenção de intrusões |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | Ativar ou desativar regra de prevenção de intrusões |
| PUT | `/api/guard/jails-order` | `guard:write` | Reordenar regra de prevenção de intrusões |
| PUT | `/api/guard/jails-group` | `guard:write` | Definir o grupo de vários itens: regra de prevenção de intrusões |
| GET | `/api/guard/presets` | `guard:read` | Modelos de regras para serviços comuns |
| POST | `/api/guard/jails/test` | `guard:write` | Testar filtros com linhas de log de exemplo |
| GET | `/api/guard/status` | `guard:read` | Status da sincronização do firewall e ambiente |
| GET | `/api/guard/settings` | `guard:read` | Configurações globais |
| PUT | `/api/guard/settings` | `guard:write` | Alterar configurações globais |
| GET | `/api/guard/bans` | `guard:read` | IPs bloqueados no momento |
| POST | `/api/guard/bans` | `guard:write` | Bloquear IP ou rede manualmente |
| DELETE | `/api/guard/bans` | `guard:write` | Desbloquear (body é {"key": endereço}) |
| GET | `/api/guard/firewall/rules` | `guard:read` | Regras que o Gately gravou no nftables (texto) |

## Wake-on-LAN

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | Ver logs de execução do módulo |
| DELETE | `/api/wol/logs` | `wol:write` | Limpar logs de execução do módulo |
| GET | `/api/wol/devices` | `wol:read` | Listar Despertar dispositivo |
| POST | `/api/wol/devices` | `wol:write` | Adicionar Despertar dispositivo |
| PUT | `/api/wol/devices/{key}` | `wol:write` | Modificar Despertar dispositivo |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | Excluir Despertar dispositivo |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | Ativar ou desativar Despertar dispositivo |
| PUT | `/api/wol/devices-order` | `wol:write` | Reordenar Despertar dispositivo |
| PUT | `/api/wol/devices-group` | `wol:write` | Definir o grupo de vários itens: Despertar dispositivo |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | Enviar o pacote mágico de despertar ao dispositivo |
| GET | `/api/wol/status` | `wol:read` | Ver o estado online do dispositivo |

## Terminal web

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | Ver logs de execução do módulo |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | Limpar logs de execução do módulo |
| GET | `/api/webterminal/connections` | `webterminal:read` | Listar Conexão de terminal |
| POST | `/api/webterminal/connections` | `webterminal:write` | Adicionar Conexão de terminal |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | Modificar Conexão de terminal |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | Excluir Conexão de terminal |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | Ativar ou desativar Conexão de terminal |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | Reordenar Conexão de terminal |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | Definir o grupo de vários itens: Conexão de terminal |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | Ver segredos em texto puro |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | Limpar chaves de host registradas |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | Testar conexão salva |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | Testar parâmetros de conexão temporários |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | Atalhos de conexão |
| GET | `/api/webterminal/credentials` | `webterminal:read` | Listar Entrada |
| POST | `/api/webterminal/credentials` | `webterminal:write` | Adicionar Entrada |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | Modificar Entrada |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | Excluir Entrada |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | Ativar ou desativar Entrada |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | Reordenar Entrada |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | Ver segredos em texto puro |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | Abrir sessão de terminal (WebSocket) |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | Anexar a uma sessão de terminal existente (WebSocket) |
| GET | `/api/webterminal/sessions` | `webterminal:read` | Listar sessões de terminal |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | Ver detalhes da sessão de terminal |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | Modificar a observação da sessão de terminal |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | Ver estatísticas das sessões de terminal |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | Fechar sessão de terminal |
| GET | `/api/webterminal/security` | Somente sessão de login | Ver o estado das configurações de segurança do terminal |
| POST | `/api/webterminal/security/agreement` | Somente sessão de login | Confirmar o contrato de uso do terminal |
| POST | `/api/webterminal/security/2fa/setup` | Somente sessão de login | Gerar chave de verificação em duas etapas do terminal |
| POST | `/api/webterminal/security/2fa/enable` | Somente sessão de login | Ativar verificação em duas etapas do terminal |
| POST | `/api/webterminal/security/2fa/disable` | Somente sessão de login | Desativar verificação em duas etapas do terminal |
| POST | `/api/webterminal/security/verify2fa` | Somente sessão de login | Verificar o código de verificação em duas etapas do terminal |
| GET | `/api/webterminal/security/check2fa` | Somente sessão de login | Verificar se a verificação em duas etapas do terminal foi aprovada |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | Listar diretório remoto |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | Ler arquivo remoto |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | Gravar arquivo remoto |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | Enviar arquivo para o host remoto |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | Criar diretório remoto |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | Criar arquivo vazio remoto |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | Renomear ou mover arquivo remoto |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | Copiar arquivo remoto |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | Modificar permissões de arquivo remoto |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | Excluir arquivo remoto |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | Compactar arquivos no host remoto |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | Extrair arquivos no host remoto |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | Pré-visualizar o conteúdo de um pacote compactado remoto |
| GET | `/api/webterminal/settings` | `webterminal:read` | Ler configurações do terminal web |
| PUT | `/api/webterminal/settings` | `webterminal:write` | Salvar configurações do terminal web |
| GET | `/api/webterminal/shells` | `webterminal:read` | Listar shells disponíveis nesta máquina |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | Ler comandos rápidos globais |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | Salvar comandos rápidos globais |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | Ler layout de painéis divididos |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | Salvar layout de painéis divididos |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | Limpar layout de painéis divididos |

## Tarefa agendada

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | Ver logs de execução do módulo |
| DELETE | `/api/cron/logs` | `cron:write` | Limpar logs de execução do módulo |
| GET | `/api/cron/tasks` | `cron:read` | Listar Tarefa agendada |
| POST | `/api/cron/tasks` | `cron:write` | Adicionar Tarefa agendada |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | Modificar Tarefa agendada |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | Excluir Tarefa agendada |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | Ativar ou desativar Tarefa agendada |
| PUT | `/api/cron/tasks-order` | `cron:write` | Reordenar Tarefa agendada |
| PUT | `/api/cron/tasks-group` | `cron:write` | Definir o grupo de vários itens: Tarefa agendada |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | Executar uma tarefa agendada imediatamente |
| POST | `/api/cron/parse` | `cron:write` | Analisar e pré-visualizar expressão Cron |

## Gerenciamento do Docker

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | Ver logs de execução do módulo |
| DELETE | `/api/docker/logs` | `docker:write` | Limpar logs de execução do módulo |
| GET | `/api/docker/tasks` | `docker:read` | Listar tarefas em segundo plano |
| GET | `/api/docker/tasks/{id}` | `docker:read` | Ver progresso e saída das tarefas em segundo plano |
| DELETE | `/api/docker/tasks` | `docker:write` | Limpar tarefas em segundo plano já encerradas |
| GET | `/api/docker/images` | `docker:read` | Listar imagens |
| POST | `/api/docker/images/pull` | `docker:write` | Baixar imagem |
| POST | `/api/docker/images/push` | `docker:write` | Enviar imagem |
| POST | `/api/docker/images/search` | `docker:write` | Pesquisar imagens |
| POST | `/api/docker/images/load` | `docker:write` | Importar arquivo de imagem |
| POST | `/api/docker/images/build` | `docker:write` | Compilar imagem |
| GET | `/api/docker/images/{id}` | `docker:read` | Ver detalhes da imagem |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | Marcar imagem com tag |
| DELETE | `/api/docker/images/{id}` | `docker:write` | Excluir imagem |
| GET | `/api/docker/images/{id}/export` | `docker:write` | Exportar imagem |
| GET | `/api/docker/networks` | `docker:read` | Listar redes |
| POST | `/api/docker/networks` | `docker:write` | Criar rede |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | Excluir rede |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | Conectar o contêiner à rede |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | Desconectar o contêiner da rede |
| GET | `/api/docker/volumes` | `docker:read` | Listar volumes |
| POST | `/api/docker/volumes` | `docker:write` | Criar volume de dados |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | Excluir volume de dados |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | Exportar volume de dados |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | Importar dados para o volume de dados |
| GET | `/api/docker/compose/projects` | `docker:read` | Listar projetos Compose |
| POST | `/api/docker/compose/projects` | `docker:write` | Criar projeto Compose |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | Iniciar projeto Compose |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | Baixar imagens do projeto Compose |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | Executar operação no projeto Compose |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | Ler arquivo de configuração do Compose |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | Salvar arquivo de configuração do Compose |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | Ver logs do projeto Compose |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | Excluir projeto Compose |
| GET | `/api/docker/info` | `docker:read` | Ver informações do mecanismo Docker |
| GET | `/api/docker/disk-usage` | `docker:read` | Ver uso de disco do Docker |
| POST | `/api/docker/prune` | `docker:write` | Limpar recursos Docker não utilizados |
| GET | `/api/docker/settings` | `docker:read` | Ler configurações do módulo Docker |
| PUT | `/api/docker/settings` | `docker:write` | Salvar configurações do módulo Docker |
| GET | `/api/docker/settings/secrets` | `docker:write` | Ver senhas dos registros em texto puro (chaves `registry:<endereço do registro>`) |
| GET | `/api/docker/containers` | `docker:read` | Listar contêineres |
| POST | `/api/docker/containers` | `docker:write` | Criar contêiner |
| POST | `/api/docker/containers/check-all` | `docker:write` | Verificar atualizações de todas as imagens de contêineres |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | Atualizar contêineres em lote |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | Verificar se há nova versão da imagem |
| GET | `/api/docker/containers/{id}` | `docker:read` | Ver detalhes do contêiner |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | Ver em texto puro as variáveis de ambiente e o comando de inicialização do contêiner |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | Excluir contêiner |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | Modificar a configuração do contêiner e recriá-lo |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | Renomear contêiner |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | Confirmar o contêiner como imagem |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | Verificar se há atualização para a imagem do contêiner |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | Atualizar a imagem do contêiner e recriá-lo |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | Executar operação no contêiner |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | Ver logs do contêiner |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | Ver uso de recursos do contêiner |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | Ver processos dentro do contêiner |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | Listar diretório dentro do contêiner |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | Ler arquivo dentro do contêiner |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | Baixar arquivo dentro do contêiner |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | Gravar arquivo dentro do contêiner |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | Criar diretório dentro do contêiner |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | Renomear arquivo dentro do contêiner |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | Enviar arquivo para o contêiner |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | Excluir arquivo dentro do contêiner |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | Abrir terminal dentro do contêiner (WebSocket) |

## Serviço WebDAV

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | Ver logs de execução do módulo |
| DELETE | `/api/webdav/logs` | `webdav:write` | Limpar logs de execução do módulo |
| GET | `/api/webdav/settings` | `webdav:read` | Ler configurações do WebDAV |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | Ver senhas dos usuários locais em texto puro |
| GET | `/api/webdav/shared-users` | `webdav:read` | Ler os usuários compartilhados dos serviços de arquivos (endpoint antigo) |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | Salvar os usuários compartilhados dos serviços de arquivos (endpoint antigo) |
| PUT | `/api/webdav/settings` | `webdav:write` | Salvar configurações do WebDAV |

## Serviço FTP

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | Ver logs de execução do módulo |
| DELETE | `/api/ftp/logs` | `ftp:write` | Limpar logs de execução do módulo |
| GET | `/api/ftp/settings` | `ftp:read` | Ler configurações do FTP |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | Ver senhas dos usuários locais em texto puro |
| GET | `/api/ftp/shared-users` | `ftp:read` | Ler os usuários compartilhados dos serviços de arquivos (endpoint antigo) |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | Salvar os usuários compartilhados dos serviços de arquivos (endpoint antigo) |
| PUT | `/api/ftp/settings` | `ftp:write` | Salvar configurações do FTP |

## Serviço SMB

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | Ver logs de execução do módulo |
| DELETE | `/api/smb/logs` | `smb:write` | Limpar logs de execução do módulo |
| GET | `/api/smb/settings` | `smb:read` | Obter configurações do SMB |
| GET | `/api/smb/settings/secrets` | `smb:write` | Ver senhas dos usuários locais em texto puro |
| GET | `/api/smb/shared-users` | `smb:read` | Ler os usuários compartilhados dos serviços de arquivos (endpoint antigo) |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | Salvar os usuários compartilhados dos serviços de arquivos (endpoint antigo) |
| PUT | `/api/smb/settings` | `smb:write` | Salvar configurações do SMB |

## File Browser

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | Ver logs de execução do módulo |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | Limpar logs de execução do módulo |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | Listar Instância do File Browser |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | Adicionar Instância do File Browser |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Modificar Instância do File Browser |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Excluir Instância do File Browser |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | Ativar ou desativar Instância do File Browser |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | Reordenar Instância do File Browser |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | Definir o grupo de vários itens: Instância do File Browser |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | Ver segredos em texto puro |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | Reiniciar uma instância do File Browser |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | Ler as configurações do programa File Browser |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | Salvar as configurações do programa File Browser |
| GET | `/api/filebrowser/install` | `filebrowser:read` | Ver as informações de instalação e o progresso de um programa gerenciado |
| POST | `/api/filebrowser/install` | `filebrowser:write` | Instalar ou atualizar online um programa gerenciado |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | Mudar para uma versão anterior salva |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | Desinstalar um programa gerenciado |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | Excluir uma versão anterior salva |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | Restaurar as configurações padrão do módulo |

## Montagens de armazenamento

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | Ver logs de execução do módulo |
| DELETE | `/api/storage/logs` | `storage:write` | Limpar logs de execução do módulo |
| GET | `/api/storage/mounts` | `storage:read` | Listar Montagens de armazenamento |
| POST | `/api/storage/mounts` | `storage:write` | Adicionar Montagens de armazenamento |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | Modificar Montagens de armazenamento |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | Excluir Montagens de armazenamento |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | Ativar ou desativar Montagens de armazenamento |
| PUT | `/api/storage/mounts-order` | `storage:write` | Reordenar Montagens de armazenamento |
| PUT | `/api/storage/mounts-group` | `storage:write` | Definir o grupo de vários itens: Montagens de armazenamento |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | Ver segredos em texto puro |
| GET | `/api/storage/files/{key}/list` | `storage:write` | Listar diretório de armazenamento |
| GET | `/api/storage/files/{key}/download` | `storage:write` | Baixar arquivo do armazenamento |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | Enviar arquivo para o armazenamento |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | Criar diretório de armazenamento |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | Renomear ou mover arquivo do armazenamento |
| DELETE | `/api/storage/files/{key}` | `storage:write` | Excluir arquivo do armazenamento |

## Central de notificações

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | Ver logs de execução do módulo |
| DELETE | `/api/notify/logs` | `notify:write` | Limpar logs de execução do módulo |
| GET | `/api/notify/channels` | `notify:read` | Listar Canais de notificação |
| POST | `/api/notify/channels` | `notify:write` | Adicionar Canais de notificação |
| PUT | `/api/notify/channels/{key}` | `notify:write` | Modificar Canais de notificação |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | Excluir Canais de notificação |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | Ativar ou desativar Canais de notificação |
| PUT | `/api/notify/channels-order` | `notify:write` | Reordenar Canais de notificação |
| PUT | `/api/notify/channels-group` | `notify:write` | Definir o grupo de vários itens: Canais de notificação |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | Ver segredos em texto puro |
| GET | `/api/notify/rules` | `notify:read` | Listar Regras de notificação |
| POST | `/api/notify/rules` | `notify:write` | Adicionar Regras de notificação |
| PUT | `/api/notify/rules/{key}` | `notify:write` | Modificar Regras de notificação |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | Excluir Regras de notificação |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | Ativar ou desativar Regras de notificação |
| PUT | `/api/notify/rules-order` | `notify:write` | Reordenar Regras de notificação |
| PUT | `/api/notify/rules-group` | `notify:write` | Definir o grupo de vários itens: Regras de notificação |
| GET | `/api/notify/event-types` | `notify:read` | Tipos de evento que podem ser assinados |
| GET | `/api/notify/history` | `notify:read` | Registros de entrega recentes |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | Enviar notificação de teste ao canal |

## Diagnóstico de rede

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | Ver logs de execução do módulo |
| DELETE | `/api/nettools/logs` | `nettools:write` | Limpar logs de execução do módulo |
| POST | `/api/nettools/run` | `nettools:write` | Executar ferramenta de diagnóstico e retornar a tarefa (tool: ping / traceroute / tcp / dns / speed / scan) |
| GET | `/api/nettools/tasks` | `nettools:read` | Tarefas de diagnóstico recentes |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | Detalhes e saída da tarefa |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | Cancelar a tarefa em execução ou excluir uma tarefa já encerrada |
| GET | `/api/nettools/settings` | `nettools:read` | Configurações de diagnóstico |
| PUT | `/api/nettools/settings` | `nettools:write` | Modificar a lista de sondagem proibida |
| GET | `/api/nettools/networks` | `nettools:read` | Faixas de rede desta máquina disponíveis para varredura da rede local |

## Backup de configuração

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | Ver logs de execução do módulo |
| DELETE | `/api/backup/logs` | `backup:write` | Limpar logs de execução do módulo |
| GET | `/api/backup/targets` | `backup:read` | Listar Destino do backup |
| POST | `/api/backup/targets` | `backup:write` | Adicionar Destino do backup |
| PUT | `/api/backup/targets/{key}` | `backup:write` | Modificar Destino do backup |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | Excluir Destino do backup |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | Ativar ou desativar Destino do backup |
| PUT | `/api/backup/targets-order` | `backup:write` | Reordenar Destino do backup |
| GET | `/api/backup/settings` | `backup:read` | Configurações de backup |
| PUT | `/api/backup/settings` | `backup:write` | Modificar configurações de backup (enviar o marcador no lugar da senha mantém o valor atual) |
| GET | `/api/backup/secrets` | `backup:write` | Ver segredos em texto puro |
| POST | `/api/backup/run` | `backup:write` | Fazer um backup agora |
| GET | `/api/backup/status` | `backup:read` | Resultado do último backup e horário da próxima execução |
| GET | `/api/backup/mounts` | `backup:read` | Pontos de montagem de armazenamento que podem ser destino de backup |
| GET | `/api/backup/local` | Somente sessão de login | Lista de arquivos de backup locais |
| GET | `/api/backup/local/{name}` | Somente sessão de login | Baixar arquivo de backup local |

## Usuários compartilhados dos serviços de arquivos

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | Ler os usuários compartilhados dos serviços de arquivos |
| PUT | `/api/fileusers` | `fileusers:write` | Salvar os usuários compartilhados dos serviços de arquivos |
| GET | `/api/fileusers/secrets` | `fileusers:write` | Ver senhas dos usuários compartilhados em texto puro |

## Sistema

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/session` | Qualquer token válido | Ver a identidade autenticada atual |
| GET | `/api/status` | `status:read` | Ver o estado do sistema e dos módulos |
| GET | `/api/logs` | `logs:read` | Ver logs do sistema |
| GET | `/api/system/settings` | Somente sessão de login | Ler configurações globais |
| PUT | `/api/system/settings` | Somente sessão de login | Salvar configurações globais |
| GET | `/api/system/settings/secrets` | Somente sessão de login | Ver Client Secrets do login de terceiros em texto puro |
| GET | `/api/system/info` | `status:read` | Ver informações do sistema |
| GET | `/api/system/update` | `status:read` | Verificar nova versão |
| POST | `/api/system/upgrade` | Somente sessão de login | Atualizar online para a versão mais recente |
| GET | `/api/system/upgrade/status` | Somente sessão de login | Ver o progresso da atualização online |
| POST | `/api/system/upgrade/cancel` | Somente sessão de login | Cancelar a atualização online |
| POST | `/api/system/upgrade/upload` | Somente sessão de login | Atualizar com um pacote enviado |
| GET | `/api/system/backup` | Somente sessão de login | Baixar backup da configuração |
| GET | `/api/system/diagnostics` | Somente sessão de login | Exportar pacote de diagnóstico anonimizado |
| GET | `/api/audit` | Somente sessão de login | Consultar o log de auditoria |
| GET | `/api/metrics` | `metrics:read` | Métricas globais do Prometheus |
| GET | `/api/history` | Somente sessão de login | Listar versões do histórico de configuração |
| GET | `/api/history/{name}/{id}/diff` | Somente sessão de login | Comparar uma versão do histórico com a configuração atual |
| POST | `/api/history/{name}/{id}/restore` | Somente sessão de login | Reverter uma configuração para uma versão do histórico |
| POST | `/api/system/restore` | Somente sessão de login | Restaurar a configuração a partir do backup |
| POST | `/api/system/restart` | Somente sessão de login | Reiniciar o Gately |
| POST | `/api/system/selfcheck` | Somente sessão de login | Autoverificação de rede e configuração |
| GET | `/api/system/onboarding` | Somente sessão de login | Se a configuração inicial está pendente |
| POST | `/api/system/onboarding/dismiss` | Somente sessão de login | Concluir ou pular a configuração inicial |
| GET | `/api/openapi.json` | Qualquer token válido | Obter a descrição OpenAPI desta API |

## Importação de configuração

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | Importar configuração do Lucky |

## Token de API

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| GET | `/api/tokens` | Somente sessão de login | Listar tokens de API |
| GET | `/api/tokens/scopes` | Somente sessão de login | Listar domínios de permissão autorizáveis |
| POST | `/api/tokens` | Somente sessão de login | Criar token de API |
| PUT | `/api/tokens/order` | Somente sessão de login | Reordenar tokens de API |
| PUT | `/api/tokens/group` | Somente sessão de login | Definir o grupo de vários tokens de API |
| PUT | `/api/tokens/{id}` | Somente sessão de login | Alterar nome, grupo, permissões, expiração e estado de um token de API |
| POST | `/api/tokens/{id}/regenerate` | Somente sessão de login | Regenerar o texto simples do token |
| DELETE | `/api/tokens/{id}` | Somente sessão de login | Excluir token de API |

## Conta

| Método | Caminho | Permissão necessária | Descrição |
| --- | --- | --- | --- |
| PUT | `/api/account` | Somente sessão de login | Modificar usuário e senha do administrador |
| PUT | `/api/account/ui-language` | Somente sessão de login | Registrar o idioma do console de administração |
| GET | `/api/account/passkeys` | Somente sessão de login | Listar Passkeys registradas |
| POST | `/api/account/passkeys/begin` | Somente sessão de login | Iniciar o registro de uma Passkey |
| POST | `/api/account/passkeys` | Somente sessão de login | Concluir o registro da Passkey |
| DELETE | `/api/account/passkeys/{id}` | Somente sessão de login | Excluir uma Passkey |
| GET | `/api/account/users` | Somente sessão de login | Listar contas somente leitura |
| POST | `/api/account/users` | Somente sessão de login | Criar uma conta somente leitura |
| PUT | `/api/account/users/{name}` | Somente sessão de login | Redefinir a senha de uma conta somente leitura |
| DELETE | `/api/account/users/{name}` | Somente sessão de login | Excluir uma conta somente leitura |
| GET | `/api/account/totp` | Somente sessão de login | Ver o estado da verificação em duas etapas |
| POST | `/api/account/totp/setup` | Somente sessão de login | Gerar chave de verificação em duas etapas |
| POST | `/api/account/totp/enable` | Somente sessão de login | Ativar verificação em duas etapas |
| POST | `/api/account/totp/disable` | Somente sessão de login | Desativar verificação em duas etapas |

## Estruturas de dados

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
| `results` | Array de `backup.Result` |
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

### crud.enableRequest

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| Campo | Tipo |
| --- | --- |
| `group` | string |
| `keys` | Array de string |

### crud.orderRequest

| Campo | Tipo |
| --- | --- |
| `keys` | Array de string |

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
| `records` | Array de `ddns.Record` |
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
| `records` | Array de `ddns.recordLayout` |

### dnsprovider.Credentials

| Campo | Tipo |
| --- | --- |
| `extra` | Mapa de string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| Campo | Tipo |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| Campo | Tipo |
| --- | --- |
| `accessUrls` | Array de string |
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
| `sources` | Array de `filebrowser.Source` |
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
| `proxies` | Array de `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | Array de string |

### frp.Proxy

| Campo | Tipo |
| --- | --- |
| `client` | string |
| `customDomains` | Array de string |
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
| `checkedAt` | Hora (RFC 3339) |
| `dropped` | Mapa de integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | Mapa de integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | Array de integer |
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
| `threshold` | integer |

### guard.Settings

| Campo | Tipo |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | Array de string |
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
| `userTrapPorts` | Array de integer |

### guard.TestResult

| Campo | Tipo |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
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
| `urls` | Array de string |

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
| `deny` | Array de string |

### nettools.Task

| Campo | Tipo |
| --- | --- |
| `error` | string |
| `finished` | Hora (RFC 3339) |
| `id` | string |
| `lines` | Array de string |
| `result` | Qualquer |
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
| `channels` | Array de string |
| `enable` | boolean |
| `events` | Array de string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### portforward.Rule

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | Array de string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | Array de string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### protoroute.Route

| Campo | Tipo |
| --- | --- |
| `alpn` | Array de string |
| `domains` | Array de string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | Array de string |
| `targetPort` | integer |

### protoroute.Rule

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | Array de string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | Array de `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | Array de string |
| `targetPorts` | string |

### ssl.ACMEConfig

| Campo | Tipo |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `domains` | Array de string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | Array de string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| Campo | Tipo |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | Array de `ssl.Deploy` |
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
| `containers` | Array de string |
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
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `keepAliveServers` | Array de string |
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
| `stunServers` | Array de string |
| `stunTimeout` | integer |
| `targetAddresses` | Array de string |
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
| `ruleSets` | Array de `waf.RuleSet` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| Campo | Tipo |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### webhook.Config

| Campo | Tipo |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | Array de string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `successContent` | Array de string |
| `timeout` | integer |
| `url` | string |

### webservice.AuthUser

| Campo | Tipo |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | Array de string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

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
| `ttl` | integer |

### webservice.ForwardAuth

| Campo | Tipo |
| --- | --- |
| `copyHeaders` | Array de string |
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
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | Hora (RFC 3339) |
| `url` | string |

### webservice.Listener

| Campo | Tipo |
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
| `sites` | Array de `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| Campo | Tipo |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | Array de string |
| `targets` | Array de string |

### webservice.OIDCClient

| Campo | Tipo |
| --- | --- |
| `allow` | Array de string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | Array de string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| Campo | Tipo |
| --- | --- |
| `clients` | Array de `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.PathRule

| Campo | Tipo |
| --- | --- |
| `auth` | string |
| `ipList` | Array de string |
| `ipMode` | string |
| `ipSets` | Array de string |
| `prefix` | string |

### webservice.Point

| Campo | Tipo |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.RateLimit

| Campo | Tipo |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

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
| `authSecret` | string |
| `authUsers` | Array de `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | Array de `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | Array de string |

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
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | Array de string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | Array de string |
| `enable` | boolean |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `insecureSkipVerify` | boolean |
| `ipList` | Array de string |
| `ipMode` | string |
| `ipSets` | Array de string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | Array de `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | Array de string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | Array de string |
| `responseHeaders` | Array de string |
| `rewrites` | Array de `webservice.Rewrite` |
| `root` | string |
| `rules` | Array de `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | Array de `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | Array de string |
| `text` | string |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | Array de `webservice.UserRoute` |
| `users` | Array de string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteSharedUsers

| Campo | Tipo |
| --- | --- |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| Campo | Tipo |
| --- | --- |
| `bytesOut` | integer |
| `series` | Array de `webservice.Point` |
| `status` | Mapa de integer |
| `topCountries` | Array de `webservice.TopItem` |
| `topIps` | Array de `webservice.TopItem` |
| `upstreams` | Array de `webservice.UpstreamStatus` |

### webservice.TopItem

| Campo | Tipo |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.UpstreamStatus

| Campo | Tipo |
| --- | --- |
| `backup` | boolean |
| `conns` | integer |
| `healthy` | boolean |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| Campo | Tipo |
| --- | --- |
| `targets` | Array de string |
| `users` | Array de string |

### webservice.UserSite

| Campo | Tipo |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | Array de string |
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
| `authUsers` | Array de `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Array de `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| Campo | Tipo |
| --- | --- |
| `authUsers` | Array de `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Array de `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| Campo | Tipo |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.geoipDownload

| Campo | Tipo |
| --- | --- |
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
| `quickAccessDirs` | Array de string |
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

### wol.Device

| Campo | Tipo |
| --- | --- |
| `broadcastIps` | Array de string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | Array de string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |

