# Port Knocking

Single Packet Authorization (SPA) compatível com o [fwknop](https://www.cipherdyne.org/fwknop/): as portas protegidas ficam inacessíveis de fora, o cliente primeiro envia um pacote de knock criptografado, autenticado por HMAC e protegido contra replay e, depois que o Gately o valida, só o endereço de origem desse cliente é liberado por um tempo limitado. É usado sobretudo para proteger o acesso direto por SSH: o que o terminal Web não cobre (scp, desenvolvimento remoto em IDE, encaminhamento de portas) também se conecta com segurança pela internet, e scanners e ataques de força bruta não enxergam a porta.

## Recursos

- **Compatível com clientes fwknop**: use diretamente o cliente oficial do fwknop (Linux, macOS, WSL); o formato das chaves é o mesmo de `KEY_BASE64` e `HMAC_KEY_BASE64` gerados por `fwknop -k`, e a página da regra gera com um clique o comando do cliente e a seção de `~/.fwknoprc`;
- **Segurança**: os pacotes são criptografados com AES-256-CBC, o HMAC (SHA-256 / 384 / 512) é verificado antes da descriptografia e pacotes com chave errada nunca chegam a ser descriptografados; cada pacote só pode ser usado uma vez (proteção contra replay) e carimbos de tempo fora da validade (padrão 120 segundos) são recusados; é possível limitar o nome de usuário e exigir que o pacote informe o endereço a liberar;
- **Bloqueio no firewall**: as portas protegidas são gravadas no firewall do sistema (nftables; iptables + ipset quando o kernel não tem nf_tables) e só os endereços que fizeram o knock são liberados, protegendo o sshd local e outros serviços que não passam pelo Gately;
- **Entradas do Gately liberadas por knock**: regras de redirecionamento de portas, roteamento por protocolo e encaminhamento integrado do STUN, além de cada rota individual do roteamento por protocolo, podem usar «Exigir knock», decidido pelo próprio Gately, sem permissão de firewall, o que funciona também no Windows, no macOS e em contêineres sem `NET_ADMIN`;
- **Uma única porta aberta**: o roteamento por protocolo reconhece pacotes de knock enviados por TCP (`fwknop -P tcp`), de modo que a mesma porta recebe o knock e, conforme o resultado, libera rotas como SSH;
- **Liberação após login web**: um site do Serviço Web pode servir de entrada de liberação; depois que o visitante passa pela autenticação do site, o IP de origem dele recebe a autorização da regra de knock, sem instalar cliente. O site também pode exigir admissão por batida; veja [Admissão por batida e liberação após login no Serviço Web](/pt-BR/docs/modules/web#knock);
- **Integração com a Prevenção de intrusão**: knocks recusados publicam o evento «Knock recusado», o modelo «Falha de port knocking» da Prevenção de intrusão bloqueia a origem por número de ocorrências, e knocks vindos de endereços já bloqueados são descartados direto;
- Lista das autorizações atuais (origem da autorização, pacote de knock ou login web, usuário que fez login, tempo restante, com revogação); knocks aceitos e recusados são registrados no log de execução e podem ser assinados na Central de notificações.

## Início rápido: proteger o SSH local

1. Abra «Port Knocking» e adicione uma regra: em porta, `tcp/22`; ative «Bloqueio no firewall»; deixe as chaves vazias para que sejam geradas automaticamente;
2. No cartão da regra, clique em «Cliente», informe o domínio ou o IP público do servidor e copie a linha de comando ou a seção de `~/.fwknoprc`;
3. Instale o fwknop no cliente: macOS `brew install fwknop`, Debian / Ubuntu `apt install fwknop-client`, Windows dentro do WSL;
4. Conecte por SSH nos 30 segundos seguintes ao knock:

```bash
fwknop -n gately-ssh && ssh user@example.com
```

Por padrão, o pacote de knock é enviado à porta UDP 62201. Se o Gately estiver atrás de um roteador, encaminhe a UDP 62201 junto com a porta protegida para o host do Gately; com «Liberar no firewall automaticamente» ativado, o firewall local (OpenWrt fw4, firewalld, ufw etc.) libera a porta de knock e, quando o bloqueio entra em vigor, também a porta protegida, e só o knock decide se é possível conectar.

::: warning Evite se trancar para fora
- «Knock dispensado na rede local» vem ativado: endereços privados, endereços link-local e as faixas IPv6 públicas das interfaces do host (o prefixo da interface quando é /64 ou mais curto; só o próprio prefixo nos /124 ou /128 comuns em servidores na nuvem) não precisam de knock, e você pode acrescentar uma lista de dispensa nas configurações. Endereços de CGNAT (100.64.0.0/10) não entram, porque outros clientes do mesmo provedor também os usam; quem usa Tailscale pode adicionar 100.64.0.0/10 à lista;
- Com o bloqueio no firewall ativado, as conexões já estabelecidas não são afetadas, e continuam normalmente mesmo depois que a autorização expira;
- Quando o Gately para normalmente, as regras de bloqueio são removidas; se ele é encerrado à força (`kill -9`, `docker rm -f`), as regras permanecem até o Gately iniciar de novo. Confirme que consegue entrar na máquina pela rede local ou por outro caminho.
:::

## Regras

| Campo | Descrição |
| --- | --- |
| Portas | Portas que o cliente pode pedir para liberar, como `tcp/22`, `udp/51820`, no máximo 16. Todas as portas do parâmetro `-A` do cliente precisam estar nesta lista; caso contrário o knock inteiro é recusado. Pode ficar vazio quando o bloqueio no firewall está desligado: a regra então só recebe autorizações pelo «Liberar após login» de um site web, não aceita pacotes de knock e não abre a porta UDP de knock por causa dela, e o cartão mostra «Nenhuma (liberado apenas por login web)» |
| Bloqueio no firewall | Quando ativado, bloqueia essas portas no firewall do sistema e libera só os endereços que fizeram o knock; desativado, o firewall não é alterado e a regra serve apenas às entradas que usam «Exigir knock» |
| Chave de criptografia / chave HMAC | Codificadas em base64; vazias, são geradas automaticamente (32 / 64 bytes, como em `fwknop -k`), e também é possível informar as chaves de um servidor fwknop existente. Copiar a regra gera chaves novas. Ao editar uma regra que continua ativada, mudar as portas, o bloqueio no firewall, a duração, o usuário exigido ou a exigência de endereço revoga todas as autorizações obtidas por pacotes de knock; mudar só o nome, o grupo, a observação, as [identidades](#identities) ou «um endereço por vez» revoga apenas as autorizações cujas chaves deixaram de valer (identidade desativada, excluída ou com chaves novas, chaves compartilhadas trocadas ou desativadas). As autorizações obtidas por login web são mantidas; desativar ou excluir a regra revoga todas as autorizações |
| Algoritmo HMAC | SHA-256 (padrão do fwknop), SHA-384, SHA-512; deve coincidir com `--hmac-digest-type` do cliente |
| Duração do acesso | Tempo em que novas conexões podem ser estabelecidas após o knock; padrão 30 segundos |
| Duração máxima do cliente | Limite quando o cliente pede uma duração maior com `-f` (padrão 300 segundos); 0 ignora a duração pedida pelo cliente; se o pedido for menor que a duração do acesso, ele sempre vale |
| Limitar nome de usuário | Aceita só knocks cujo nome de usuário no pacote seja este (o fwknop envia por padrão o login local, e `-U` define outro) |
| Exigir endereço | Recusa pacotes sem endereço (`fwknop -s`); o cliente precisa informar o endereço a liberar com `-R` ou `-a` |

Uma regra usada por redirecionamento de portas, roteamento por protocolo ou STUN não pode ser excluída; o cartão lista quem a utiliza.

### Identidades: chaves próprias para cada pessoa {#identities}

Por padrão, uma regra tem um conjunto de chaves compartilhadas e todos fazem knock com as mesmas chaves. Adicione identidades à regra e cada pessoa (ou dispositivo) faz knock com as próprias chaves:

- os logs, os eventos «Port knocking: knock bem-sucedido / recusado» (campo `identity`) e a lista de autorizações atuais mostram qual identidade foi, e o card mostra o número de knocks de cada identidade;
- desativar ou excluir uma identidade, ou dar chaves novas a ela, revoga só as autorizações dessa identidade; os demais continuam usando normalmente;
- a janela «Cliente» gera o comando do fwknop e a seção do `~/.fwknoprc` da identidade escolhida, então cada pessoa recebe só as próprias chaves;
- as identidades usam as portas, o algoritmo HMAC, a duração, o usuário exigido e a exigência de endereço da regra. O encaminhamento de portas, o roteamento por protocolo, o STUN e os sites web continuam referenciando a regra, e um knock bem-sucedido de qualquer identidade libera o acesso conforme a regra.

Uma regra pode ter até 64 identidades, e todas as regras ativadas juntas aceitam no máximo 1.024 chaves de knock (chaves compartilhadas e identidades). Todas as chaves HMAC precisam ser diferentes (inclusive entre regras); caso contrário, o pacote só corresponde à primeira. Uma regra sem portas não aceita pacotes de knock e não pode ter identidades.

- **Desativar chaves compartilhadas**: só as chaves das identidades são aceitas, e clientes com as chaves compartilhadas não conseguem mais fazer knock; é preciso ter pelo menos uma identidade ativada.
- **Um endereço por vez**: cada identidade (as chaves compartilhadas contam como uma) mantém autorização para só um endereço por vez. Após um knock bem-sucedido de um endereço novo, os outros endereços que essa identidade liberou por knock são revogados na hora (inclusive as entradas no firewall), impedindo que várias pessoas usem as mesmas chaves ao mesmo tempo. Conexões estabelecidas e autorizações por login web não são afetadas. Se duas identidades fizerem knock do mesmo endereço, a autorização fica com a mais recente.

### Qual endereço é liberado

- `-R` (recomendado): o cliente consulta o próprio IPv4 público e o grava no pacote; mesmo que o pacote seja interceptado no caminho, ele não serve para outro endereço;
- `-s`: o pacote não leva endereço e libera o endereço de origem do pacote. Quem intercepta o pacote pode tentar reenviá-lo antes, a partir do próprio endereço; a regra pode ativar «Exigir endereço» para recusar esses pacotes;
- `-a endereço`: libera o endereço indicado, por exemplo para dar o knock em nome de outro dispositivo.

O cliente fwknop 2.6 só envia pacotes de knock por IPv4 e só libera endereços IPv4, então o servidor precisa estar acessível por IPv4; o próprio Gately também aceita pacotes de knock enviados por IPv6 e libera o endereço de origem deles.

## Configurações globais

- **Porta UDP de knock**: padrão 62201 (a porta padrão do cliente fwknop); com 0 não escuta UDP e recebe apenas knocks enviados pelo roteamento por protocolo. Sem regras ativadas, ou quando nenhuma regra ativada tem portas preenchidas (liberação apenas por login web), não há escuta;
- **Liberar no firewall automaticamente**: veja acima;
- **Knock dispensado na rede local / lista de dispensa**: afetam apenas o bloqueio no firewall; as entradas com «Exigir knock» são controladas pelas próprias listas de IP;
- **Validade do pacote**: diferença máxima entre o carimbo de tempo do pacote e a hora local, padrão 120 segundos; os relógios do cliente e do servidor precisam estar corretos.

## Bloqueio no firewall {#firewall}

| Ambiente | Bloqueio no firewall | Entradas do Gately liberadas por knock |
| --- | --- | --- |
| Linux (root ou `CAP_NET_ADMIN`, com nftables) | Disponível | Disponível |
| Docker (rede host + `cap_add: NET_ADMIN`) | Disponível, as regras são gravadas no host | Disponível |
| Synology DSM e outros Linux sem nf_tables | Usa iptables (legacy) + ipset, já incluídos na imagem Docker oficial | Disponível |
| Windows, macOS, FreeBSD, Linux sem permissão | Indisponível, o motivo aparece no topo da página | Disponível |

O Gately cria no nftables uma tabela própria, `inet gately_knock`, sem alterar as regras existentes do sistema: a cadeia `prerouting` (prioridade -160, depois do rastreamento de conexões e antes da tradução de endereços de destino) entrega à cadeia `gate` os pacotes destinados às portas protegidas, que libera, em ordem, o tráfego cujo destino não é o host (conexões encaminhadas por este host a outros dispositivos não são afetadas), a interface de loopback, as conexões já estabelecidas, os pacotes TCP que não iniciam conexão, e endereço + protocolo + porta presentes nas redes dispensadas e no conjunto de liberações por knock, descartando o restante. Como a decisão é tomada no prerouting, portas mapeadas para contêineres Docker são bloqueadas pela porta anterior ao mapeamento. As liberações são gravadas como elementos de conjunto com timeout e o kernel os remove automaticamente ao expirar.

Sem nf_tables, usa-se a tabela mangle do iptables: `PREROUTING` salta para `GATELY_KNOCK`, com um conjunto ipset por porta protegida (`gkn4_tcp_22` etc., com timeout). Se a tabela ou o salto forem removidos por um recarregamento do firewall, por `nft flush ruleset` etc., ele é regravado em até 10 segundos, junto com as liberações ainda válidas. «Ver regras do firewall» na página mostra as regras gravadas no momento.

### Autorização por login web {#web-grant}

As autorizações obtidas pelo [Liberar após login](/pt-BR/docs/modules/web#knock) de um site web também valem nas regras com o bloqueio no firewall ativado: o Gately grava no conjunto de liberações os endereços que de fato acessaram o site de entrada com essa autorização (até 8 por autorização; acima disso, sai o mais antigo), junto com as portas da regra e com a mesma validade da autorização, e os remove quando a autorização é revogada ou expira. Autorizações IPv6 são avaliadas por faixa (padrão /64), mas o firewall libera apenas os endereços específicos registrados. Se o mesmo endereço também tiver uma autorização obtida por pacote de knock, cada uma é mantida até a própria validade.

::: tip SSH que entra por redirecionamento de portas
Conexões encaminhadas pelo redirecionamento de portas do Gately para `127.0.0.1:22` do host chegam pela interface de loopback e não passam pelo bloqueio no firewall. Nesse caso, escolha «Exigir knock» na regra de redirecionamento de portas ou proteja, no bloqueio no firewall, a porta externa de encaminhamento.
:::

::: warning Atenção
- **O mapeamento de portas do Docker reescreve o endereço de origem**: quando o Gately roda numa rede bridge com a porta de knock mapeada por `-p 62201:62201/udp` e o userland-proxy do Docker cuida desse mapeamento (por exemplo, no acesso por IPv6 ou pelo endereço de loopback, ou com a integração com iptables desligada), o Gately vê o endereço do gateway do Docker como origem: um pacote `-s` autoriza o gateway em vez do cliente real e pode até abrir a porta para todas as conexões que passam por ele. Rode o Gately na rede host (o bloqueio no firewall também precisa) ou faça os clientes informarem o endereço com `-R` / `-a` e ative «Exigir endereço»;
- **A troca de bloquear só SYN**: para não derrubar conexões estabelecidas antes da ativação e ainda não registradas pelo rastreamento de conexões, no TCP só são descartados os pacotes SYN que abrem conexão, e pacotes sem SYN passam. Em tese, um sistema com SYN cookies ativados (`net.ipv4.tcp_syncookies`) aceita, enquanto a fila SYN transborda, um ACK avulso que acerte um cookie válido e estabelece a conexão; o cookie é um número de sequência inicial de 32 bits calculado com um segredo do kernel e o horário, então acertá-lo é praticamente impossível; em cenários com exigência de segurança muito alta dá para desativar os SYN cookies (`sysctl net.ipv4.tcp_syncookies=0`).
:::

## Entradas do Gately liberadas por knock {#gately-entries}

Em «Controle de acesso por IP» de redirecionamento de portas, roteamento por protocolo e STUN (encaminhamento integrado), é possível escolher uma regra de knock como «Exigir knock»: o endereço de origem precisa ter uma autorização ainda válida dessa regra para estabelecer novas conexões (no UDP, ao criar uma sessão nova: depois que a autorização expira, sessões existentes como WireGuard continuam sendo encaminhadas, como as conexões estabelecidas no bloqueio pelo firewall; listas de permissões / de bloqueio continuam sendo verificadas por pacote), e as demais conexões são fechadas; quando combinada com lista de permissões / de bloqueio, as duas condições precisam ser atendidas. A regra de knock pode desativar o «Bloqueio no firewall» e servir apenas a essas entradas.

O roteamento por protocolo também permite «Exigir knock» em uma rota individual, por exemplo: na porta 443, a rota HTTPS fica aberta a todos e a rota SSH só aos endereços que fizeram o knock.

### Abrir uma única porta

Em uma regra de roteamento por protocolo, adicione uma rota «Port Knocking» (protocolo `spa`); o cliente envia o pacote de knock por TCP à mesma porta:

```bash
fwknop -A tcp/443 -P tcp -p 443 -R -D example.com --use-hmac \
  --key-base64-rijndael <encryption key> --key-base64-hmac <HMAC key>
ssh -p 443 user@example.com
```

- A rota de knock entrega o pacote ao módulo de port knocking, sem encaminhar nem responder; a porta em `-A` precisa estar na lista de portas da regra de knock (por exemplo `tcp/443`);
- Quando a regra inteira exige knock, a origem que ainda não fez o knock só pode usar a rota de knock, e só depois do knock acessa as outras rotas e o destino padrão;
- Pacotes de knock só são reconhecidos quando a regra tem uma rota de knock, sem afetar a identificação de protocolos das outras regras;
- A porta UDP da regra de knock pode ser 0, expondo apenas a porta do roteamento por protocolo;
- As portas do Roteamento por protocolo com rota de knock e a porta UDP de knock não podem estar numa regra com bloqueio no firewall (verificado ao salvar e ao ativar, porque bloqueá-las impediria a chegada dos próprios pacotes de knock), e são sempre excluídas ao gerar as regras do firewall.

## Integração com a Prevenção de intrusão

Knocks recusados publicam o evento «Knock recusado» (o campo `via` é `tcp`), no máximo uma vez a cada 5 segundos por origem. O endereço de origem de um pacote UDP pode ser forjado, e um pacote válido pode ser capturado e reenviado de uma origem forjada; para que ninguém use a Prevenção de intrusão para bloquear IPs arbitrários, knocks recusados que chegam por UDP só são contados e registrados no log (um a um quando passaram no HMAC), sem evento. Pacotes que chegam por uma porta TCP do Roteamento por protocolo vêm de um handshake completo e não podem ser forjados, então ali um HMAC diferente ou uma recusa da regra publicam o evento. Na Prevenção de intrusão, crie uma regra com o modelo «Falha de port knocking» (por padrão, 5 ocorrências em 10 minutos bloqueiam por 1 dia) e as origens que continuarem enviando pacotes inválidos por TCP serão bloqueadas; knocks de endereços bloqueados são descartados. Dados fora do formato de knock (varreduras de portas, outros protocolos) só são contados, sem evento.

Antes da autenticação, a taxa é limitada por faixa de origem (no máximo 10 pacotes por segundo para cada /24 IPv4 e cada /64 IPv6) e depois por um orçamento global medido em trabalho de HMAC (cada pacote custa o número de chaves que aceitam knock (chaves compartilhadas e identidades) vezes uma unidade a cada 512 bytes; 20.000 unidades por segundo, cerca de 20.000 pacotes comuns por segundo com uma única chave); o excedente é descartado. Assim, uma enxurrada de pacotes forjados não ocupa toda a CPU e a enxurrada de uma origem não expulsa os knocks normais de outras. Pacotes que falham no HMAC não são registrados um a um: a cada minuto viram uma linha «M pacotes de knock inválidos de N origens».

## Limitações

- Só há suporte ao modo Rijndael (AES) CBC do fwknop com HMAC; GnuPG, o modo IV antigo e pacotes sem HMAC não são suportados;
- Não executa mensagens de comando (`fwknop -C`) e não suporta solicitações de acesso NAT (`-N`);
- As autorizações de knock ficam na memória; depois de reiniciar o Gately, é preciso dar o knock de novo ou abrir outra vez a página de liberação web (conexões já estabelecidas não são afetadas). Os registros antirreplay também ficam só na memória, então após um reinício qualquer pacote com carimbo de tempo anterior à inicialização é recusado; se o relógio do cliente estiver atrasado em relação ao servidor, um knock nos primeiros segundos após o reinício pode ser recusado, e basta bater de novo.
