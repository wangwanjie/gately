# Guia de reforço de segurança

A interface de administração controla DNS, certificados e portas. Percorra a lista de verificação abaixo.

## Interface de administração

- [ ] Após o primeiro acesso, altere a senha do administrador quando solicitado (com a senha padrão ou uma senha fraca, a troca é exigida antes de continuar) e confira a pontuação de segurança em "Configurações → Conta e segurança", resolvendo os itens que perdem pontos
- [ ] Ative a autenticação em dois fatores (TOTP) ou registre uma passkey
- [ ] Depois de registrar uma passkey, você pode desativar o login por senha: a conta de administrador passa a entrar só com passkey ou login de terceiros, e a senha não pode mais ser quebrada por força bruta; se perder todas as passkeys, reative-o com `gately reset-password`
- [ ] Se a porta de administração estiver aberta para a internet, mantenha ativado o "Orçamento de falhas de endereços desconhecidos" (padrão: 20 falhas em 10 minutos) e troque o nome de usuário do administrador por um nome diferente de admin e difícil de adivinhar
- [ ] Quando familiares ou colegas só precisarem visualizar, crie para eles uma conta somente leitura em vez de compartilhar a senha do administrador
- [ ] Defina um caminho de entrada seguro, para que a interface de administração só seja acessível por um caminho personalizado
- [ ] Ative o "Acesso apenas pela rede local" e use uma VPN (como o [Tailscale](/pt-BR/docs/modules/tunnels)) quando precisar administrar remotamente
- [ ] Se a porta do console estiver acessível pela internet, mude "Configurações → Controle de acesso → Resposta não autorizada" para página 404 simples, página em branco, fechar a conexão, sem resposta ou página personalizada, para que visitantes não autorizados não percebam que é o Gately; uma varredura de portas ainda encontra a porta, então oculte-a por completo com um firewall ou removendo o redirecionamento
- [ ] Ative o HTTPS na interface de administração ao acessá-la por redes não confiáveis
- [ ] Dê aos scripts de automação e à [CLI / MCP](/pt-BR/docs/guide/cli-mcp) [tokens de API](/pt-BR/docs/reference/api) com privilégios mínimos e data de expiração, e evite compartilhar a senha do administrador
- [ ] Defina uma senha de criptografia para os [backups da configuração](/pt-BR/docs/modules/backup) e ative os backups agendados
- [ ] Configure notificações na [Central de notificações](/pt-BR/docs/modules/notify) com os modelos "Segurança e logins", "Alterações de configuração" e "Falhas críticas"
- [ ] Se a interface de administração for exposta por um proxy reverso neste mesmo host, confirme que o [log de auditoria](/pt-BR/docs/guide/audit) registra a origem real; nas entradas de redirecionamento de portas, ative o [protocolo PROXY](/pt-BR/docs/modules/portforward#protocolo-proxy)
- [ ] Depois de baixar uma nova versão, confira as assinaturas conforme [Verificar versões](/pt-BR/docs/guide/verify)
- [ ] Desative em "Configurações → Módulos" os módulos que você não usa, para reduzir os serviços em escuta e os endpoints disponíveis
- [ ] No fnOS, ative "Entrar com o fnOS" somente se você confia em todas as contas de administrador do fnOS; com a opção ativada, uma conta de administrador do fnOS comprometida equivale a uma conta de administrador do Gately comprometida

## Serviços expostos à internet

- Ao expor serviços da rede local por meio de [Serviços Web](/pt-BR/docs/modules/web), ative o HTTPS e combine com autenticação básica, autenticação web ou login de terceiros;
- Ative o [WAF](/pt-BR/docs/modules/waf) nos sites públicos: comece no modo "apenas detectar" para observar falsos positivos e depois mude para "bloquear";
- Restrinja as origens com listas de IPs permitidos e bloqueados, filtro de países por GeoIP e banimento automático após falhas de autenticação.

## Docker

Montar o `docker.sock` equivale a conceder acesso de nível root ao host. Remova a montagem quando não precisar do [Gerenciamento do Docker](/pt-BR/docs/modules/docker).

## Terminal Web

O shell local fica desativado por padrão. Ative-o somente quando realmente necessário e confirme que a autenticação em dois fatores está ativa na interface de administração.

## Backup da configuração

Mantenha o diretório de configuração em um repositório git privado ou faça backup com regularidade. O `ssl.json` contém chaves privadas de certificados, então guarde-o com cuidado.
