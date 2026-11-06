# Pacote de diagnóstico

Quando o Gately apresentar problemas, exporte um pacote de diagnóstico em **Configurações → Diagnóstico** e anexe-o à sua [issue no GitHub](https://github.com/wangwanjie/gately/issues/new). Ele ajuda os mantenedores a encontrar a causa sem pedir capturas de tela e logs copiados e colados.

O pacote é gerado na sua própria máquina e nunca é enviado a lugar nenhum. Clique em **Visualizar conteúdo** antes de baixar para ver exatamente o que será exportado. O zip contém apenas texto simples e JSON, então você também pode descompactá-lo e revisar cada arquivo.

## O que é exportado

| Arquivo | Conteúdo |
| --- | --- |
| `README.txt` | Uma descrição bilíngue do pacote |
| `summary.json` | Versão e informações de build do Gately, sistema operacional e arquitetura, modelo e número de núcleos da CPU, totais de memória / disco, tempo de atividade, estatísticas do runtime Go, números das portas da interface de administração, **nomes** das interfaces de rede com as **categorias** de endereço (por exemplo `private: 1, public: 2`) |
| `modules.json` | Lista de módulos e o resumo de status mostrado na página de visão geral |
| `config/*.json` | **Estrutura** da configuração: nomes de campos, números, opções e valores de enumeração como protocolo e modo |
| `logs/*.log` | Logs de execução recentes do sistema e de cada módulo, incluindo os gravados em disco antes de reiniciar (podem ser desativados) |
| `logs/crash.log` | Registro de saídas anormais: mensagem e pilhas das falhas do runtime do Go (panic, fatal error) e o horário aproximado em que o processo foi encerrado à força (por exemplo, pelo OOM killer por falta de memória); presente só depois de uma saída anormal (pode ser desativado) |

## O que nunca é exportado

- Senhas, hashes de senha, tokens de API, segredos TOTP, chaves de sessão, client secrets do OAuth, chaves de provedores de DNS, senhas e chaves privadas SSH, certificados e chaves privadas. `account.json` e `tokens.json` nunca são lidos.
- Nomes de domínio, nomes de host, endereços IP reais e endereços MAC. Eles são substituídos por marcadores como `<domain>`, `<ipv4:private>`, `<ipv4:public>` e `<mac>`; as portas são mantidas.
- O caminho do diretório de configuração e o seu diretório home (substituídos por `<config-dir>` e `<home>`).
- Nomes, observações e qualquer outro texto que você digitou na configuração: toda string que não é um valor de enumeração conhecido vira `<redacted>`.
- Conteúdo de arquivos, contêineres, imagens e variáveis de ambiente do Docker, sessões de terminal e conteúdo de tráfego.

## Como a anonimização funciona

- **Configuração**: os valores são removidos por padrão. Só sobrevivem nomes de campos, números, booleanos, uma lista fixa de campos de enumeração (`protocol`, `mode`, `provider`, …) e timestamps. Campos de endereço mantêm apenas a porta e a categoria do IP.
- **Logs**: endereços IP, domínios, e-mails, URLs, pares no estilo `password=…`, tokens `Bearer`, strings longas parecidas com tokens, endereços MAC e diretórios de usuário são mascarados. Domínios de serviços públicos como `github.com` e `letsencrypt.org` são mantidos porque mostram se uma falha é um problema de rede.
- Um nome que você digitou (por exemplo a observação de uma regra) ainda pode aparecer em uma linha de log se não parecer um IP ou domínio. Dê uma olhada em `logs/*.log` antes de publicar e remova o que você não quiser compartilhar.

## Investigar saídas anormais

Durante a execução, o Gately grava os detalhes das falhas em `crash.log` no diretório de configuração. Depois que o processo termina inesperadamente, o log do sistema na próxima inicialização informa se a última execução travou ou foi encerrada à força:

- **Falha**: `crash.log` traz a causa e as pilhas de todas as goroutines; exporte o pacote de diagnóstico e anexe-o à issue. Em erros fatais como `concurrent map writes`, o Go imprime a linha `fatal error: …` só na saída de erro padrão, que você encontra em `docker logs`.
- **Término inesperado**: o processo não deixou nada, normalmente porque o sistema o encerrou por falta de memória (OOM), ele foi encerrado com `kill -9` / `docker kill` ou houve queda de energia. Rode `dmesg -T | grep -i -E "oom|killed process"` no host para verificar se foi OOM.

## API

`GET /api/system/diagnostics` (somente sessão de login; tokens de API são recusados). `?logs=0` exclui os logs e `?preview=1` retorna os arquivos como JSON.
