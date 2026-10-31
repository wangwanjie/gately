# Roteamento por protocolo

Vários serviços compartilham uma única porta pública (por exemplo, um site, uma VPN, SSH e Área de Trabalho Remota, todos na 443). O Gately lê os primeiros dados enviados pelo cliente para identificar o protocolo, depois examina o domínio (SNI) e o ALPN do handshake TLS ou o Host do HTTP, e repassa a conexão inteira ao backend correspondente. Nada é descriptografado nem terminado aqui, então os certificados TLS continuam nos backends. Ele faz o trabalho das regras `req_ssl_sni` do haproxy somado ao sslh.

O roteamento por domínio (roteamento SNI) é um caso particular: adicione uma rota TLS e preencha os domínios.

Cada regra tem:

- Escuta: o protocolo (somente TCP; tcp é pilha dupla, tcp4 / tcp6 usam uma única família de endereços), o endereço e as portas de escuta (`443`, `443,8443` ou um intervalo);
- Rotas: cada rota tem um protocolo, um nome, endereços de destino (vários endereços são tentados em sequência, com failover), uma porta de destino opcional (vazia significa a porta de escuta) e uma configuração do protocolo PROXY (desativado, v1 ou v2). A configuração PROXY pertence à rota porque alguns backends a exigem e outros a recusam;
- Um destino padrão: os endereços de destino, as portas de destino e a configuração do protocolo PROXY da própria regra, usado quando nenhuma rota corresponde ou o protocolo não é reconhecido;
- Um tempo de detecção: quantos segundos esperar por dados reconhecíveis do cliente; 0 usa o padrão de 5 segundos.

## Protocolos reconhecidos

| Protocolo da rota | Reconhecido por | Correspondência adicional |
| --- | --- | --- |
| TLS | O registro de handshake TLS (ClientHello) | Domínio (SNI), ALPN |
| HTTP | O método e a versão `HTTP/` na linha de requisição, incluindo CONNECT, métodos WebDAV e HTTP/2 com conhecimento prévio (h2c) | Domínio (cabeçalho Host) |
| SSH | A string de versão do cliente `SSH-` | — |
| RDP (Área de Trabalho Remota) | O cabeçalho TPKT e a solicitação de conexão X.224 | — |
| SOCKS4 / SOCKS5 | Requisições SOCKS4 e SOCKS4a e a negociação de método do SOCKS5 | — |
| OpenVPN (TCP) | O pacote de hard reset do cliente, incluindo tls-auth, tls-crypt e tls-crypt-v2 | — |
| XMPP | Uma declaração XML ou `<stream:stream` | — |
| MQTT | O pacote CONNECT (3.1, 3.1.1, 5.0) | — |
| PostgreSQL | A mensagem de inicialização, SSLRequest e GSSENCRequest | — |
| Minecraft (Java Edition) | O pacote de handshake; o ping da lista de servidores da 1.6 e anteriores | Endereço do servidor |
| RTSP | A versão `RTSP/1.0` ou `RTSP/2.0` na linha de requisição (câmeras IP, NVRs) | — |
| SIP | A versão `SIP/2.0` na linha de requisição (TCP) | — |
| Redis | Comandos enviados como arrays RESP | — |
| AMQP | O cabeçalho de protocolo `AMQP` (0-9-1 e 1.0, RabbitMQ e outros) | — |
| SMB | Requisições de negociação SMB1, SMB2 e SMB3 dentro de uma mensagem de sessão NetBIOS | — |
| Git | A primeira pkt-line do protocolo `git://` (git daemon) | — |
| IRC | O comando CAP, PASS, NICK ou USER que o cliente envia primeiro | — |
| ADB | A mensagem CNXN do Android Debug Bridge (adb connect) | — |
| BitTorrent | Um handshake entre pares sem criptografia | — |
| Tempo de espera | O cliente não envia nada dentro do tempo de detecção | — |

## Correspondência

Rotas TLS, HTTP e Minecraft correspondem por domínio (o Minecraft usa o endereço do servidor digitado pelo cliente, lido do handshake; os dados acrescentados por Forge, BungeeCord ou TCPShield são ignorados): primeiro os domínios exatos (`vpn.example.com`), depois o curinga mais longo (`*.example.com`, em qualquer profundidade), depois `*` (qualquer conexão que traga um domínio) e, por fim, as rotas sem domínio (todas as conexões daquele protocolo, incluindo as sem SNI ou Host). Os domínios não diferenciam maiúsculas de minúsculas.

Rotas TLS também podem listar valores ALPN: a rota só corresponde quando o cliente oferece um deles. No mesmo domínio, rotas com ALPN têm prioridade e, quando o ALPN não corresponde, a busca segue para domínios mais amplos. Por exemplo, envie `acme-tls/1` ao programa que responde à validação de certificado TLS-ALPN-01, ou `postgresql` às conexões TLS diretas do PostgreSQL 17. Dentro de um protocolo, um domínio (com um dado ALPN) só pode pertencer a uma rota; os demais protocolos admitem uma rota cada.

"Tempo de espera" serve para protocolos em que o servidor envia primeiro, como VNC, MySQL, FTP, SMTP, POP3 e IMAP: seus clientes conectam e aguardam o servidor, então o Gately não recebe nada e não consegue distinguir qual é. Por isso cada porta pode enviar apenas um deles para a rota Tempo de espera, e essas conexões são encaminhadas quando o tempo de detecção expira. O tempo de detecção aceita uma casa decimal: clientes TLS, SSH, HTTP e semelhantes enviam o primeiro pacote milissegundos após o handshake, então com uma rota Tempo de espera bastam 0,3–1 segundo, e é só isso que VNC e conexões semelhantes esperam.

Estas conexões vão para o destino padrão: nenhuma rota corresponde; os dados não pertencem a nenhum dos protocolos acima (por exemplo, protocolos sem assinatura como Shadowsocks, VMess ou BitTorrent criptografado, e protocolos cujo primeiro pacote é só um campo de comprimento e se confunde facilmente com outros, como MongoDB ou DNS over TCP); o ClientHello não pode ser interpretado; ou o cliente não envia nada e não há rota Tempo de espera. Sem destino padrão, essas conexões são encerradas e contadas como "encerradas". Um cliente que se desconecta antes de a detecção terminar é descartado.

## Observações

- Somente TCP. O tráfego UDP, como HTTP/3 (QUIC) ou WireGuard, fica fora do roteamento; use uma regra separada para ele;
- As variantes criptografadas de SIP, Redis, AMQP, IRC e protocolos semelhantes rodam sobre TLS; roteie-as com rotas TLS por domínio ou ALPN;
- O ClientHello pode ser dividido em vários registros TLS e ter até 64 KB; as linhas de requisição de HTTP, RTSP e SIP podem ter até 8 KB, e no HTTP até 32 KB de cabeçalhos de requisição são lidos para encontrar o Host;
- Cada conexão é detectada uma única vez, então as requisições seguintes na mesma conexão HTTP keep-alive vão para o backend escolhido na primeira vez;
- O cartão da regra mostra quantas conexões cada rota e o destino padrão receberam; as entradas de depuração do log de execução registram o protocolo, o SNI, o ALPN ou o Host detectado em cada conexão;
- Alterações em rotas, destino padrão e tempo de detecção valem na hora, sem parar a escuta, e as conexões abertas continuam.

## Exemplo: substituindo o haproxy

Uma configuração de haproxy como esta:

```text
frontend https
    bind 192.0.2.10:443
    mode tcp
    tcp-request inspect-delay 5s
    tcp-request content accept if { req_ssl_hello_type 1 }
    use_backend ocserv if { req_ssl_sni -i tz.example.com }
    default_backend web
backend ocserv
    server ocserv 127.0.0.1:443 send-proxy-v2
backend web
    server web 127.0.0.1:8443 send-proxy-v2
```

vira uma única regra do Gately:

| Configuração | Valor |
| --- | --- |
| Protocolo | TCP |
| Endereço / porta de escuta | `192.0.2.10` / `443` |
| Rota `ocserv` | protocolo TLS, domínio `tz.example.com`, destino `127.0.0.1`, porta `443`, PROXY v2 |
| Destino padrão | `127.0.0.1`, porta de destino `8443`, PROXY v2 |

Defina o endereço de escuta como o endereço público do servidor para evitar conflito com backends que escutam em `127.0.0.1:443`. Para servir sites com o próprio Gately, adicione um listener de [Serviços Web](/pt-BR/docs/modules/web) em `127.0.0.1:8443` com "Aceitar protocolo PROXY" ativado e `127.0.0.0/8` nas origens PROXY confiáveis. Pare o haproxy antes de ativar a regra, pois apenas um programa pode escutar na porta.

## Exemplo: substituindo o sslh

Estes argumentos do sslh:

```text
sslh --listen 0.0.0.0:443 --timeout 2 \
     --ssh 127.0.0.1:22 --openvpn 127.0.0.1:1194 --tls 127.0.0.1:8443
```

viram uma única regra do Gately:

| Configuração | Valor |
| --- | --- |
| Protocolo | TCP |
| Endereço / porta de escuta | vazio / `443` |
| Tempo de detecção | `2` |
| Rota `ssh` | protocolo SSH, destino `127.0.0.1`, porta `22` |
| Rota `vpn` | protocolo OpenVPN, destino `127.0.0.1`, porta `1194` |
| Rota `web` | protocolo TLS, sem domínios, destino `127.0.0.1`, porta `8443` |
| Rota `vnc` (opcional) | protocolo Tempo de espera, destino `127.0.0.1`, porta `5900` |

Por padrão, o sslh envia as conexões ao SSH quando o tempo limite expira. O Gately reconhece o SSH na hora pela string de versão `SSH-`, então o tempo limite pode levar a um serviço em que o servidor envia primeiro, como o VNC. Você pode continuar adicionando rotas RDP, SOCKS e HTTP ou dividir o TLS entre vários backends por domínio.

## Atualizando a partir do redirecionamento de portas

O roteamento por protocolo era antes o modo "Roteamento SNI" das regras de redirecionamento de portas. Na primeira inicialização após a atualização, as regras de roteamento SNI do redirecionamento de portas passam automaticamente para o módulo Roteamento por protocolo: cada regra mantém sua chave, nome, estado de ativação, grupo e todas as rotas, e as rotas antigas se tornam rotas TLS. Se o módulo de redirecionamento de portas estava desativado, o Roteamento por protocolo também fica desativado, e os tokens de API com acesso ao redirecionamento de portas recebem o mesmo nível de acesso ao Roteamento por protocolo.

Depois da migração, gerencie essas regras por `/api/protoroute/rules`: o campo de rotas passa de `sniRoutes` para `routes` e o campo `mode` deixa de ser usado; regras de roteamento enviadas para `/api/portforward/rules` são recusadas. As métricas agora são `gately_protoroute_*`; veja [Monitoramento](/pt-BR/docs/reference/monitoring).

## Dica de implantação

O roteamento por protocolo escuta diretamente nas portas do host; em implantações com Docker, recomendamos `--network host`.

## Grupos e ordenação

As regras podem ter um "Grupo". Agrupamento, ordenação e busca funcionam como no [Redirecionamento de portas](/pt-BR/docs/modules/portforward#grupos-e-ordenacao); a caixa de busca filtra por nome, grupo, porta de escuta, destino, domínio ou protocolo da rota.
