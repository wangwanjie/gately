# Migrar do Lucky para o Gately

> O Gately é um projeto independente, sem afiliação com o projeto Lucky. A compatibilidade se baseia em comportamentos, interfaces e formatos de configuração observáveis publicamente.

O Gately lê diretamente os arquivos de configuração `.lkcf` do Lucky ou o zip do site inteiro exportado por "Backup de configuração" do Lucky, e traz regras, tarefas e certificados para cá. Você não precisa desinstalar o Lucky: os dois podem funcionar lado a lado, com a troca feita módulo por módulo.

::: tip Migrar do nginx
Este guia trata da migração a partir do Lucky. Sites existentes do nginx podem ser importados diretamente em "Serviços Web → Importar nginx"; veja [Serviços Web · Importar configuração do nginx](/pt-BR/docs/modules/web#import-nginx).
:::

## 1. Preparação

1. Localize o diretório de configuração do Lucky: em implantações com Docker, costuma ser o diretório do host mapeado para `/app/conf` no contêiner, com arquivos como `lucky_base.lkcf` e `lucky_ddns.lkcf`.
2. Implante o Gately (veja o README). Recomendações:
   - use `--network host`, escutando diretamente nas portas do host;
   - a porta de administração padrão é a 16680, sem conflito com a 16601 do Lucky;
   - se os scripts ou mapeamentos de certificados do Lucky usam caminhos internos do contêiner (por exemplo `/etc/lucky/custom_scripts`, `/data/nginx/...`), monte-os no contêiner do Gately no **mesmo caminho**, e nada precisará ser alterado após a importação.

## 2. Importação

Abra "Configurações → Backup e migração" no Gately, selecione todos os arquivos `lucky_*.lkcf` ou diretamente o zip de configuração do site inteiro exportado pelo Lucky (os `.lkcf` dentro do pacote são reconhecidos automaticamente e os demais arquivos são ignorados). Clique em "Pré-visualizar" para ver quantos itens cada módulo consegue importar e quais avisos existem; depois de conferir, clique em "Importar".

Importar mais de uma vez é seguro: regras, tarefas e certificados com o mesmo nome de itens já existentes são ignorados.

Arquivos compatíveis:

| Arquivo | Importado para | Observações |
| --- | --- | --- |
| lucky_portforward.lkcf | Redirecionamento de portas | Criptografia TLS/de fluxo e opções de limitação de velocidade ainda não são compatíveis e geram aviso; o modo de segurança vira controle de acesso por IP de origem (envie `lucky_ipfilter.lkcf` na mesma importação) |
| lucky_ddns.lkcf | DNS dinâmico | Compatível com 11 provedores; os endereços oficiais de consulta de IP do Lucky são removidos |
| lucky_stun.lkcf | Travessia STUN | Inclui a lista global de servidores STUN e o webhook global; o modo de segurança do encaminhamento integrado também vira controle de acesso por IP de origem |
| lucky_ssl.lkcf | Certificados SSL | Certificados já emitidos são importados como estão e não são solicitados de novo antes de expirar |
| lucky_reverseproxy.lkcf | Serviços Web | Sites, autenticação e referências a WAF (quando o WAF da sub-regra é main, usa a instância da regra de escuta); variáveis das sub-regras de redirecionamento, como `{host}{path}{args}`, `{port}`, `{1}` e `{STUN_NomeRegra_PORT}`, continuam funcionando |
| lucky_coraza.lkcf | Firewall WAF | Mantém a key da instância, e as referências dos sites continuam válidas |
| lucky_wol.lkcf | Wake-on-LAN | |
| lucky_webterminal.lkcf | Terminal Web | Conexões SSH e Telnet, proxies, `keyFile`, `workDir`, timeouts, variáveis de ambiente, diretórios de acesso rápido, configurações de sessão, atalhos globais, o termo de uso e o segredo do 2FA; a impressão digital do host é registrada na primeira conexão SSH |
| lucky_webdav.lkcf | WebDAV | A lista de montagens do usuário vira vários diretórios, e usuários com o mesmo nome e senha são mesclados; montagens que referenciam o gerenciamento de armazenamento são resolvidas para o diretório local correspondente |
| lucky_ftpserver.lkcf | FTP | Porta de escuta, intervalo de portas passivas, TLS e parâmetros de timeout, além dos usuários (tratados como no WebDAV) |
| lucky_storagemanagement.lkcf | Montagens de armazenamento | Diretórios locais e armazenamento WebDAV; Aliyun Drive, montagens do sistema e proxy de acesso não são migrados |
| lucky_frp.lkcf | frp | A primeira instância de cliente (endereço do servidor, token, protocolo de transporte e proxies) e a primeira instância de servidor; visitantes, proxies de plugin e console não são migrados |
| lucky_cloudflared.lkcf | Cloudflare Tunnel | Túneis que usam token de túnel (com o token de API); clientes Access e túneis com apenas token de API não são migrados |
| lucky_base.lkcf | Configurações globais, Serviços Web | Apenas acesso pela rede local, limite de falhas de login, duração da sessão, fuso horário, DNS personalizado (incluindo a sintaxe doh@), parâmetros de GC e origens confiáveis do protocolo PROXY. Porta de administração, caminho de entrada seguro, conta de administrador e autenticação em dois fatores não são migrados |

As tarefas agendadas, o filtro de IP e as configurações do Docker do Lucky ficam vazios na maioria das configurações e ainda não são migrados; se houver conteúdo, configure manualmente.

**As regras importadas ficam desativadas por padrão**: isso evita que, ao importar em uma máquina de teste, os domínios de produção passem a apontar para endereços errados ou que as regras disputem portas e mapeamentos UPnP do roteador com um Lucky ainda em execução. Quando você tiver certeza de que o Gately está no dispositivo de destino e o Lucky está parado, marque "Manter o estado de ativação do Lucky após a importação", e as regras passarão a funcionar imediatamente conforme o estado de ativação no Lucky (incluindo a chave geral de cada módulo).

## 3. Troca

Recomendamos trocar módulo por módulo, validando cada um antes de passar ao próximo:

1. **Certificados**: ative o certificado no Gately. Ao ativar, ele grava o certificado no diretório mapeado e executa o script de alteração; confirme que o diretório é gravável.
2. **DDNS**: desative a tarefa no Lucky, ative a tarefa correspondente no Gately e clique em "Forçar sincronização" para confirmar que o registro foi atualizado.
3. **Redirecionamento de portas / STUN**: primeiro desative a regra no Lucky (liberando a porta e o mapeamento UPnP) e depois ative no Gately. Após ativar uma regra STUN, a porta pública pode mudar, e scripts e webhooks são notificados automaticamente.
4. **Serviços Web**: desative a regra de escuta do Lucky e ative a do Gately. Sites HTTPS precisam de um certificado ativo no módulo de certificados que cubra o domínio.
5. Só pare o contêiner do Lucky depois de confirmar que tudo está correto.

## 4. Montagens em implantações com Docker

Os scripts e mapeamentos de certificados do Lucky usam caminhos dentro do contêiner do Lucky, então o contêiner do Gately precisa montar os mesmos diretórios do host nos mesmos caminhos internos. Compare com o compose do Lucky e adicione item por item, por exemplo:

```yaml
volumes:
  - ./config:/etc/gately
  - /var/run/docker.sock:/var/run/docker.sock
  # Se no Lucky o certificado é mapeado para /data/nginx/ssl, monte o mesmo diretório do host no mesmo caminho
  - /srv/nginx/ssl:/data/nginx/ssl
  # Scripts chamados pelas regras (confira se o caminho informado no Lucky realmente existe dentro do contêiner do Lucky)
  - /srv/lucky/custom_scripts:/etc/lucky/custom_scripts:ro
```

A imagem do Gately inclui `sh`, `bash` e `curl`, então os scripts de notificação mais comuns funcionam sem ajustes.

## 5. Itens que exigem tratamento manual

- **Login de terceiros**: o login por GitHub/QQ/Google do Lucky passa pelo servidor intermediário oficial do Lucky; o Gately usa os aplicativos OAuth que você mesmo registra. Na importação, selecione também `lucky_thirdPartyAuthManager.lkcf` (tabela de identidades), `lucky_reverseproxy.lkcf` e `lucky_base.lkcf`:
  - As identidades de terceiros referenciadas por sites e pelo painel de administração são migradas como lista de permissão, de acordo com a tabela de identidades (ID numérico no GitHub, sub no Google, openid no QQ; nome de usuário e e-mail vão para a observação).
  - O método de login do painel de administração fica em "Configurações → Conta e segurança → Login de terceiros", e o dos sites em "Serviços Web → Configurações globais"; ele só vale depois que você informar o seu próprio Client ID/Secret e a URL de callback e ativar. A importação não ativa nada automaticamente. Os identificadores dos métodos de login são `github`, `google` e `qq`, para corresponderem às identidades migradas.
  - O openid do QQ é diferente para cada aplicativo do QQ Connect: ao trocar para o seu aplicativo, o primeiro login será recusado e exibirá o novo identificador da conta; basta adicioná-lo à lista de permissão.
  - Sites que dependem apenas de login de terceiros e para os quais não há identidade migrável permanecem desativados após a importação.
- **Certificados ZeroSSL**: usam as credenciais EAB do Lucky para registrar uma nova conta ACME; se o ZeroSSL recusar, gere novamente as EAB no painel do ZeroSSL.
- **Instâncias frp**: o Gately tem apenas um frpc e um frps; as instâncias excedentes do Lucky são listadas no relatório, então use outra máquina ou mescle em uma única instância. Um frpc já configurado no Gately para outro servidor, ou um frps já ativado, não é sobrescrito. A conta do console do frps é gerenciada pelo Lucky; se precisar do console após a migração, defina novamente a porta e a senha.
- **Referências de armazenamento**: usuários de FTP / WebDAV do Lucky podem montar o armazenamento do "Gerenciamento de armazenamento". Na importação, selecione também `lucky_storagemanagement.lkcf` (ou importe diretamente o zip do site inteiro); o armazenamento local é resolvido para o diretório correspondente, enquanto armazenamento WebDAV, Aliyun Drive e montagens rclone ainda não são compatíveis e aparecem como aviso no relatório.
- **Recursos do Lucky ainda não implementados**: DLNA, montagem de armazenamento em nuvem com rclone, Aliyun Drive, banco de dados de IP (`lucky_ipdb.lkcf`) e outros ainda não são migrados e são ignorados na importação.
- **Servidores oficiais do Lucky**: o Gately nunca se conecta a endereços como `66666.host` e `lucky666.cn`; depois da migração, a consulta do IP público usa serviços públicos de terceiros (que podem ser alterados na tarefa de DDNS).

## 6. Rollback

O Gately não modifica nenhum arquivo do Lucky. Para voltar atrás, desative as regras correspondentes no Gately e reative-as no Lucky.
