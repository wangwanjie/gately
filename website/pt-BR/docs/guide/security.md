# Modelo de segurança e compromisso de privacidade

O Gately roda na borda da rede e lida com chaves de provedores de DNS, chaves privadas de certificados e outras credenciais, por isso segurança e transparência são os seus principais objetivos de design.

## Compromisso de privacidade

- **Zero conexões externas por padrão**: nenhuma conexão com servidores do projeto, nenhum envio de informações do dispositivo, nenhum envio remoto de configuração.
- **Endereços externos visíveis e editáveis**: a consulta do IP público, os servidores STUN e endereços semelhantes aparecem na interface e podem ser alterados ou desativados.
- **Login de terceiros sob o seu controle**: o login com GitHub, Google, QQ e OIDC usa aplicativos OAuth que você mesmo registra, sem intermediários.
- **Atualizações sob o seu controle**: o programa nunca se baixa nem se substitui em segundo plano. A atualização online das instalações binárias no Linux só roda depois que um administrador clica, confere o SHA-256 e guarda a versão anterior; você decide quando atualizar.

## Proteção de dados

- A configuração é JSON em texto simples com permissão 0600, podendo ser auditada e revertida.
- Campos sensíveis são mascarados na saída da API. O botão de olho na interface busca o texto puro salvo sob demanda e exige permissão de escrita. Senhas de usuários web armazenadas como hash bcrypt, segredos de clientes OIDC e chaves privadas nunca são retornados; usuários compartilhados com login em dois fatores podem exibir o QR code novamente. Variáveis de ambiente, comandos de inicialização e linhas de comando de processos dos contêineres Docker são tratados da mesma forma: sem permissão de escrita do Docker, aparecem como `********`.
- O token do cloudflared é passado por variável de ambiente e não aparece na lista de processos; tokens e chaves do frp e do Tailscale são armazenados com permissão 0600.

## Autenticação e controle de acesso

- Hash de senhas com bcrypt e tokens de sessão assinados com HMAC;
- A senha padrão `gately.cn` é pública; enquanto ela estiver em uso, um aviso fixo no topo da interface de administração lembra de alterá-la. Senha esquecida ou autenticador de dois fatores perdido só podem ser redefinidos no servidor com `gately reset-password` / `gately reset-totp` (exige leitura e gravação no diretório de configuração e confirmação digitando `yes`); depois disso, todas as sessões são invalidadas e a operação fica no log de auditoria. Veja [Redefinir a senha e a autenticação em dois fatores](/pt-BR/docs/guide/install#reset); enquanto a senha padrão estiver em uso, a conta, a verificação em duas etapas, as passkeys, as contas adicionais, os tokens de API, as configurações do sistema e as configurações de segurança do terminal web podem ser vistos, mas não alterados (restaurar um backup ou reverter as configurações do sistema ou do terminal web para uma versão do histórico também é recusado), até que a senha padrão seja trocada; a nova senha não pode ser a padrão e, depois de trocada, não pode voltar a ela;
- Login com passkey (WebAuthn): só a chave pública é armazenada, a verificação do usuário (digital, rosto ou PIN) é obrigatória e um contador de assinaturas impede clonagem; exige acesso por domínio com HTTPS (ou localhost);
- Autenticação em dois fatores (TOTP, verificada com os vetores de teste da RFC 6238);
- Contas somente leitura: podem ver o status e a configuração, mas não podem alterar nada, baixar backups, ler segredos ou abrir o terminal; ao redefinir a senha ou excluir a conta, as sessões são invalidadas na hora; quando contas somente leitura e tokens de API apenas com leitura leem a configuração, as senhas dos usuários compartilhados dos sites, os tokens dos links de compartilhamento, os valores dos cabeçalhos de requisição e resposta, os segredos e as senhas e valores de consulta em URLs aparecem como `********`;
- Limitação de tentativas de login;
- Proxy reverso local: quando a interface de administração fica atrás de nginx, haproxy ou de um Serviço Web do Gately no mesmo host, os cabeçalhos `X-Real-IP` / `X-Forwarded-For` só são aceitos de endereços deste host. O acesso apenas pela rede local, a limitação de login e o log de auditoria obtêm a origem real a partir deles. Isso pode ser desativado, ou proxies confiáveis adicionais podem ser incluídos, em "Configurações → Restrições de acesso"; quando os dois cabeçalhos divergem, vale a entrada do `X-Forwarded-For` acrescentada pelo proxy, e se um for um endereço interno e o outro público, o público é usado, de modo que um endereço interno forjado não consegue contornar o acesso somente pela rede local; configure o proxy para gravar os dois cabeçalhos (nginx: `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`);
- Caminho de entrada seguro, acesso apenas pela rede local e HTTPS na interface de administração;
- Gateway unificado do fnOS: quando o pacote do fnOS é acessado pelo gateway unificado do fnOS, o fnOS controla o acesso e somente administradores do fnOS podem abri-lo. "Entrar com o fnOS" vem desativado por padrão; depois de ativado, os administradores que já estão conectados ao fnOS entram como administrador do Gately (sem a verificação em dois fatores), enquanto os usuários comuns do fnOS continuam fazendo login. Veja [Aplicativo nativo para fnOS](/pt-BR/docs/guide/install#fnos);
- Ativação e desativação de módulos: os módulos desativados em "Configurações → Módulos" não iniciam serviços nem oferecem a API de administração, o que reduz a superfície de ataque;
- API com vários tokens: cada token tem acesso de leitura e escrita apenas aos módulos selecionados na criação, pode ter data de expiração e registra o IP de origem do último uso. Os endpoints de conta, autenticação em dois fatores, gerenciamento de tokens, backup, restauração, reinício e configurações do sistema aceitam apenas sessões com login. Veja [Tokens de API e OpenAPI](/pt-BR/docs/reference/api);
- Controle de acesso por site: banimento automático após falhas de autenticação, filtro de países por GeoIP e regras de caminho. Veja [Serviços Web](/pt-BR/docs/modules/web);
- Os backups da configuração aceitam criptografia (scrypt + AES-GCM). Veja [Backup da configuração](/pt-BR/docs/modules/backup).

## Auditoria e rastreabilidade

O log de auditoria registra logins, alterações de configuração e acessos sensíveis, e o histórico de configuração guarda a versão anterior de cada arquivo de configuração antes de ser sobrescrito, com reversão em um clique. Veja [Auditoria e histórico de configuração](/pt-BR/docs/guide/audit). Os artefatos de cada versão vêm com assinaturas cosign, atestados de proveniência de build e SBOM; para conferi-los, veja [Verificar versões](/pt-BR/docs/guide/verify).

## Padrões dos recursos de alto risco

O terminal de shell local fica desativado por padrão. O terminal SSH usa a política trust-on-first-use para registrar as impressões digitais das chaves de host.

## Relatar problemas de segurança

Relate vulnerabilidades de forma privada pela página Security do repositório e evite divulgar detalhes em issues públicas: <https://github.com/wangwanjie/gately/security>

Para mais recomendações práticas, veja o [Guia de reforço de segurança](/pt-BR/docs/guide/hardening).
