# Instalação

## Docker

Imagem: `ghcr.io/wangwanjie/gately`, publicada também no Docker Hub como `vanjayak/gately` (a mesma imagem com as mesmas tags), disponível para amd64, arm64 e arm/v7. Se o GHCR for difícil de acessar, troque o nome da imagem abaixo por `vanjayak/gately:latest`; espelhos de registro do Docker também funcionam.

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

### Docker Compose

```yaml
services:
  gately:
    image: ghcr.io/wangwanjie/gately:latest
    container_name: gately
    restart: unless-stopped
    # Redirecionamento de portas, travessia STUN e serviços Web escutam diretamente nas portas do host
    network_mode: host
    # A Prevenção de intrusão grava os bloqueios no firewall do host (remova se não precisar)
    cap_add:
      - NET_ADMIN
    volumes:
      # Diretório de configuração (JSON em texto simples, pode ficar no git; o ssl.json contém chaves privadas de certificados)
      - ./data:/etc/gately
      # Gerenciamento do Docker (remova se não precisar)
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Shanghai
```

`--cap-add NET_ADMIN` e `cap_add` permitem que a [Prevenção de intrusão](/pt-BR/docs/modules/guard) grave os bloqueios no firewall nftables do host. Você pode removê-los; os bloqueios continuam valendo para os serviços do Gately.

A imagem inclui `sh`, `bash` e `curl`, então os scripts de notificação mais comuns funcionam sem ajustes.

## Binário

1. Obtenha o pacote da sua plataforma na [página de download](/pt-BR/download): Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows ou FreeBSD.
2. Extraia e execute:

```bash
./gately -c /etc/gately
```

### systemd

O repositório traz um arquivo de serviço completo, [deploy/systemd/gately.service](https://github.com/wangwanjie/gately/blob/main/deploy/systemd/gately.service), que já concede as capabilities necessárias para escutar em portas baixas:

```bash
sudo install -m 0755 gately /usr/local/bin/gately
sudo install -m 0644 gately.service /etc/systemd/system/gately.service
sudo systemctl daemon-reload && sudo systemctl enable --now gately
```

### OpenWrt {#openwrt}

Cada versão inclui pacotes para OpenWrt com o binário, o script de inicialização procd e uma regra que mantém a sua configuração nas atualizações de firmware:

- OpenWrt 24.10 e anteriores (incluindo o iStoreOS e outros firmwares baseados na 24.10) usam o `.ipk`, instalado com `opkg install`;
- OpenWrt 25.12 e posteriores usam o `.apk`, instalado com `apk add --allow-untrusted` (os pacotes não têm assinatura, por isso o `--allow-untrusted` é obrigatório).

Execute `grep DISTRIB_ARCH /etc/openwrt_release` para descobrir a arquitetura do dispositivo e escolha o pacote no grupo OpenWrt da [página de download](/pt-BR/download):

| DISTRIB_ARCH | Pacote |
| --- | --- |
| `x86_64` | `gately_<versão>_openwrt_amd64` |
| `aarch64_*` | `gately_<versão>_openwrt_arm64` |
| `arm_cortex-a*_*` (ARMv7 com VFP / NEON) | `gately_<versão>_openwrt_arm` |
| `mipsel_*` | `gately_<versão>_openwrt_mipsle` |
| `mips_*` | `gately_<versão>_openwrt_mips` |
| `riscv64_*` | `gately_<versão>_openwrt_riscv64` |

```bash
cd /tmp
# OpenWrt 24.10 / iStoreOS (exemplo com x86_64)
opkg install ./gately_<versão>_openwrt_amd64.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./gately_<versão>_openwrt_amd64.apk
```

- O serviço é ativado na inicialização e iniciado logo após a instalação; a interface de administração fica em `http://<endereço do roteador>:16680`. A configuração fica em `/etc/gately` e é mantida nas atualizações de firmware (sysupgrade);
- Atualização: baixe o pacote mais novo e execute `opkg install` ou `apk add --allow-untrusted` de novo; a sua configuração é mantida. Se você desativou o serviço na inicialização, ele continua desativado após a atualização;
- Desinstalação: `opkg remove gately` ou `apk del gately`. O diretório de configuração é mantido; execute `rm -rf /etc/gately` para removê-lo também;
- A arquitetura do dispositivo é verificada antes da instalação. O opkg interrompe a instalação quando a arquitetura não corresponde; o apk do OpenWrt 25.12 instala o pacote mesmo assim, mas deixa o serviço parado, então execute `apk del gately` e instale o pacote correto;
- O binário tem cerca de 32 MB e precisa do mesmo espaço livre (confira com `df -h /overlay`), o que atende soft routers x86, boxes ARM64 e outros dispositivos com bastante armazenamento; roteadores com 16 MB / 32 MB de flash são pequenos demais;
- O pacote arm requer ARMv7 com ponto flutuante por hardware. Dispositivos ARMv5 / ARMv6 e núcleos sem FPU, como o `arm_cortex-a9`, ainda não são compatíveis.

Você também pode implantar manualmente: baixe o arquivo compactado para Linux e use o script de inicialização procd [deploy/openwrt/gately.init](https://github.com/wangwanjie/gately/blob/main/deploy/openwrt/gately.init):

```bash
cp gately /usr/bin/gately && chmod +x /usr/bin/gately
cp gately.init /etc/init.d/gately && chmod +x /etc/init.d/gately
/etc/init.d/gately enable && /etc/init.d/gately start
```

#### Aplicativo LuCI {#openwrt-luci}

Cada versão também traz um aplicativo LuCI independente de arquitetura, `luci-app-gately_<versão>_openwrt_all.ipk` / `.apk`. Instale-o depois do Gately:

```bash
# OpenWrt 24.10 / iStoreOS
opkg install ./luci-app-gately_<versão>_openwrt_all.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./luci-app-gately_<versão>_openwrt_all.apk
```

Depois abra **Serviços → Gately** no LuCI para:

- ver o estado do serviço, a versão, o uso de memória e se ele inicia com o sistema;
- iniciar, parar e reiniciar o serviço, e ativar ou desativar o início automático;
- abrir a interface de administração do Gately com um clique, com a entrada segura das configurações globais já incluída no link;
- definir a porta da interface de administração. Ela fica em `/etc/config/gately` e, quando preenchida, substitui a porta do endereço de escuta das configurações globais do Gately (a parte de host, como um endereço de loopback ou da LAN, é mantida), o que ajuda quando a interface não abre por conflito de portas ou porta errada; deixe vazio para seguir as configurações globais.

Todo o resto continua sendo configurado na interface de administração do Gately. O aplicativo segue o idioma do LuCI e está traduzido para chinês simplificado e tradicional, inglês, japonês, coreano, alemão, francês, espanhol, russo e português (Brasil). Para desinstalar, remova primeiro o `luci-app-gately` (`opkg remove luci-app-gately` ou `apk del luci-app-gately`) e depois o gately.

## Aplicativo nativo para fnOS {#fnos}

Cada versão inclui pacotes offline para o fnOS: `gately_<versão>_fnos_x86.fpk` (Intel / AMD) e `gately_<versão>_fnos_arm.fpk` (ARM). Eles não estão na Central de aplicativos oficial, então instale-os manualmente:

1. Baixe o `.fpk` correspondente à arquitetura do seu dispositivo na [página de download](/pt-BR/download#fnos);
2. No fnOS, abra "Central de aplicativos → Instalação manual" (o botão no canto inferior esquerdo), escolha o arquivo e conclua o assistente;
3. Clique no ícone do Gately na área de trabalho do fnOS para abrir a interface de administração.

### Integrado ao gateway unificado do fnOS

O ícone da área de trabalho abre a interface de administração pelo gateway unificado do fnOS, em `/app/gately/` no endereço que você usa para o fnOS, compartilhando o mesmo ponto de entrada. Tanto o endereço da rede local quanto o seu domínio remoto do fnOS funcionam, sem precisar abrir ou redirecionar a porta 16680.

O acesso pelo gateway é controlado pelo fnOS, e somente administradores do fnOS podem abri-lo. A entrada segura e o "Acesso somente pela rede local" valem apenas para o acesso direto pela porta 16680. Na rede local, `http://<endereço do NAS>:16680` continua funcionando.

### Entrar com a sessão de administrador do fnOS

Ative "Entrar com o fnOS" em "Configurações → Controle de acesso" e os administradores que já estão conectados ao fnOS entram no Gately direto pelo ícone da área de trabalho, sem senha nem código de verificação em dois fatores:

- Somente administradores do fnOS são conectados, como administrador do Gately; os usuários comuns do fnOS continuam informando usuário e senha;
- Vale apenas para o acesso pelo gateway unificado; o acesso direto pela porta 16680 continua exigindo o login normal;
- Depois que você sai, o Gately não conecta você de novo silenciosamente; a página de login mostra um botão "Entrar como usuário do fnOS &lt;nome&gt;";
- A opção vem desativada por padrão e só aparece no pacote do fnOS.

### Observações

- O aplicativo roda como `root`, então pode escutar nas portas 80/443, gerenciar o Docker pelo `docker.sock` e abrir um terminal Web de verdade. Instale-o somente em um NAS que você controla.
- A configuração fica no diretório `var` do aplicativo e é mantida na desinstalação, a menos que você escolha "excluir tudo" no assistente de desinstalação.
- O pacote também adiciona o comando `/usr/local/bin/gately` para usar `gately ctl`, `gately mcp` e `gately reset-password`.
- A porta 16680 é a mesma da imagem Docker. Pare o contêiner `gately` antes de iniciar o aplicativo nativo.

## Primeiro acesso

A interface de administração escuta por padrão na porta `16680`. A conta padrão é `admin` e a senha padrão é `gately.cn`. Em implantações automatizadas, você pode definir outra senha inicial com a variável de ambiente `GATELY_INITIAL_PASSWORD` no lugar da senha padrão.

A senha padrão é pública; enquanto ela estiver em uso, um aviso fixo no topo da interface de administração pede para alterá-la. No primeiro acesso aparece um assistente de configuração inicial que sugere, em sequência, alterar a senha, ativar a autenticação em dois fatores ou uma passkey, executar o autodiagnóstico de rede, importar do Lucky e configurar notificações.

## Redefinir a senha e a autenticação em dois fatores {#reset}

Se você esquecer a senha do administrador ou perder o autenticador de dois fatores (TOTP), redefina-os com um comando no servidor onde o Gately roda. O comando precisa ler e gravar o diretório de configuração, o que equivale a ter acesso ao servidor, por isso não pode ser usado pela rede:

- `gately reset-password`: redefine a senha do administrador para a senha padrão `gately.cn`; com `-p <nova senha>` você já define uma senha nova;
- `gately reset-totp`: desativa a autenticação em dois fatores do administrador; depois de entrar, você pode ativá-la de novo.

Antes de executar, o comando mostra o nome de usuário do administrador e a ação que será feita e pede que você digite `yes` para confirmar, evitando execuções acidentais; em scripts e outros ambientes não interativos, use `-y` para pular a confirmação. Um Gately em execução aplica a mudança em poucos segundos, sem reiniciar; se ele não estiver rodando, a mudança vale na próxima inicialização. Depois da redefinição, todas as sessões abertas são invalidadas e os bloqueios de login são liberados; o nome de usuário, as passkeys, as contas somente leitura e o restante da configuração continuam iguais.

| Forma de instalação | Comando |
| --- | --- |
| Docker | `docker exec -it gately gately reset-password` |
| systemd | `sudo gately reset-password` |
| OpenWrt / iStoreOS | `gately reset-password` |
| fnOS | `sudo gately reset-password` |
| Outras | `./gately reset-password -c <diretório de configuração>` |

Sem `-c`, o comando procura, nesta ordem, em `GATELY_CONFIG_DIR`, em `data` no diretório atual, em `/etc/gately` e no diretório de configuração do pacote do fnOS, `/var/apps/gately/var/config`, e mostra o diretório usado de fato. Cada redefinição fica registrada no log do sistema e no log de auditoria.

## Atualização

### Verificação automática de atualizações e alertas de segurança {#update-check}

Por padrão, o Gately consulta o GitHub a cada 12 horas para ler a lista de versões do Gately e os [avisos de segurança publicados](https://github.com/wangwanjie/gately/security/advisories), e verifica a versão mais recente dos programas gerenciados instalados online (frp, túnel Cloudflare, Tailscale, File Browser). Todas as requisições são GET anônimas e não levam informações do dispositivo, da configuração ou do uso.

- **Nova versão**: um aviso aparece no topo da interface; você pode ver as novidades ou ignorar essa versão;
- **Atualização de segurança**: quando a versão atual está no intervalo afetado de um aviso, ou uma versão mais nova traz nas notas uma seção de correções de segurança, um aviso vermelho no topo lista os avisos e versões. O alerta de segurança só pode ser adiado por 24 horas e volta na hora se surgir um novo problema de segurança;
- **Notificações**: uma nova versão ou um novo problema de segurança publica o evento `system.update` / `system.security`, e uma nova versão de programa gerenciado publica `program.update`. Adicione uma regra com o modelo "Atualizações" em [Notificações](/pt-BR/docs/modules/notify) para recebê-las. Cada versão e cada problema de segurança é notificado uma única vez, mesmo após reiniciar.

O resultado fica salvo em `update-check.json` no diretório de configuração e, após atualizar, é reavaliado para a nova versão sem outra verificação. A primeira verificação ocorre cerca de 3 minutos após a inicialização (ou espera até completar 12 horas desde a última bem-sucedida) e, se falhar, é repetida depois de uma hora; o GitHub é acessado pelo proxy de atualização descrito abaixo. É possível desativá-la na página Sobre ou em Configurações → Rede e execução; aí o GitHub só é consultado ao clicar em "Verificar atualizações". A verificação automática apenas avisa: nunca baixa nem instala nada. O botão **Verificar atualizações** aparece só para administradores; contas somente leitura e tokens de API com apenas permissão de leitura que chamam a API de verificação recebem o último resultado sem consultar o GitHub.

### Atualização online {#online-upgrade}

Instalações binárias no Linux (systemd, contêineres LXC do PVE, OpenWrt implantado manualmente etc.) podem ser atualizadas online pela página Sobre do painel: clique em "Verificar atualizações" e, havendo versão nova, em "Atualizar agora". Isso vale também para contêineres LXC criados no PVE a partir da imagem OCI, sem precisar apagá-los e recriá-los. O Gately vai:

1. Baixar do GitHub o `gately_<versão>_linux_<arquitetura>.tar.gz` desta plataforma;
2. Conferir o SHA-256 com o `checksums.txt` da mesma versão;
3. Executar o novo binário em teste com `-version` para confirmar que roda nesta máquina;
4. Substituir o executável e reiniciar com os mesmos argumentos, guardando a versão anterior como `gately.old` no mesmo diretório.

Tudo acontece em uma caixa de diálogo: primeiro ela mostra as notas da versão e, ao iniciar, a etapa atual, o progresso e a velocidade do download. Até o binário ser substituído (obtenção, download e verificação) você pode clicar em "Cancelar atualização" a qualquer momento, sem alterar o binário atual.

Se o GitHub estiver difícil de acessar, defina um **proxy de atualização** na página Sobre ou em Configurações → Rede e execução. Ele suporta `http://`, `https://` e `socks5://`, e tanto a verificação quanto o download passam por ele. Vazio, o Gately conecta diretamente e continua respeitando a variável de ambiente `HTTPS_PROXY`.

Sem acesso à internet, baixe o tar.gz na [página de download](/pt-BR/download) e clique em "Enviar pacote de atualização" na página Sobre. Qualquer versão é aceita, então também dá para voltar a uma anterior.

A atualização online só roda quando um administrador a inicia; o Gately nunca baixa nem instala versões por conta própria. Se a nova versão não iniciar, reverta manualmente com a cópia guardada (exemplo com systemd):

```bash
mv /usr/local/bin/gately.old /usr/local/bin/gately
systemctl restart gately
```

Se você concedeu ao binário acesso a portas baixas com `setcap`, execute `setcap` de novo após atualizar.

### Outras implantações

- Docker: execute `docker pull` da nova imagem e recrie o contêiner (mantenha o diretório de configuração montado);
- fnOS: instale manualmente o `.fpk` mais novo pela Central de aplicativos; a sua configuração é mantida;
- Pacotes para OpenWrt: baixe o `.ipk` / `.apk` mais novo e execute `opkg install` / `apk add --allow-untrusted`; a sua configuração é mantida (veja [OpenWrt](#openwrt));
- Windows, macOS, FreeBSD: substitua o executável e reinicie o serviço.

Antes de atualizar, exporte um backup da configuração ou mantenha o diretório de configuração no git. As assinaturas dos arquivos e imagens baixados podem ser conferidas conforme [Verificar versões](/pt-BR/docs/guide/verify).
