# Comparativo de desempenho

Esta página compara o Gately com as ferramentas comuns que ele pode substituir, na mesma máquina e com os mesmos limites de CPU: proxy reverso, autenticação web, WAF, prevenção de intrusão, Port Knocking, DNS dinâmico e o comportamento em um roteador OpenWrt. Todos os números vêm de medições feitas em novembro de 2026. Os scripts de teste, as configurações e os resultados brutos estão no diretório [tools/benchmark](https://github.com/wangwanjie/gately/tree/main/tools/benchmark) do repositório, e é possível reproduzi-los.

<script setup>
const H = 'Quanto maior, melhor'
const L = 'Quanto menor, melhor'
</script>

## Resumo

- **Proxy reverso**: nginx e HAProxy, escritos em C, têm a maior vazão: cerca de 3.5 vezes a do Gately em HTTP e cerca de 2.6 vezes em HTTPS. Entre os gateways escritos em Go, o Gately está no primeiro grupo: a vazão HTTPS é 43% maior que a do Caddy, 2.3 vezes a do Lucky e 9% menor que a do Traefik; a taxa de novas conexões TLS é a mais alta entre os 7 programas.
- **Sem perda de velocidade com muitos sites**: com 1000 sites, a vazão do Gately cai apenas 6%, superando HAProxy, Traefik e Caddy e ficando atrás somente do nginx.
- **Autenticação web**: nas requisições de usuários já autenticados, o Gately chega a 1.9 vez o nginx + Authelia e a 3.7 vezes o Caddy + Authelia, com um terço da memória deles.
- **WAF**: com o mesmo OWASP CRS 4.25, o Gately processa requisições comuns cerca de 70% mais rápido que o nginx + ModSecurity, com resultados de bloqueio idênticos; ao inspecionar o corpo de requisições POST, o ModSecurity é mais rápido e usa muito menos memória.
- **Prevenção de intrusão**: com 10,000 IPs atacantes, o Gately termina de bloquear em 12 segundos e o fail2ban leva 51 segundos; com o mesmo volume de logs, a CPU fica abaixo da metade da do fail2ban. Ao ler arquivos de log, a latência de um bloqueio individual é cerca de 0.2 segundo maior que a do fail2ban.
- **Port Knocking**: a latência de batida do Gately e do fwknop fica abaixo de 10 ms, e depois da liberação o tráfego não passa pelo programa de batidas; o knock do fnOS encaminha o tráfego pelo seu gateway em um Linux comum, e a vazão com conexões curtas é de apenas 3% da de uma conexão direta.
- **Consumo de recursos**: para uma única função, a memória do Gately é maior que a de ferramentas pequenas como fail2ban, fwknop e ddns-go; quando um só Gately cuida ao mesmo tempo de proxy reverso, WAF, autenticação, DDNS, prevenção de intrusão e Port Knocking, ele ocupa cerca de 91 MiB, contra cerca de 204 MiB no total com ferramentas independentes. Veja [Tudo em um](#tudo-em-um).

## Ambiente de teste

| Item | Descrição |
| --- | --- |
| Host | Intel Core i5-13500T, 32 GB de memória, fnOS (KVM) |
| VM de teste de carga | Debian 13 (kernel 6.12), 8 vCPUs vinculadas a 8 threads de 4 núcleos de desempenho do host, 8 GB de memória, Docker 29 |
| Divisão de CPU | Programa testado nas vCPUs 0–1, nginx de origem nas vCPUs 2–3, ferramentas de carga nas vCPUs 4–7, sem competição entre si |
| Roteador | VM ImmortalWrt 25.12.1, 2 vCPUs (dois threads do mesmo núcleo de desempenho), 1 GB de memória |
| Ferramentas de carga | wrk 4.1, oha 1.17, h2load 1.64 (nghttp2) |
| Certificado | Todos os programas usam o mesmo certificado ECDSA P-256, TLS 1.3 |
| Origem | nginx retorna 1 KB de texto; 100 MB no teste de arquivo grande |

Versões testadas: Gately v0.6.8.1, nginx 1.29.8, HAProxy 3.2.26, Caddy 2.11.9, Traefik 3.7.19, Lucky 3.8.1, Nginx Proxy Manager 2.17.0 (OpenResty 1.31.8), Authelia 4.39.28, ModSecurity 3.0.16 (OWASP CRS 4.25.0), fail2ban 1.1.0, fwknop 2.6.11, o knock do fnOS fn-knock 2.5.11, ddns-go 6.17.9. Exceto fail2ban e fwknop, que usam pacotes do Debian, tudo roda a partir de imagens oficiais do Docker com rede em modo host.

**Critérios de medição**

- Vazão: cada item roda por 20 segundos, em 3 rodadas, e usa-se a mediana. HTTP e HTTPS usam o wrk com 128 conexões persistentes; novas conexões TLS usam o oha com 64 em paralelo e uma conexão nova por requisição; HTTP/2 usa o h2load com 64 conexões e 10 fluxos simultâneos em cada uma.
- Memória: soma do PSS de todos os processos do contêiner testado (a memória compartilhada entre processos é dividida proporcionalmente, sem contagem dupla). «Ocioso» é o valor 10 segundos depois de concluídos a inicialização e a configuração; «pico» é o máximo durante o teste de carga.
- CPU: tempo de CPU do cgroup do contêiner testado, em que 100% equivale a uma vCPU.
- Inicialização: reinicia-se o programa com a configuração pronta, mede-se o tempo até a primeira requisição ser respondida corretamente e usa-se a mediana de 3 execuções.
- Todos os programas estão com o log de acesso desativado. O nginx tem o pool de conexões persistentes para a origem ativado e o limite de requisições por conexão ampliado (ajuste comum); os demais programas usam a configuração padrão.

## Proxy reverso

O domínio `bench.test` é encaminhado por proxy reverso à origem, com HTTP na porta 8080 e HTTPS na porta 8443 (o Nginx Proxy Manager só pode usar 80/443).

<BenchBars title="Requisições HTTPS por segundo (conexões persistentes)" unit="QPS" :better-label="H" :items="[['nginx',85434],['HAProxy',83953],['Traefik',35812],['Nginx Proxy Manager',32816],['Gately',32764],['Caddy',22871],['Lucky',14376]]" />

<BenchBars title="Novas conexões TLS por segundo (um handshake completo por requisição)" unit="conex./s" :better-label="H" :items="[['Gately',4636],['Traefik',4482],['Nginx Proxy Manager',4015],['Caddy',3987],['nginx',3796],['Lucky',3621],['HAProxy',3293]]" />

<BenchBars title="Memória ociosa" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['nginx',15.7],['HAProxy',36.4],['Gately',47.6],['Caddy',48.8],['Traefik',89.0],['Lucky',124.6],['Nginx Proxy Manager',155.7]]" note="Os 47.6 MiB do Gately incluem todos os módulos de funções (DDNS, certificados, WAF, prevenção de intrusão etc. rodam no mesmo processo); o Lucky tem 2 processos e o Nginx Proxy Manager tem 22 (nginx, o backend em Node.js etc.)." />

| Programa | HTTP | HTTPS | HTTP/2 | TLS novas/s | Arquivo de 100 MB | CPU a 5000 QPS | Memória ociosa | Pico sob carga | Por conexão ociosa | Tempo de reinício | Imagem |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 35,364 | 32,764 | 21,591 | **4,636** | 4,157 MB/s | 67% | 47.6 MiB | 107 MiB | 21.2 KiB | 0.40 s | 229 MB |
| nginx | **123,343** | **85,434** | 78,110 | 3,796 | 2,161 MB/s | 55% | **15.7 MiB** | **25 MiB** | **0.6 KiB** | **0.32 s** | 93 MB |
| HAProxy | 117,923 | 83,953 | **90,633** | 3,293 | 3,912 MB/s | **52%** | 36.4 MiB | 56 MiB | 1.0 KiB | 0.33 s | 58 MB |
| Caddy | 24,459 | 22,871 | 11,746 | 3,987 | 4,116 MB/s | 68% | 48.8 MiB | 132 MiB | 27.8 KiB | 0.45 s | 92 MB |
| Traefik | 39,112 | 35,812 | 19,606 | 4,482 | **4,311 MB/s** | 66% | 89.0 MiB | 253 MiB | 24.4 KiB | 0.53 s | 250 MB |
| Lucky | 15,150 | 14,376 | 10,261 | 3,621 | 3,963 MB/s | 80% | 124.6 MiB | 193 MiB | 26.2 KiB | 3.47 s | 58 MB |
| Nginx Proxy Manager | 45,257 | 32,816 | 34,710 | 4,015 | 2,150 MB/s | 58% | 155.7 MiB | 215 MiB | pré-alocada | 1.50 s | 1,959 MB |

- HTTP, HTTPS e HTTP/2 estão em requisições por segundo. «CPU a 5000 QPS» é a CPU usada pelo programa testado sob uma carga fixa de 5000 requisições por segundo; quanto menor, mais econômico em energia.
- Por padrão, o nginx fecha a conexão após 1000 requisições, enquanto o h2load não reconecta; por isso, na coluna HTTP/2, esse limite foi ampliado no nginx e no Nginx Proxy Manager (no Nginx Proxy Manager, só no teste HTTP/2, por configuração personalizada; os demais testes mantiveram o padrão).
- Na coluna de arquivo grande, nginx e Nginx Proxy Manager ficam mais baixos porque o nginx, por padrão, grava primeiro em um arquivo temporário em disco as respostas que excedem o buffer de memória.
- «Por conexão ociosa» é a memória adicional ao manter 10,000 conexões persistentes ociosas, dividida pelo número de conexões. Programas em Go têm buffers de leitura e escrita próprios e uma goroutine por conexão, cerca de 20–28 KiB; nginx e HAProxy usam menos de 1 KiB. O Nginx Proxy Manager pré-aloca uma tabela de conexões conforme sua configuração, por isso não foi possível medir o aumento.
- A imagem do Gately é maior porque inclui a CLI do Docker e as ferramentas nftables e iptables; o programa em si é um único arquivo de 40 MB, e o pacote de download tem 18 MB.

## 1000 sites

Cada programa é configurado com 1000 sites de proxy reverso com domínios diferentes, e no teste de carga cada requisição escolhe um domínio aleatório.

<BenchBars title="Requisições HTTP por segundo com 1000 sites" unit="QPS" :better-label="H" :items="[['nginx',122958],['Gately',33211],['Traefik',24229],['HAProxy',24057],['Caddy',9819]]" />

| Programa | Requisições/s | Em relação a 1 site | Latência p99 | Reinício até o último site ficar disponível | Memória ociosa | Pico sob carga |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 33,211 | -6% | 29.8 ms | **0.43 s** | 94 MiB | 463 MiB |
| nginx | **122,958** | **0%** | **1.2 ms** | 0.54 s | 57 MiB | **58 MiB** |
| HAProxy | 24,057 | -80% | 30.0 ms | **0.37 s** | **51 MiB** | **58 MiB** |
| Caddy | 9,819 | -60% | 52.7 ms | 0.80 s | 80 MiB | 361 MiB |
| Traefik | 24,229 | -38% | 18.8 ms | 0.74 s | 115 MiB | 157 MiB |

- O HAProxy compara regra por regra com `use_backend ... if { hdr(host) -i domínio }`, e por isso fica mais lento quanto mais sites existem; um arquivo map evita isso, mas aqui se mantém a forma comum de «uma regra por site».
- O pico de memória do Gately vem principalmente das estatísticas de acesso de cada site: na v0.6.8.1, cada site com tráfego guarda de uma vez um bloco inteiro de estatísticas por minuto de 24 horas e quantis de latência (cerca de 108 KiB). A versão de desenvolvimento as guarda em blocos por hora, o que reduz o pico para 225 MiB; veja [Otimizações da versão de desenvolvimento](#otimizacoes-da-versao-de-desenvolvimento).

## Autenticação web

O site exige login, e o teste cobre as requisições de usuários autenticados e o próprio login. O Gately usa a autenticação web e a autenticação básica do próprio site; o grupo de comparação é nginx ou Caddy com o Authelia na frente (`auth_request` / `forward_auth`), além do `auth_basic` do próprio nginx. Tudo passa por HTTPS.

<BenchBars title="Requisições por segundo de usuários autenticados (com cookie de sessão)" unit="QPS" :better-label="H" :items="[['Gately',26726],['nginx + Authelia',14291],['Lucky',10786],['Caddy + Authelia',7141]]" />

| Configuração | Requisições autenticadas/s | Sem autenticação | Autenticação básica (bcrypt) | Logins bem-sucedidos/s | Pico de memória no login | Memória ociosa |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **26,726** | 32,520 | **28,598** | **38.3** | **70 MiB** | **45 MiB** |
| nginx + Authelia | 14,291 | 83,474 | 40 | 13.4 | 1,082 MiB | 120 MiB |
| Caddy + Authelia | 7,141 | 22,703 | 21,213 | 13.4 | 1,271 MiB | 149 MiB |
| Lucky | 10,786 | 13,841 | 10,107 | ver notas | 209 MiB | 127 MiB |

- Hash de senhas: a autenticação básica do Gately e a de nginx/Caddy usam bcrypt (cost 10), o Authelia usa argon2id (t=3, 64 MiB, p=4) e o Lucky guarda em texto simples as senhas dos usuários do site. Cada login no Authelia ocupa 64 MiB de memória, e com 8 logins simultâneos a memória passa de 1 GB.
- O `auth_basic` do nginx recalcula o bcrypt a cada requisição, por isso chega a apenas 40 requisições/s; Gately e Caddy guardam em cache o resultado da verificação. Com apr1 (MD5), o nginx chega a 13,082 requisições/s.
- Senhas incorretas: o Gately recusa diretamente uma origem após 10 falhas (processa 16,000 requisições incorretas por segundo, e a CPU deixa de calcular hashes); o Authelia aplica a cada falha um atraso fixo de cerca de 1 segundo. Durante o bloqueio, ambos recusam também a senha correta dessa origem, e as sessões já autenticadas não são afetadas.
- Com 8 logins simultâneos, o Lucky retornou `create webauth session error` em todos os casos, e por isso não foi possível medir a vazão de login.

## WAF

Os três usam OWASP CRS 4.25.0, nível de paranoia 1 e modo de bloqueio. Gately e Caddy usam o mesmo mecanismo Coraza, e o nginx usa o ModSecurity v3.

<BenchBars title="Requisições GET por segundo com o WAF ativado" unit="QPS" :better-label="H" :items="[['Gately',2498],['Caddy + Coraza',2265],['nginx + ModSecurity',1491]]" />

| Programa | GET | GET com 8 parâmetros | POST de formulário de 4 KB | Tráfego de ataque | Pico de memória sob carga | Memória ociosa (ligado/desligado) |
| --- | --- | --- | --- | --- | --- | --- |
| **Gately** | **2,498** | **1,067** | 72 | **1,218** | 754 MiB | 88 / 48 MiB |
| nginx + ModSecurity | 1,491 | 591 | **102** | 1,014 | **73 MiB** | 62 / 24 MiB |
| Caddy + Coraza | 2,265 | 943 | 46 | 1,004 | 792 MiB | 96 / 55 MiB |

| Eficácia de detecção | Gately | nginx + ModSecurity | Caddy + Coraza |
| --- | --- | --- | --- |
| Amostras de ataque bloqueadas (557) | 77.2% | 77.2% | 77.2% |
| Falsos positivos em amostras normais (389) | 20.1% | 20.1% | 20.1% |
| Casos de regressão do CRS detectados (2049) | 95.9% | 95.2% | 95.7% |

- As regras do CRS são pesadas por natureza: com o WAF ativado, a vazão dos três cai para apenas 1%–10% do valor com ele desativado; para sites que precisam de WAF, recomenda-se ativá-lo somente nas interfaces dinâmicas.
- Ao inspecionar o corpo de requisições POST, o mecanismo Coraza (tanto no Gately quanto no Caddy) passa de 700 MiB de memória no pico, enquanto o ModSecurity se mantém em 73 MiB; em dispositivos com pouca memória, convém ter cuidado ao ativar a inspeção do corpo.
- As amostras vêm do gotestwaf (MIT), dos testes de regressão que acompanham o CRS (Apache-2.0) e de requisições normais escritas à mão; os falsos positivos são principalmente artigos técnicos com palavras-chave de SQL/shell e interfaces PUT/DELETE (o CRS não permite esses métodos por padrão).

## Prevenção de intrusão

Cenário: um serviço web é escaneado e o IP é bloqueado após 5 respostas 404 em 60 segundos (nftables). O fail2ban lê o log de acesso do nginx com pyinotify; o Gately lê o mesmo log com uma regra de arquivo de log equivalente, e outro grupo mede a detecção de caminhos embutida nos Serviços Web do Gately.

<BenchBars title="Tempo para bloquear todos os 10,000 IPs atacantes" unit="s" better="lower" :digits="1" :better-label="L" :items="[['Gately',12.3],['fail2ban',51.2]]" />

| Métrica | Gately (leitura de arquivo de log) | Gately (detecção embutida no serviço web) | fail2ban |
| --- | --- | --- | --- |
| Latência de bloqueio de um único atacante p50 / p99 | 478 / 996 ms | **10 / 13 ms** | 280 / 864 ms |
| CPU com 1000 linhas/s de log | **2.9%** | — | 20.4% |
| CPU com 20,000 linhas/s de log | **35.7%** | — | 83.2% |
| 10,000 IPs atacantes: todos bloqueados | **12.3 s** | — | 51.2 s |
| 10,000 IPs atacantes: CPU consumida | **10.1 s de CPU** | — | 60.3 s de CPU |
| Memória ociosa | 44.1 MiB | 40.7 MiB | **17.6 MiB** |
| Não bloqueados / bloqueados por engano | 0 / 0 | 0 / 0 | 0 / 0 |

- O Gately lê o arquivo de log uma vez por segundo, por isso a latência de um bloqueio individual fica entre 0 e 1 segundo e, em média, é cerca de 0.2 segundo mais lenta que o inotify do fail2ban; a detecção embutida no serviço web conta diretamente ao processar a requisição e não tem esse atraso.
- O fail2ban inicia um processo `nft` a cada bloqueio, então forma-se fila quando há muitos atacantes: com 10,000 IPs, o último foi bloqueado 41 segundos depois do fim da injeção.
- O fail2ban em si tem 2 MB, mas depende de um ambiente de execução Python de cerca de 24 MB.

## Port Knocking

Comparam-se três abordagens: o Port Knocking do Gately (autorização de pacote único SPA compatível com o fwknop, bloqueio com nftables), o fwknop 2.6.11 (também SPA, bloqueio com iptables) e o knock do fnOS fn-knock 2.5.11 (libera o IP de origem após um login na web, e o gateway dele encaminha o tráfego). O Gately também oferece uma entrada que libera o acesso após um login na web, medida como um grupo separado. O protegido é um serviço nginx em uma porta TCP, e o cliente usa um endereço de teste público, portanto não é afetado pela exceção «a rede local não precisa bater».

<BenchBars title="Requisições por segundo com conexões curtas pela porta protegida após a liberação" unit="QPS" :better-label="H" :items="[['fwknop',65046],['Gately',64763],['Gately liberação web',63556],['fn-knock',1886]]" note="Conectando direto à origem, sem nenhum programa de batidas, são 67,034 requisições/s." />

| Métrica | Gately (SPA) | Gately (liberação web) | fwknop | fn-knock |
| --- | --- | --- | --- | --- |
| Da batida até a porta ficar acessível p50 / p99 | 8.4 / 13.4 ms | 58 / 73 ms | 9.0 / 18.1 ms | 84 / 143 ms |
| Porta protegida sem autorização | Descartada (timeout) | Descartada (timeout) | Descartada (timeout) | Reset após o handshake (detectável) |
| Requisições/s com conexão persistente / curta após a liberação | 143,867 / 64,764 | 143,839 / 63,556 | 148,371 / 65,046 | 140,137 / 1,886 |
| CPU gasta no encaminhamento após a liberação (conexões curtas) | 2% | 1% | 0% | 134% |
| Batidas legítimas com 50,000 pacotes inválidos/s de uma única origem | **10/10** (CPU 26%) | — | 3/8 (CPU 102%) | — |
| Batidas legítimas com 10,000 pacotes inválidos/s de origens forjadas | 1/7 | — | **10/10** (CPU 54%) | — |
| Memória ociosa | 48.7 MiB | 48.7 MiB | **2.2 MiB** | 63.2 MiB (3 processos) |

- No Gately e no fwknop, após a liberação o firewall deixa o tráfego passar diretamente e ele não passa pelo programa de batidas, portanto a vazão é igual à de uma conexão direta; o fn-knock encaminha pelo gateway em um Linux comum e ainda consulta o plano de controle sobre a autorização a cada nova conexão, de modo que a vazão com conexões curtas é de apenas 3% da de uma conexão direta. Na versão fpk do fnOS, o fn-knock tem também um modo de conexão direta com o firewall; aqui foi medida a versão Docker.
- O login do fn-knock exige resolver antes uma prova de trabalho ALTCHA (neste teste, p50 de 28 ms para resolver); sem a resolução, são 50 ms.
- O fwknopd processa cada pacote em um único thread; com 50,000 pacotes/s de uma única origem ele satura um núcleo e as batidas legítimas começam a falhar. O Gately limita a taxa por origem, então uma inundação vinda de uma única origem quase não consome CPU.
- Problema conhecido: quando um atacante forja muitos endereços de origem diferentes, a tabela de limite de taxa por origem do Gately fica cheia, a cota de excedente compartilhada pelas novas origens se esgota com a inundação e a maioria das batidas legítimas falha (nesse momento a CPU do Gately está em apenas cerca de 19%). O fwknop não limita a taxa e não é afetado até a CPU saturar.

## DNS dinâmico

Cada programa é configurado com 20 tarefas DDNS e um período de verificação de 30 segundos. O IP público é consultado na origem local e as atualizações são enviadas à origem local por um «callback personalizado», sem tocar em nenhum provedor de DNS real. Depois da inicialização e da primeira sincronização, faz-se uma amostragem de 5 minutos.

<BenchBars title="Memória média com 20 tarefas DDNS" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['ddns-go',16.9],['Gately',51.6],['Lucky',131.3]]" />

| Programa | Memória média | Memória de pico | CPU média | Imagem |
| --- | --- | --- | --- | --- |
| ddns-go 6.17.9 | **16.9 MiB** | **18.2 MiB** | **0.04%** | 43 MB |
| Gately | 51.6 MiB | 55.4 MiB | 0.16% | 229 MB |
| Lucky 3.8.1 | 131.3 MiB | 134.9 MiB | 0.48% | 58 MB |

- Os três só consultam o provedor quando o IP muda ou quando chega o período de sincronização forçada, então o custo permanente é pequeno em todos. Quando só se precisa de DDNS, o ddns-go é o que usa menos memória; o número do Gately inclui todos os seus módulos, e o mesmo processo ainda pode executar certificados, proxy reverso e outras funções.

## Roteador OpenWrt

Cada programa roda em uma VM de roteador ImmortalWrt 25.12.1 (2 vCPUs, 1 GB de memória). A máquina de carga acessa o roteador por uma bridge da rede local, e o roteador encaminha por proxy reverso à origem na máquina de carga. nginx 1.26.3 e HAProxy 3.2.21 vêm do repositório de pacotes do ImmortalWrt, o Gately usa o pacote apk oficial do OpenWrt e o Lucky usa o binário oficial para Linux.

<BenchBars title="Requisições HTTPS por segundo no roteador (conexões persistentes)" unit="QPS" :better-label="H" :items="[['nginx',40665],['HAProxy',36266],['Gately',15669],['Lucky',8113]]" />

| Programa | HTTP | HTTPS | TLS novas/s | CPU a 2000 QPS | Memória ociosa | Pico sob carga | Tamanho do programa |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Gately** | 16,439 | 15,669 | 2,665 | 27% | 43 MiB | 65 MiB | 40.5 MB |
| nginx | **49,566** | **40,665** | **3,111** | **12%** | **15 MiB** | **22 MiB** | **0.8 MB** |
| HAProxy | 43,046 | 36,266 | 2,337 | 12% | 24 MiB | 29 MiB | 3.7 MB |
| Lucky | 8,642 | 8,113 | 2,021 | 44% | 64 MiB | 81 MiB | 17.9 MB |

- No roteador, o gargalo está principalmente no envio e recebimento de rede do kernel (interrupções de software), e nenhum programa usa os dois núcleos por completo, por isso as diferenças são menores que na máquina de carga. A vazão do Gately é cerca de 2 vezes a do Lucky, com um terço a menos de memória.
- O kernel do OpenWrt não tem smaps, então aqui a memória é estimada a partir de `/proc/<pid>/statm` (soma da memória residente privada de cada processo mais uma cópia da parte compartilhada). Durante o teste, a proteção padrão contra inundação de SYN do firewall (cerca de 25 novas conexões por segundo) foi desativada temporariamente; caso contrário, ela limitaria o teste de novas conexões TLS.
- Para apenas proxy reverso, o nginx do repositório de pacotes é o menor em tamanho e em memória; os 40 MB do Gately incluem todos os módulos, e em [build.gately.cn](https://build.gately.cn) é possível escolher só os módulos necessários para gerar um pacote de instalação menor. Em roteadores com pouca memória flash, convém conferir antes o espaço livre.

## Tudo em um

Um mesmo Gately com tudo isto ativado ao mesmo tempo: dois sites de proxy reverso (HTTP e HTTPS, ambos com WAF), um site com autenticação web, 20 tarefas DDNS, uma regra de log de prevenção de intrusão e uma regra de Port Knocking. O grupo de comparação é uma combinação de ferramentas independentes que oferecem as mesmas funções, somando a memória ociosa medida acima para cada uma.

<BenchBars title="Memória ociosa total para o mesmo conjunto de funções" unit="MiB" better="lower" :digits="1" :better-label="L" :items="[['Gately',90.8],['nginx + ModSecurity + Authelia + ddns-go + fail2ban + fwknop',203.5],['Nginx Proxy Manager + Authelia + ddns-go + fail2ban + fwknop',297.3]]" note="O grupo com Nginx Proxy Manager não tem WAF." />

| Função | Ferramenta independente | Memória ociosa | Tamanho de instalação |
| --- | --- | --- | --- |
| Proxy reverso + WAF | nginx + ModSecurity (CRS 4.25) | 61.9 MiB | Imagem de 156 MB |
| Proxy reverso (interface gráfica) | Nginx Proxy Manager | 155.7 MiB | Imagem de 1,959 MB |
| Autenticação web | Authelia | 104.4 MiB | Imagem de 110 MB |
| DNS dinâmico | ddns-go | 17.4 MiB | Imagem de 43 MB |
| Prevenção de intrusão | fail2ban | 17.6 MiB | 24 MB (com Python) |
| Port Knocking | fwknop | 2.2 MiB | 0.3 MB |
| **Tudo** | **Gately** | **90.8 MiB** | **Programa de 40 MB (download de 18 MB)** |

- A instância tudo em um do Gately, com 1000 requisições/s de tráfego HTTPS passando pelo WAF, tem pico de memória de 114 MiB e CPU de 71%; as 20 tarefas DDNS foram todas sincronizadas com sucesso.
- A emissão de certificados (certbot, acme.sh) só roda na renovação e não entra na memória residente.

## Otimizações da versão de desenvolvimento

Com base nos resultados acima, a versão de desenvolvimento (não lançada, posterior ao commit `3d579389`) recebeu as mudanças a seguir e foi testada de novo no mesmo ambiente. O proxy reverso usa o build do commit `3d579389`; os 1000 sites e a prevenção de intrusão usam um build que inclui as mudanças posteriores. A coluna de referência é a v0.6.8.1 medida de novo no mesmo dia (nesse dia só as linhas de HTTP e HTTPS foram remedidas; as demais referências vêm dos dados acima).

- Sites do mesmo ouvinte que fazem proxy reverso para o mesmo backend com as mesmas configurações de conexão compartilham um pool de conexões.
- Com pelo menos 1 GiB de memória disponível, o valor padrão do percentual de GC passa de 100 para 200 (pode ser alterado nas configurações; dispositivos com pouca memória mantêm 100).
- O proxy reverso copia a requisição uma vez a menos e cria três valores de contexto a menos por requisição.
- As estatísticas por minuto de 24 horas de cada site são guardadas em blocos por hora alocados no primeiro uso, e os campos dos buckets passam a contadores de 32 bits (um site com tráfego em apenas uma hora cai de cerca de 108 KiB para cerca de 5 KiB, e um com tráfego o dia todo ocupa cerca de 50 KiB).
- A prevenção de intrusão lê os arquivos de log com inotify, ou seja, lê assim que algo é escrito (a verificação a cada segundo continua como reserva); as gravações de bloqueios no firewall são agrupadas no máximo uma vez a cada 250 milissegundos.

| Métrica | v0.6.8.1 | Versão de desenvolvimento | Variação |
| --- | --- | --- | --- |
| Requisições HTTP/s (referência do mesmo dia) | 35,968 | 38,358 | +6.6% |
| Requisições HTTPS/s (referência do mesmo dia) | 34,072 | 36,079 | +5.9% |
| Novas conexões TLS/s | 4,636 | 4,942 | +6.6% |
| Requisições HTTP/2/s | 21,591 | 23,788 | +10% |
| 1000 sites: requisições/s | 33,211 | 37,423 | +13% |
| 1000 sites: latência p99 | 29.8 ms | 12.9 ms | -57% |
| 1000 sites: pico de memória durante o teste | 463 MiB | 225 MiB | -51% |
| Pico de memória no teste HTTP | 64.5 MiB | 81.1 MiB | +26% |
| Prevenção de intrusão: latência de bloqueio de um único atacante p50 / p99 | 478 / 996 ms | 9.8 / 129 ms | |
| Prevenção de intrusão: latência de bloqueio sob ataque contínuo p50 / p99 (20,000 linhas/s) | 509 / 1006 ms | 143 / 269 ms | |
| Prevenção de intrusão: CPU a 1000 linhas/s | 2.9% | 8.0% | maior |
| Prevenção de intrusão: CPU a 20,000 linhas/s | 35.7% | 26.5% | -26% |
| Prevenção de intrusão: bloqueio dos 10,000 IPs atacantes | 12.3 s, 10.1 s de CPU | 10.1 s, 2.5 s de CPU | |

- O ganho de vazão vem principalmente da configuração de GC, ao custo de um pico de memória cerca de uma dezena de MiB maior no teste. O profiling mostra que quase todo o custo restante no caminho do proxy está na implementação HTTP da biblioteca padrão do Go e no GC; a diferença para nginx e HAProxy está na linguagem e no runtime.
- Com 1000 sites, a vazão sobe e a latência cai porque o pool de conexões compartilhado reduz muito as conexões com a origem; o pico de memória cai pela metade graças ao armazenamento das estatísticas em blocos.
- A latência de um bloqueio isolado agora é menor que a do fail2ban (280 / 864 ms). Com poucas linhas de log por segundo, a CPU é maior que na versão anterior porque o firewall é gravado no máximo 4 vezes por segundo, contra 1 vez antes; ainda fica em quatro décimos da do fail2ban (20.4%).

## Reproduzir

```bash
git clone https://github.com/wangwanjie/gately && cd gately/tools/benchmark
# Execute como root em uma VM Debian 13; requer Docker, wrk, oha e h2load
bash provision-debian.sh
python3 lib/run.py gately 3     # proxy reverso; troque por nginx, haproxy, caddy, traefik, lucky, npm
bash auth/run.sh                # autenticação web
bash waf/run.sh                 # WAF
bash guard/run.sh               # prevenção de intrusão
bash knock/run.sh               # Port Knocking
bash scale/run.sh 1000          # 1000 sites
bash ddns/run.sh 20             # DNS dinâmico
bash bundle/run.sh              # tudo em um
bash router/run.sh              # roteador (requer ainda uma máquina OpenWrt)
```

O README de cada diretório descreve a configuração, os critérios de medição e as limitações conhecidas. Os resultados dependem do hardware, da virtualização e da configuração, por isso recomenda-se repetir os testes com a sua própria carga de trabalho.
