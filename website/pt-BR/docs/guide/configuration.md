# Configuração e opções de execução

## Parâmetros de linha de comando

| Parâmetro | Descrição |
| --- | --- |
| `-c` | Diretório de configuração; também pode ser definido pela variável de ambiente `GATELY_CONFIG_DIR` |
| `-listen` | Substitui o endereço de escuta da interface de administração, por exemplo `:16680` |
| `-apply` | Diretório de configuração declarativa; também pode ser definido pela variável de ambiente `GATELY_APPLY_DIR`, veja abaixo |
| `-version` | Exibe a versão |

| Variável de ambiente | Descrição |
| --- | --- |
| `GATELY_INITIAL_PASSWORD` | Senha inicial do administrador na primeira inicialização (no mínimo 8 caracteres); se não for definida, vale a senha padrão `gately.cn`; sem efeito se a conta já existir |

## Diretório de configuração

Toda a configuração fica no diretório de configuração, com um arquivo de texto simples `<name>.json` por módulo, gravado de forma atômica com permissão 0600. Isso permite:

- Inspecionar tudo com qualquer editor de texto;
- Manter o diretório no git, auditar cada alteração com diffs e fazer rollback a qualquer momento;
- Fazer backup simplesmente copiando o diretório.

::: warning Atenção
O `ssl.json` contém chaves privadas de certificados. Antes de versioná-lo, confirme que o repositório é privado ou adicione o arquivo à lista de ignorados.
:::

## Configuração declarativa

Com `-apply <diretório>`, a cada inicialização os arquivos `<nome>.json` desse diretório são copiados por cima dos do diretório de configuração; arquivos com conteúdo idêntico não são reescritos. Isso é útil para distribuir a configuração de forma centralizada a partir de um repositório git, de um ConfigMap do Kubernetes ou de scripts de implantação.

- Antes de gravar, verifica-se se cada arquivo é JSON válido; se algum for inválido, a inicialização é recusada;
- `account.json` e `tokens.json` não podem ser sobrescritos: contas e tokens só são gerenciados na interface de administração;
- O conteúdo antigo sobrescrito fica salvo no [histórico de configuração](/pt-BR/docs/guide/audit) e pode ser revertido a qualquer momento.

## Diretório de programas externos

Tailscale, frp, cloudflared e File Browser gerenciam os programas externos oficiais. O Gately procura cada executável nesta ordem:

1. O caminho definido nas configurações do módulo;
2. O subdiretório `bin/` do diretório de configuração;
3. O `PATH`.

A página de cada módulo permite instalar, atualizar e reverter online; o programa fica em `bin/` e as versões anteriores em `bin/.versions/`. Veja os detalhes em [Instalação, atualização e reversão online](/pt-BR/docs/modules/tunnels#install). Você também pode colocar os binários no diretório de configuração por conta própria, em implantações com Docker por exemplo `./gately-data/bin/frpc`; o Tailscale precisa de `tailscale` e `tailscaled`.

## Logs de execução

Os logs de execução do sistema e de cada módulo são gravados por padrão em `logs/` no diretório de configuração, então continuam disponíveis na página de logs do sistema e na de cada módulo depois de reiniciar ou atualizar:

- Um arquivo JSON Lines por origem: `logs/<ID do módulo>.log`, e os do sistema em `logs/gately.log`. Na inicialização, as entradas mais recentes, até "Linhas de log retidas", são restauradas do arquivo;
- Novas entradas são gravadas em lote com no máximo 1 segundo de atraso. Quando um arquivo passa do limite (2 MB por padrão), ele é rotacionado para `.1` e só esse arquivo anterior é mantido, então cada origem ocupa no máximo o dobro do limite;
- Os logs de acesso do Serviço Web são volumosos e costumam ser revistos depois, por isso são arquivados por dia: quando o arquivo atual vira o dia (ou passa de um quarto do tamanho total, no máximo 16 MB), ele é renomeado para `logs/webservice.<hora de início>.log` e compactado em segundo plano para `.log.gz`; os arquivos mais antigos são apagados conforme "Retenção dos logs de acesso (dias)" (7 por padrão) e "Tamanho total dos logs de acesso (MB)" (arquivo atual mais todos os arquivos compactados, 32 MB por padrão e no máximo 5% da partição onde ficam os logs);
- Cada módulo registra as origens de log que já apareceram em `logs/<ID do módulo>.sources`, mantido após reiniciar;
- Em Configurações → Rede e execução → Parâmetros de execução você pode desativar "Salvar logs em disco", alterar "Limite do arquivo de log", "Retenção dos logs de acesso (dias)" e "Tamanho total dos logs de acesso (MB)", ou mudar o "Diretório de logs" para outro caminho absoluto (por exemplo, um diretório num disco externo; se não existir, é criado com acesso só para o usuário com que o Gately roda), com efeito imediato. Depois de mudar o diretório, os logs novos vão para o novo diretório e os arquivos do antigo ficam onde estão. Com a gravação em disco desativada, os logs ficam apenas na memória e são apagados ao reiniciar;
- Limpar os logs de um módulo na interface também apaga os arquivos dele, o arquivo rotacionado, os arquivos compactados e a lista de origens;
- Os backups de configuração não incluem `logs/` nem o log de auditoria `audit.log`.

### Visualizar e filtrar {#log-filter}

A página Logs do sistema e o cartão de logs no fim de cada página de módulo usam o mesmo visualizador. As condições abaixo podem ser combinadas; as mudanças valem na hora e só entram as novas entradas que correspondem:

- Nível e origem (a lista mostra todas as origens da configuração do módulo e as que já apareceram nos logs, por exemplo todos os listeners do Serviço Web, inclusive os que ainda não tiveram acesso ou cujos logs estão só em disco; também dá para digitar);
- Palavras-chave: termos separados por espaço devem corresponder todos, aspas duplas agrupam uma frase e um `-` inicial exclui um termo, por exemplo `GET "timeout" -debug`; as correspondências ficam destacadas. Com a interface em outro idioma, pesquise pelo texto traduzido que aparece na tela;
- Excluir: entradas que contêm qualquer um dos termos ficam ocultas, junto com as palavras-chave; a escrita é a mesma (separado por espaço, aspas duplas para frases), por exemplo `debug "health check"`. Para excluir um termo que começa com `-`, coloque-o entre aspas duplas, como `"-v"`;
- IP em Mais filtros: endereço completo, faixa (como `10.0.0.0/8`) ou parte do endereço, separados por vírgula; quando a entrada não tem um campo de IP próprio, os endereços são reconhecidos no texto, então também funciona com falhas de login e logs da proteção contra intrusão; o prefixo `!` ou `-` exclui, por exemplo `!1.2.3.4, !10.0.0.0/8` oculta essas origens, e pode ser combinado com itens incluídos (`10.0.0.0/8, !10.0.0.5`);
- Intervalo de tempo em Mais filtros: dos últimos 15 minutos aos últimos 7 dias, ou início e fim personalizados;
- A barra de ferramentas do Serviço Web também tem duas chaves de exibição que afetam só o que é mostrado (os logs continuam salvos e a escolha fica gravada no navegador): Ocultar origens banidas oculta as requisições rejeitadas porque a origem está banida (atrás de uma CDN, os banimentos do kernel não valem para as conexões da CDN, então visitantes banidos são rejeitados na camada web e deixam registros 403 / 444), e também todas as requisições de origens que continuam banidas, incluindo as que causaram o banimento (voltam a aparecer quando o banimento é removido ou expira); Ocultar requisições do painel oculta as requisições com uma sessão do painel de administração desta instância, como a consulta periódica da página de logs ao abrir o painel por um site do Serviço Web, e vem ativada por padrão. Esses registros trazem a etiqueta "Bloqueado por=Banido" ou "Usuário do painel=…", e os parâmetros da API são `hideBanned=1` e `hideAdmin=1` (só contam sessões de administrador, não contas somente leitura; requisições bloqueadas nunca recebem a etiqueta de usuário do painel);
- Os logs do Serviço Web também podem ser filtrados por nome do site ou domínio e por código de status (`404`, `4xx`, `500-599`, separados por vírgula); os do Firewall WAF, por domínio, e as detecções registram o domínio e o ID da regra.

Clique numa origem do log, ou numa etiqueta como "Site=…" ou "Domínio=…" no fim da linha, para filtrar por esse valor; nos logs do Serviço web, clicar em "Site=…" compara o nome do site por completo, a barra de ferramentas mostra uma etiqueta removível "Site: …" (parâmetro de API `site`) e sites de nome parecido não entram no resultado. A API também aceita o parâmetro `listener` (chave do listener, correspondência exata); quando dois listeners têm o mesmo nome, os nomes de origem no log recebem o endereço de escuta, por exemplo `HTTPS (0.0.0.0:443)` e `HTTPS ([::]:443)`, e nomes únicos continuam iguais. Quando as entradas em memória não bastam, Carregar logs anteriores continua pelo histórico guardado nos arquivos de log (inclusive os arquivos compactados do Serviço Web; ao filtrar por período, os arquivos fora do período são pulados); com a gravação de logs em disco ativada, a filtragem não fica limitada ao número de linhas mantidas. `GET /api/logs` e `GET /api/<módulo>/logs` aceitam os mesmos parâmetros de filtro; veja a [referência da API](/pt-BR/docs/reference/api-reference).

### Exportar {#log-export}

"Exportar" na barra de ferramentas de logs exporta todas as entradas que atendem aos filtros atuais, inclusive o histórico guardado nos arquivos de log e nos arquivos compactados, da mais antiga para a mais recente, e o navegador baixa o arquivo diretamente:

- CSV (planilha): UTF-8 com BOM, abre direto no Excel; as colunas são hora, nível, origem, mensagem e campos como IP, domínio, site e código de status; células que começam com `=`, `+`, `-` ou `@` recebem um apóstrofo na frente, para que a planilha nunca execute como fórmula caminhos enviados por visitantes;
- JSON Lines: uma entrada por linha, com os mesmos campos da API de logs;
- As mensagens são exportadas no idioma da interface; contas somente leitura e tokens de API só com permissão de leitura também podem exportar, e senhas e tokens em endereços ficam ocultos como na visualização;
- A API é `GET /api/logs/export` e `GET /api/<módulo>/logs/export`, com `format` igual a `csv` ou `jsonl` e os demais parâmetros de filtro iguais aos da visualização de logs; a API da lista de origens é `GET /api/logs/sources` e `GET /api/<módulo>/logs/sources`.

## Backup e restauração

Em "Configurações" você encontra o backup e a restauração da configuração, que também é o ponto de entrada para importar configurações do Lucky.
