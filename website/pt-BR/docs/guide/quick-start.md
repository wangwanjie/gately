# Início rápido

Implante, entre no painel e crie a sua primeira regra em cinco minutos.

## 1. Iniciar

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  ghcr.io/wangwanjie/gately:latest
```

Para gerenciar o Docker pelo Gately, adicione `-v /var/run/docker.sock:/var/run/docker.sock` ao comando. `--cap-add NET_ADMIN` permite que a Prevenção de intrusão grave os bloqueios no firewall do host. Redirecionamento de portas, STUN e serviços Web escutam nas portas do host, por isso recomendamos `--network host`.

Se o GHCR estiver lento para você, use `vanjayak/gately:latest` do Docker Hub; é a mesma imagem.

::: tip Usuários do fnOS
Instale o aplicativo nativo: ele abre pelo ícone da área de trabalho através do gateway unificado do fnOS e permite que os administradores do fnOS entrem com a sessão do fnOS. Veja [Aplicativo nativo para fnOS](/pt-BR/docs/guide/install#fnos).
:::

## 2. Entrar

Abra `http://<ip-do-dispositivo>:16680` no navegador. O usuário é `admin` e a senha padrão é `gately.cn`. A senha padrão é pública; depois de entrar, abre-se uma janela de troca de senha, e só é possível continuar depois de definir uma nova senha com pelo menos 10 caracteres e difícil de adivinhar.

Em uma instalação nova, depois de alterar a senha, o Gately pergunta primeiro «Para que você vai usar o Gately?»: escolha uma ou mais entre «Publicar serviços internos em um domínio», «Encaminhamento de portas / hole punching», «DNS dinâmico», «Rede entre locais», «Compartilhamento de arquivos», «Ponto de entrada VPS» ou «Todos os recursos». Apenas os módulos dos cenários escolhidos permanecem ativos, os demais são desligados (as configurações são mantidas) e o Gately reinicia uma vez; você pode ativá-los quando quiser em «Configurações → Módulos» ou com «Adicionar mais recursos» no fim da barra lateral. «Pular» mantém tudo ativo. Instâncias atualizadas não veem esta etapa. A lista «Primeiros passos» no topo da visão geral mostra os próximos passos dos cenários escolhidos e os marca conforme a configuração real; ela pode ser recolhida ou fechada.

A barra lateral da interface de administração lista todas as páginas por grupo. Pressione Ctrl+K (⌘K no macOS) ou clique na caixa de busca no topo da barra lateral para abrir a busca global: ela leva a páginas e às suas abas pelo nome e também encontra domínios de sites, regras de encaminhamento de portas, registros DDNS e domínios de certificados. No celular, toque no ícone de busca na barra superior.

## 3. Aplicar o reforço básico de segurança

Logo após entrar, vá em "Configurações" e:

1. Ative a autenticação em dois fatores (TOTP) ou registre uma passkey;
2. Defina um caminho de entrada seguro, acesso apenas pela rede local ou HTTPS na interface de administração, conforme a sua necessidade;
3. Confira a pontuação de segurança no topo de "Configurações → Conta e segurança" e resolva os itens que perdem pontos.

Veja mais recomendações no [Guia de reforço de segurança](/pt-BR/docs/guide/hardening).

## 4. Adicionar a primeira regra

«Site rápido» na página Serviço Web publica um site em três etapas: informe o domínio, o endereço do backend (com «Testar», ou preenchido pelos modelos de Home Assistant, Jellyfin, Emby, Vaultwarden, Nextcloud, Immich, Proxmox VE, Synology DSM, fnOS) e escolha HTTP ou HTTPS. Um listener é reutilizado ou criado automaticamente; veja [Serviço Web · Site rápido](/pt-BR/docs/modules/web#quick-site).

Escolha o módulo que atende à sua necessidade:

| Necessidade | Módulo |
| --- | --- |
| O IP público da sua casa muda com frequência e você quer um nome de domínio estável | [DNS dinâmico](/pt-BR/docs/modules/ddns) |
| Sua operadora não oferece porta pública fixa e você quer acessar serviços de casa de fora | [Travessia STUN](/pt-BR/docs/modules/stun) |
| Encaminhar uma porta para outro dispositivo da rede local | [Redirecionamento de portas](/pt-BR/docs/modules/portforward) |
| Acessar vários serviços da rede local por um domínio com HTTPS | [Certificados SSL](/pt-BR/docs/modules/ssl) + [Serviços Web](/pt-BR/docs/modules/web) |
| Conectar redes em locais diferentes | [Tailscale / frp](/pt-BR/docs/modules/tunnels) |
