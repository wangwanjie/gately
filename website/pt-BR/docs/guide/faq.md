# Perguntas frequentes

## Ele precisa se conectar aos servidores do projeto?

Não. O Gately não se conecta a nenhum servidor do projeto e não envia informações do dispositivo; por padrão, ele consulta anonimamente no GitHub as versões e os avisos de segurança, o que pode ser desativado, veja [Verificação automática de atualizações](/pt-BR/docs/guide/install#update-check). Endereços externos, como a consulta do IP público, usam serviços públicos de terceiros e podem ser alterados na tarefa correspondente.

## Qual é a conta e a senha padrão?

A conta padrão é `admin` e a senha padrão é `gately.cn`. Essa senha é pública: altere-a logo depois de entrar, em "Configurações → Conta e segurança"; até lá, um aviso fica fixo no topo da interface de administração. Se a variável de ambiente `GATELY_INITIAL_PASSWORD` foi definida na implantação, vale a senha dela.

## Esqueci a senha do administrador. O que fazer?

Execute `gately reset-password` no servidor (no Docker, `docker exec -it gately gately reset-password`) e digite `yes` para confirmar: a senha volta para a senha padrão `gately.cn`, e uma instância em execução aplica isso em poucos segundos. Veja [Redefinir a senha e a autenticação em dois fatores](/pt-BR/docs/guide/install#reset).

## Perdi o autenticador de dois fatores. O que fazer?

Execute `gately reset-totp` no servidor (no Docker, `docker exec -it gately gately reset-totp`). Depois de confirmar, a autenticação em dois fatores é desativada; entre com a senha e ative-a de novo.

## Ele é compatível com o fnOS?

Sim. Cada versão inclui pacotes offline para o fnOS (`.fpk`), que você instala manualmente pela Central de aplicativos. O Gately se integra ao gateway unificado do fnOS, então abre pelo ícone na área de trabalho e também funciona pelo seu domínio remoto do fnOS. Ative "Entrar com o fnOS" em "Configurações → Controle de acesso" e os administradores que já estão conectados ao fnOS entram direto, sem senha nem código de verificação em dois fatores. Veja [Aplicativo nativo para fnOS](/pt-BR/docs/guide/install#fnos).

## Ele é compatível com OpenWrt / iStoreOS?

Sim. Cada versão traz pacotes para OpenWrt: o `.ipk` para OpenWrt 24.10 e anteriores (incluindo o iStoreOS), instalado com `opkg install`, e o `.apk` para OpenWrt 25.12 e posteriores, instalado com `apk add --allow-untrusted`. O serviço inicia logo após a instalação e guarda a configuração em `/etc/gately`; para atualizar, basta instalar o pacote novo. O binário tem cerca de 32 MB, então é indicado para soft routers com armazenamento de sobra. Veja [OpenWrt](/pt-BR/docs/guide/install#openwrt).

## Por que `--network host` é recomendado?

Redirecionamento de portas, STUN e serviços Web precisam escutar diretamente nas portas do host. Com a rede bridge, você mesmo precisa mapear cada porta usada.

## De onde vêm os programas de Tailscale, frp e cloudflared?

Esses módulos gerenciam programas externos oficiais, que o Gately não embute. No cartão "Instalação online" da página de cada módulo você baixa a versão mais recente ou uma específica, pode definir um proxy de download próprio e, se a atualização der problema, reverter para uma versão anterior, além de desinstalar o programa ou restaurar os padrões; veja [Instalação, atualização e reversão online](/pt-BR/docs/modules/tunnels#install). No Tailscale a instalação online só está disponível no Linux. Você também pode colocar os programas por conta própria; veja onde em [Configuração e opções de execução](/pt-BR/docs/guide/configuration).

## Com a regra STUN ativa, o acesso externo continua sem funcionar?

Quando a porta WAN do roteador está atrás de um CGNAT da operadora (100.64.0.0/10), as portas abertas pela travessia não são acessíveis de fora; é uma limitação do próprio ambiente de rede. Use [Tailscale / frp / Cloudflare Tunnel](/pt-BR/docs/modules/tunnels).

## Como relatar problemas ou participar do desenvolvimento?

Abra uma issue em [GitHub Issues](https://github.com/wangwanjie/gately/issues). As convenções de desenvolvimento estão em [Como contribuir](/pt-BR/docs/guide/contributing).
