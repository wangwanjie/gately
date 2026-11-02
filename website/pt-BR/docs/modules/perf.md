# Desempenho

Desempenho reúne em uma página os dados do host, do processo do Gately e de cada site, e envia um alerta pela [Central de notificações](/pt-BR/docs/modules/notify) quando uma métrica fica acima do limite, seguido de um aviso de recuperação quando a condição deixa de valer. Fica na barra lateral, em Sistema → Desempenho.

Ele cobre casos como uma CPU no máximo de madrugada, um pico de conexões em um site ou um backend que fica lento sem ninguém perceber, e guarda os dados só na memória. Para histórico de mais de 24 horas, agregação entre instâncias ou consultas complexas, use [Prometheus e Grafana](/pt-BR/docs/reference/monitoring).

## Painel

Alterne entre 60 segundos, 1 hora e 24 horas no canto superior direito da página.

### Host e processo do Gately

| Gráfico | Conteúdo |
| --- | --- |
| CPU do host | Uso médio de todos os núcleos da máquina |
| Memória / disco do host | Memória usada em relação à memória física (cache recuperável não contado), parte usada da partição do diretório de configuração |
| Carga do sistema | Carga de 1 minuto |
| CPU do processo do Gately | Parte da máquina inteira usada pelo processo do Gately, medida como a CPU do host |
| Memória do processo do Gately | Memória residente (MB) |
| Descritores de arquivo / goroutines | Descritores de arquivo abertos e goroutines Go do processo |

Uma amostra é coletada a cada 5 segundos: o intervalo de 60 segundos mostra cada amostra; o de 1 hora, um ponto por minuto (a média do minuto); o de 24 horas, um ponto a cada 5 minutos. A linha tracejada dos gráficos de CPU é o pico de cada ponto, para que picos curtos não sumam na média. Passe o mouse sobre um gráfico para ver os valores de um instante.

Algumas plataformas não fornecem todas as métricas (por exemplo, no macOS não há uso de CPU); esses gráficos mostram "Sem dados".

### Sites

Com o módulo Serviços web ligado, todos os sites em execução (incluindo os sites padrão) são listados:

| Coluna | Significado |
| --- | --- |
| Em andamento | Requisições que o site está atendendo agora, incluindo conexões longas como WebSocket |
| Requisições/s | Média de requisições por segundo em aproximadamente os últimos 60 segundos |
| Proporção de 5xx | Respostas 5xx como porcentagem das respostas concluídas em aproximadamente os últimos 60 segundos |
| Latência P95 | Percentil 95 estimado do tempo até o primeiro byte em aproximadamente os últimos 60 segundos |
| Backends | Backends saudáveis / todos os backends, e conexões sendo encaminhadas |

Clique em um site para abrir as [estatísticas do site](/pt-BR/docs/modules/web#site-stats) com as tendências de 24 horas de requisições, erros e latência.

### Tempo até o primeiro byte

A latência é o "tempo até o primeiro byte": da chegada da requisição ao envio dos cabeçalhos da resposta, o que mostra a rapidez do backend ou do próprio site. Não são contados:

- o tempo de transferência do corpo da resposta (downloads grandes não aumentam a latência);
- WebSocket e outras requisições com troca de protocolo;
- requisições interrompidas pelo controle de acesso ou por um administrador.

Cada requisição cai em uma de 50 faixas finas (de 1 ms a 60 s, com cerca de 10% a 25% de diferença entre vizinhas). Cada faixa acumula a quantidade e o tempo total; os percentis usam o tempo médio da faixa em que caem, então o erro nunca passa da largura da faixa, e nada é guardado sobre requisições individuais. O histograma `gately_site_request_duration_seconds` exportado para o Prometheus usa 13 limites comuns dentre eles.

## Regras de alerta

Clique em "Adicionar regra de alerta". Ao criar, um modelo preenche uma regra comum (CPU do host alta, pouco espaço em disco, erros 5xx do site, respostas lentas do site etc.) para você ajustar.

| Campo | Descrição |
| --- | --- |
| Nome | Obrigatório, até 64 caracteres, aparece no título da notificação |
| Métrica | Veja a tabela abaixo |
| Limite | Operador `>`, `>=`, `<` ou `<=` mais um valor; a unidade depende da métrica |
| Duração | De 1 a 1440 minutos. O alerta só é enviado depois que a condição se mantém por esse tempo; uma única queda abaixo do limite reinicia a contagem |
| Sites | Para métricas de sites e de nós de backend; vazio para todos os sites |
| Mínimo de requisições | Para a proporção de 5xx e a latência, padrão 20. Não avalia enquanto os últimos ~60 segundos tiverem menos requisições, para que um ou dois erros ou requisições lentas em um site com pouco tráfego não disparem alerta |
| Nível do alerta | Aviso ou erro; define o nível do evento de notificação |
| Observação, ativar | Opcional |

| Métrica | Unidade | Avaliada por |
| --- | --- | --- |
| Uso de CPU do host | % | Este host |
| Uso de memória do host | % | Este host |
| Uso de disco do host (partição do diretório de configuração) | % | Este host |
| Uso de CPU do processo do Gately | % | Processo do Gately |
| Memória residente do processo do Gately | MB | Processo do Gately |
| Requisições em andamento do site | quantidade | Cada site |
| Requisições por segundo do site | req/s | Cada site |
| Proporção de respostas 5xx do site | % | Cada site |
| Tempo até o primeiro byte P95 do site | ms | Cada site |
| Conexões do nó de backend | quantidade | Cada nó de backend |

As métricas de sites e de nós de backend são avaliadas e alertadas por objeto, então uma única regra "proporção de 5xx > 5%" pode alertar separadamente sobre dois sites. "Requisições por segundo do site < 1" detecta tráfego que parou de repente.

### Avaliação e notificações

1. Todas as regras habilitadas são avaliadas a cada 5 segundos.
2. Quando a condição começa a valer, o objeto fica "Pendente"; depois de se manter pela duração, fica "Disparado" e um evento `perf.alert` é publicado.
3. Quando a condição de um alerta disparado deixa de valer, um evento `perf.recovered` é publicado. A proporção de 5xx e a latência de um site não podem ser avaliadas se os últimos ~60 segundos tiverem poucas requisições; isso também conta como recuperação, e o aviso informa.
4. Quando um site é excluído ou desativado, os alertas dele somem sem aviso de recuperação.

O topo da página lista os objetos pendentes e disparados com o valor atual e a duração.

Alterar o nome, o nível ou a observação de uma regra mantém a contagem em andamento; alterar a métrica, o limite, a duração, os sites ou o mínimo de requisições, ou desativar a regra, limpa as contagens e alertas dela. Um alerta já disparado recebe um aviso de recuperação informando que a regra foi excluída, desativada ou alterada. Contagens e estados de alerta ficam só na memória e recomeçam após uma reinicialização.

Os alertas são enviados pela Central de notificações: adicione uma regra de notificação que assine "Desempenho: alerta disparado" e "Desempenho: alerta recuperado" (o modelo "Alertas de desempenho" faz isso); caso contrário, os alertas só são gravados no log desta página, e a página mostra um aviso. O silêncio da Central de notificações também vale para alertas de desempenho; cada objeto tem uma origem de evento diferente, então eles não se silenciam entre si. A duração já evita alertas intermitentes, por isso o silêncio raramente é necessário.

## Recursos e limites

- O histórico fica só na memória: host e processo têm um ponto por minuto por 24 horas; a latência dos sites guarda P50 e P95 por minuto, cerca de 23 KB por site com tráfego.
- Até 200 regras de alerta e até 200 sites por regra.
- Com o módulo Serviços web ligado, cada requisição custa uma leitura de relógio a mais e algumas atualizações de contadores.

## Métricas de monitoramento

Com Desempenho ligado, `/api/metrics` também exporta as métricas do host e do processo da última amostra e o número de alertas disparados por regra; veja [Monitoramento](/pt-BR/docs/reference/monitoring).

## API

| Endpoint | Descrição |
| --- | --- |
| `GET /api/perf/overview?range=60s\|1h\|24h` | Histórico do host e do processo |
| `GET /api/perf/sites` | Métricas de cada site em aproximadamente os últimos 60 segundos |
| `GET /api/perf/alerts` | Objetos pendentes e disparados |
| `GET /api/perf/metrics` | Métricas disponíveis para alertas |
| `/api/perf/rules` | Criar, ler, alterar e excluir regras de alerta, ativá-las, ordená-las e agrupá-las |

Ler exige a permissão `perf:read` e alterar regras `perf:write`; veja [Tokens de API](/pt-BR/docs/reference/api).
