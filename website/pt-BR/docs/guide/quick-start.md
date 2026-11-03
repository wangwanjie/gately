# Início rápido

Implante, entre no painel e crie a sua primeira regra em cinco minutos.

## 1. Iniciar

```bash
docker run -d --name gately --network host \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

Se você não precisa do gerenciamento do Docker, remova a montagem do `docker.sock`. Redirecionamento de portas, STUN e serviços Web escutam nas portas do host, por isso recomendamos `--network host`.

Se o GHCR estiver lento para você, use `vanjayak/gately:latest` do Docker Hub; é a mesma imagem.

::: tip Usuários do fnOS
Instale o aplicativo nativo: ele abre pelo ícone da área de trabalho através do gateway unificado do fnOS e permite que os administradores do fnOS entrem com a sessão do fnOS. Veja [Aplicativo nativo para fnOS](/pt-BR/docs/guide/install#fnos).
:::

## 2. Entrar

Abra `http://<ip-do-dispositivo>:16680` no navegador. O usuário é `admin` e a senha padrão é `gately.cn`. A senha padrão é pública; até você alterá-la, um aviso fica fixo no topo da interface de administração.

## 3. Aplicar o reforço básico de segurança

Logo após entrar, vá em "Configurações" e:

1. Altere a senha do administrador;
2. Ative a autenticação em dois fatores (TOTP);
3. Defina um caminho de entrada seguro, acesso apenas pela rede local ou HTTPS na interface de administração, conforme a sua necessidade.

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
