# Introdução

O Gately é um kit de gateway de rede de código aberto e auditável, para redes domésticas, NAS, servidores em nuvem e servidores de todo tipo. DDNS, travessia de NAT com STUN, redirecionamento de portas, proxy reverso, certificados, WAF e mais vêm reunidos em um único binário de cerca de 32 MB.

## Por que o Gately

O Gately reúne DDNS, travessia STUN, proxy reverso e outros recursos e é totalmente open source, para que usuários e comunidade possam revisar diretamente o código e a configuração. Para um software que fica na borda da rede e guarda chaves de DNS e chaves privadas de certificados, o código aberto permite ver exatamente o que ele faz. Os compromissos do Gately:

- O código-fonte é aberto sob a licença Apache-2.0 e aceita revisão da comunidade
- Por padrão, nenhuma conexão com servidores do projeto e nenhum envio de informações do dispositivo; o login de terceiros usa os seus próprios aplicativos OAuth, sem intermediários
- Todo endereço externo (consulta do IP público, servidores STUN etc.) é visível, editável e pode ser desativado
- A configuração é JSON em texto simples, pronta para auditoria e rollback com git

## Princípios de design

1. **Transparência**: por padrão, nenhuma conexão com servidores do projeto; todo endereço externo é visível, editável e pode ser desativado.
2. **Migração fácil**: importação de configurações existentes, como sites do nginx; as regras importadas ficam desativadas por padrão, para uma transição gradual.
3. **Configuração auditável**: JSON em texto simples (permissão de arquivo 0600) que vai direto para o git, com diff e rollback.
4. **Implantação em um único arquivo**: um binário estático com o frontend embutido, compatível com Docker, systemd e OpenWrt.

## Stack tecnológica

| Camada | Escolha |
| --- | --- |
| Backend | Go, compilado estaticamente, com compilação cruzada para roteadores e NAS arm / arm64 / mips / riscv |
| HTTP | Biblioteca padrão `net/http` |
| Armazenamento | Um arquivo JSON em texto simples por módulo, gravado de forma atômica |
| Autenticação | Senhas com bcrypt + tokens de sessão assinados com HMAC, limitação de tentativas de login, autenticação em dois fatores TOTP |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, empacotado no binário via `go:embed` |

## Visão geral dos recursos

DNS dinâmico, travessia STUN, redirecionamento de portas, roteamento por protocolo, certificados SSL, serviços Web (proxy reverso, controle de acesso, health checks, cache e limitação de taxa, estatísticas de tráfego, login único OIDC), firewall WAF, prevenção de intrusão, port knocking, Tailscale, frp, Cloudflare Tunnel, gerenciamento do Docker, terminal Web, diagnóstico de rede, Wake-on-LAN, WebDAV, FTP, SMB, File Browser (gerenciador de arquivos Web FileBrowser Quantum gerenciado pelo Gately), montagens de armazenamento, tarefas agendadas, central de notificações e backup da configuração, além de autenticação em dois fatores, caminho de entrada seguro, tokens de API, CLI e MCP. Veja os detalhes na [Visão geral dos módulos](/pt-BR/docs/modules/).

## Próximos passos

- Quer testar agora: [Início rápido](/pt-BR/docs/guide/quick-start)
- Vindo de outras ferramentas: [Guia de migração](/pt-BR/docs/guide/migration)
- Se importa com privacidade e segurança: [Modelo de segurança e compromisso de privacidade](/pt-BR/docs/guide/security)
