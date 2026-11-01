---
title: Changelog
description: Anúncios e notas de cada versão do Gately.
---

# Changelog

Abaixo estão os anúncios das versões principais. Veja as [GitHub Releases](https://github.com/wangwanjie/gately/releases) para as mudanças completas e os downloads de cada versão.

## [v0.6.2](/pt-BR/news/v0.6.2)

31 de outubro de 2026 · O novo módulo Roteamento por protocolo permite que HTTPS, SSH, RDP, OpenVPN e outros compartilhem uma única porta; todas as listas longas podem ser agrupadas e ordenadas; o DNS dinâmico pode usar endereços públicos do STUN; o STUN funciona atrás de vários NATs; várias melhorias de segurança.

## [v0.6.1.1](/pt-BR/news/v0.6.1.1)

30 de outubro de 2026 · A prevenção de intrusão bloqueia a sondagem de caminhos web; uma nova página Usuários de autenticação gerencia os usuários compartilhados e o OIDC; usuários compartilhados podem trocar a própria senha; sites podem fechar a conexão ao negar acesso; as rotas de túneis Cloudflare com token podem ser editadas no Gately; os logs ganham um filtro Excluir.

## [v0.6.1](/pt-BR/news/v0.6.1)

30 de outubro de 2026 · Correção de segurança: contas e tokens de API somente leitura não conseguem mais ler em texto puro as variáveis de ambiente e os comandos de inicialização dos contêineres; certificados podem ser emitidos somente por IPv4 ou somente por IPv6; a página do Docker não atualiza nem recria mais o próprio contêiner do Gately; notificações e logs seguem o idioma da interface de administração.

## [v0.6.0.4](/pt-BR/news/v0.6.0.4)

29 de outubro de 2026 · Adiciona um aplicativo LuCI para OpenWrt que permite ver o estado, iniciar e parar o serviço e definir a porta de administração pelo LuCI; os sites de redirecionamento aceitam variáveis como `{path}`, `{1}` e `{STUN_NomeRegra_PORT}`; os usuários compartilhados e os clientes OIDC das configurações globais aparecem como resumos e são editados em uma janela.

## [v0.6.0.3](/pt-BR/news/v0.6.0.3)

28 de outubro de 2026 · Corrige um Gately local que recebe HTTP em texto puro de uma origem PROXY confiável e não reconhecia que o visitante usava HTTPS, o que fazia falhar o login OIDC com emissor https, deixava cookies sem Secure e causava loops de redirecionamento.

## [v0.6.0.2](/pt-BR/news/v0.6.0.2)

28 de outubro de 2026 · Corrige o terminal web parado em "Conectando" ao abrir pela área de trabalho do fnOS, o File Browser que não iniciava com um diretório de configuração relativo e as respostas da API em idiomas diferentes do chinês simplificado que traduziam parte do texto em chinês digitado pelos usuários.

## [v0.6.0.1](/pt-BR/news/v0.6.0.1)

27 de outubro de 2026 · O DNS dinâmico e o redirecionamento de portas ganham grupos, ordenação e busca; as páginas Serviço web, DNS dinâmico e redirecionamento de portas permitem definir o grupo de vários itens de uma vez, arrastar vários itens marcados juntos e renomear grupos.

## [v0.6.0](/pt-BR/news/v0.6.0)

27 de outubro de 2026 · Várias instâncias de frpc e frps; o Serviço web envia usuários conectados para seus próprios backends, agrupa, ordena e pesquisa sites e pode encaminhar os cookies de login do Gately; certificados detectam a zona DNS de subdomínios delegados; senhas e chaves salvas podem ser exibidas sob demanda.

## [v0.5.9.1](/pt-BR/news/v0.5.9.1)

26 de outubro de 2026 · Listeners HTTPS do Serviço web podem aceitar HTTP na mesma porta (redirecionar ou servir); instâncias do File Browser aceitam URLs de acesso; salvar uma instância do File Browser ou um túnel do Cloudflare editados não falha mais.

## [v0.5.9](/pt-BR/news/v0.5.9)

26 de outubro de 2026 · File Browser gerenciado pelo Gately; certificados de endereço IP e de curta duração; instalação, atualização, reversão e desinstalação online de frp, cloudflared, Tailscale e File Browser; os usuários compartilhados dos serviços de arquivos ganham uma página própria.

## [v0.5.8.1](/pt-BR/news/v0.5.8.1)

26 de outubro de 2026 · WebDAV, FTP e SMB compartilham uma lista de usuários compartilhados; o Serviço Web ganha o tipo de site Fechar conexão e registra Referer e User-Agent; o frps pode escutar em endereços escolhidos; os botões de copiar funcionam via HTTP.

## [v0.5.8](/pt-BR/news/v0.5.8)

25 de outubro de 2026 · Provedor de identidade OIDC embutido para que usuários compartilhados entrem no Gitea, no Grafana e em outros aplicativos; novo Serviço SMB; todos os logs podem ser filtrados; os grupos de regras do WAF podem ser reordenados; os cabeçalhos dos sites podem ser escolhidos entre entradas comuns.

## [v0.5.7](/pt-BR/news/v0.5.7)

24 de outubro de 2026 · Montagens de armazenamento se conectam a compartilhamentos SMB; os temas viram 15 paletas com claro e escuro, destaque e cantos personalizáveis; a interface de administração é transferida compactada e o binário ficou cerca de 11% menor.

## [v0.5.6.4](/pt-BR/news/v0.5.6.4)

23 de outubro de 2026 · A implantação automática de certificados copia para vários diretórios locais, cada um com seu comando, e aceita nomes de arquivo personalizados para o certificado e a chave.

## [v0.5.6.3](/pt-BR/news/v0.5.6.3)

23 de outubro de 2026 · Corrige a visão geral que ocultava a interface br-lan no OpenWrt e o fuso horário que não podia ser definido, com o horário sempre em UTC.

## [v0.5.6.2](/pt-BR/news/v0.5.6.2)

23 de outubro de 2026 · Novo visual das páginas de login e de erro do serviço web, com a verificação em duas etapas em uma janela. Corrige o módulo Docker que não conectava no Docker 29, a visão geral que aparecia por um instante antes da página de login e credenciais Cloudflare e registros DDNS renomeados importados do Lucky que não sincronizavam.

## [v0.5.6.1](/pt-BR/news/v0.5.6.1)

22 de outubro de 2026 · Corrige as regras de firewall gravadas pela imagem Docker que faziam um nft antigo no host travar.

## [v0.5.6](/pt-BR/news/v0.5.6)

22 de outubro de 2026 · Nova prevenção de intrusão: regras de logs no estilo fail2ban, portas armadilha e detecção de varredura, com bloqueios sincronizados no nftables.

## [v0.5.5.2](/pt-BR/news/v0.5.5.2)

22 de outubro de 2026 · Corrige a leitura de arquivos sensíveis do diretório de configuração por outros usuários locais no fnOS e em sistemas de arquivos semelhantes.

## [v0.5.5.1](/pt-BR/news/v0.5.5.1)

21 de outubro de 2026 · Os logs de execução são gravados em disco e sobrevivem a reinícios e atualizações.

## [v0.5.5](/pt-BR/news/v0.5.5)

21 de outubro de 2026 · Importação de configurações do nginx, implantação automática de certificados em outros hosts e contêineres, WAF com suporte a uploads grandes e WebDAV, listas de IP assinadas e novas verificações de cobertura de certificados e conflitos do frp no autodiagnóstico.

## [v0.5.4](/pt-BR/news/v0.5.4)

21 de outubro de 2026 · Atualização online para instalações binárias no Linux, sem precisar recriar contêineres LXC do PVE; novos pacotes para OpenWrt (.ipk / .apk).

## [v0.5.3](/pt-BR/news/v0.5.3)

21 de outubro de 2026 · Aplicativo nativo para fnOS, integrado ao gateway unificado do fnOS e com login automático para administradores do fnOS pela sessão do fnOS; módulos de recursos podem ser ativados ou desativados.

## [v0.5.2](/pt-BR/news/v0.5.2)

20 de outubro de 2026 · O roteamento SNI permite que vários serviços compartilhem a porta 443, os Serviços Web podem substituir o nginx, os usuários compartilhados ganham TOTP e login único, e uma comparação de desempenho com haproxy + nginx.

## [v0.5.0](/pt-BR/news/v0.5.0)

19 de outubro de 2026 · Log de auditoria, histórico de configuração e reversão, passkey, contas somente leitura, ACME HTTP-01, HTTP/3, protocolo PROXY, métricas globais do Prometheus, autodiagnóstico de rede e versões assinadas.
