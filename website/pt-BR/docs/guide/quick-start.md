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

## 3. Aplicar o reforço básico de segurança

Logo após entrar, vá em "Configurações" e:

1. Ative a autenticação em dois fatores (TOTP) ou registre uma passkey;
2. Defina um caminho de entrada seguro, acesso apenas pela rede local ou HTTPS na interface de administração, conforme a sua necessidade;
3. Confira a pontuação de segurança no topo de "Configurações → Conta e segurança" e resolva os itens que perdem pontos.

Veja mais recomendações no [Guia de reforço de segurança](/pt-BR/docs/guide/hardening).

## 4. Adicionar a primeira regra

Escolha o módulo que atende à sua necessidade:

| Necessidade | Módulo |
| --- | --- |
| O IP público da sua casa muda com frequência e você quer um nome de domínio estável | [DNS dinâmico](/pt-BR/docs/modules/ddns) |
| Sua operadora não oferece porta pública fixa e você quer acessar serviços de casa de fora | [Travessia STUN](/pt-BR/docs/modules/stun) |
| Encaminhar uma porta para outro dispositivo da rede local | [Redirecionamento de portas](/pt-BR/docs/modules/portforward) |
| Acessar vários serviços da rede local por um domínio com HTTPS | [Certificados SSL](/pt-BR/docs/modules/ssl) + [Serviços Web](/pt-BR/docs/modules/web) |
| Conectar redes em locais diferentes | [Tailscale / frp](/pt-BR/docs/modules/tunnels) |
