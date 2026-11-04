# Diagnóstico de rede

Execute Ping, traceroute, sondagem de portas, consulta DNS, teste de velocidade e varredura da rede local no navegador. Local no menu: "Ferramentas → Diagnóstico de rede" na barra lateral.

## Tarefas em segundo plano

Cada clique em "Iniciar" cria uma tarefa em segundo plano. A API retorna o ID da tarefa imediatamente, e a interface consulta a saída periodicamente.

- No máximo 4 tarefas rodam ao mesmo tempo; além disso, aparece uma mensagem de "tente novamente mais tarde";
- Uma tarefa roda por no máximo 5 minutos e é cancelada automaticamente ao estourar o tempo;
- As 50 tarefas mais recentes são mantidas, e tarefas em execução nunca são limpas; cada tarefa mantém no máximo 500 linhas de saída;
- Uma tarefa em execução pode ser cancelada com "Parar". "Tarefas recentes", na parte inferior da página, lista as tarefas anteriores, e clicar em uma mostra a sua saída;
- Estados da tarefa: em execução, concluída, com falha, cancelada.

## Ping

Envia requisições ICMP echo ao destino e exibe cada tempo de ida e volta, a taxa de perda de pacotes e a latência mínima / média / máxima.

| Parâmetro | Descrição |
| --- | --- |
| Destino | Hostname ou IP, com suporte a IPv4 e IPv6 |
| Quantidade | Padrão 4, até 20 |

Cada requisição expira em 2 segundos, com intervalo de cerca de 0,7 segundo. Um socket ICMP no estilo UDP sem privilégios é tentado primeiro, depois um raw socket. Dentro de um contêiner Docker, isso exige a capability `NET_RAW` ou o ajuste `net.ipv4.ping_group_range`; se as permissões forem insuficientes, use a sondagem de portas. Se o destino não responder de forma alguma, a tarefa termina como falha.

## Traceroute

Chama um comando do sistema para rastrear o caminho e exibe o resultado bruto do comando.

- O Windows usa `tracert`; os demais sistemas preferem `traceroute` e, em seguida, `tracepath`;
- No máximo 20 saltos, sem resolução reversa (os IPs são exibidos diretamente);
- Se o sistema não tiver nenhum comando correspondente, a tarefa informa um erro. Em imagens Docker, você pode instalar o pacote `traceroute`.

## Sondagem de portas

Faz tentativas de conexão TCP ao destino e lista apenas as portas abertas.

| Parâmetro | Descrição |
| --- | --- |
| Destino | Hostname ou IP |
| Portas | Por exemplo `22,80,443,8000-8100`; listas separadas por vírgula e intervalos são aceitos, com portas de 1 a 65535 |

- No máximo 1024 portas por sondagem;
- Concorrência de 100, com timeout de conexão de 1,5 segundo por porta;
- Portas comuns exibem o nome do serviço (ssh, http, https, mysql, redis, postgres, rdp etc.) e o tempo de conexão.

## Consulta DNS

| Parâmetro | Descrição |
| --- | --- |
| Domínio / IP | O nome a consultar; informe um IP para o tipo `PTR` |
| Tipo | `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `PTR`; padrão `A` |
| Servidor DNS | Deixe em branco para usar o resolvedor do sistema; é possível informar `223.5.5.5`, `223.5.5.5:5353` ou um endereço DoH como `https://dns.alidns.com/dns-query` |

As consultas expiram em 8 segundos e exibem os registros e o tempo gasto.

## Teste de velocidade

Um teste de velocidade de download com 4 fluxos simultâneos. Ele imprime a taxa atual (Mbps) a cada segundo e, no final, imprime o total baixado e a taxa média.

| Parâmetro | Descrição |
| --- | --- |
| URL de download | Uma URL completa `http(s)://`. Deixe em branco para usar o endpoint de teste de velocidade da Cloudflare (`https://speed.cloudflare.com/__down?bytes=50000000`). A lista suspensa oferece arquivos de teste públicos comuns da Cloudflare, Vultr (Tóquio, Singapura, Los Angeles), Hetzner (Singapura, Alemanha) e OVH (França), e qualquer outra URL pode ser digitada. Um arquivo baixado por completo é pedido de novo até o fim da duração |
| Duração | Em segundos, de 2 a 30 na interface, padrão 8 |

As requisições do teste de velocidade não seguem redirecionamentos, então não podem ser desviadas para endereços cuja sondagem está bloqueada.

## Varredura da rede local

Varre uma sub-rede em busca de dispositivos online e mostra os resultados em uma tabela: IP, MAC, hostname, portas abertas.

| Parâmetro | Descrição |
| --- | --- |
| Sub-rede | CIDR, por exemplo `192.168.1.0/24`. Deixe em branco para usar a sub-rede da primeira interface IPv4 privada desta máquina (a lista suspensa mostra as sub-redes locais disponíveis, e sub-redes grandes demais são reduzidas para /24) |

Limites e funcionamento:

- Somente sub-redes IPv4 privadas são compatíveis (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`);
- No máximo 1024 endereços por varredura, o equivalente a uma /22; endereços de rede e de broadcast são excluídos automaticamente;
- Para cada endereço, as portas comuns 80, 443, 22, 445, 139, 3389, 8080, 62078, 5000 e 53 são sondadas, e uma conexão bem-sucedida ou recusada conta como online;
- Os endereços MAC vêm da tabela de vizinhos do sistema, e um host que tem entrada na tabela de vizinhos, mas nenhuma porta comum aberta, também é listado como online;
- Os hostnames são resolvidos por DNS reverso (timeout de 1,5 segundo);
- Para dispositivos com MAC, clique em "Adicionar ao Wake-on-LAN" para incluí-los na lista de dispositivos do [Wake-on-LAN](/pt-BR/docs/modules/tools).

## Lista de bloqueio de sondagem

"Lista de bloqueio de sondagem", no canto superior direito da página, define os destinos que as ferramentas de diagnóstico não podem acessar.

- Os endereços de metadados de provedores de nuvem são sempre bloqueados: `169.254.169.254/32`, `100.100.100.200/32`, `fd00:ec2::254/128`, o que impede que as ferramentas sejam usadas para roubar credenciais de instâncias;
- Você pode adicionar IPs ou sub-redes, um por linha, como `10.0.0.0/8`;
- Se qualquer endereço para o qual o hostname de destino resolva coincidir com a lista, a tarefa falha com um erro. A URL de download do teste de velocidade também é verificada, e as varreduras ignoram endereços correspondentes.

::: tip Dica
As ferramentas de diagnóstico rodam no host ou contêiner em que o Gately está, então os resultados refletem a rede desse ambiente. Para limitar o que pode ser sondado, adicione sub-redes internas sensíveis à lista de bloqueio de sondagem.
:::

## Autodiagnóstico de rede

O botão "Autodiagnóstico" na página de visão geral (também disponível no assistente de configuração inicial) verifica de uma só vez os problemas mais comuns de rede e de configuração. Os itens com problema aparecem primeiro, com sugestões de correção:

| Verificação | Descrição |
| --- | --- |
| Endereço público | Obtém o IPv4 / IPv6 de saída pelas URLs de consulta de IP público usadas pelo DDNS |
| NAT | Se o host tem diretamente um IPv4 / IPv6 público; quando uma interface de rede tem um endereço em 100.64.0.0/10, avisa que pode haver NAT da operadora (interfaces de redes overlay como o Tailscale são ignoradas) |
| Hora do sistema | Estima o desvio do relógio pelo horário de resposta do serviço de consulta; mais de 60 segundos fazem falhar os códigos de dois fatores e a validação de certificados |
| Portas 80 / 443 | Se estão livres, já ocupadas ou sem permissão para escutar |
| Resolução DDNS | Se o domínio dos registros com endereço dinâmico resolve atualmente para o endereço de saída deste host. Primeiro consulta o DNS local; se ele não retornar o endereço deste host, consulta também DNS públicos (223.5.5.5, 119.29.29.29, 1.1.1.1, 8.8.8.8) e a verificação passa se qualquer um retornar, evitando alertas falsos por timeout ocasional ou cache antigo do DNS local. Quando nenhum DNS responde, indica falha na consulta, separada de registro inexistente |
| Certificados | Certificados expirados ou que expiram em até 14 dias |
| Certificados dos sites HTTPS | Em listeners HTTPS ativados, quais sites têm domínios sem certificado correspondente (esses domínios falham no handshake TLS) |
| Conflitos do frp | Proxies com nome repetido no frpc e conflitos de nome de proxy, domínio ou porta remota relatados pelo frps nos logs dos últimos 30 minutos (comum quando vários dispositivos copiam a mesma configuração do frpc) |
| Estado dos módulos | Módulos em estado de aviso ou erro na visão geral |

O autodiagnóstico acessa os serviços de consulta de IP público configurados, só roda quando você clica e não está disponível para contas somente leitura nem tokens de API.
