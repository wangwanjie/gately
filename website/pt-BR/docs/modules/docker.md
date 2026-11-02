# Docker

Gerencie contêineres, projetos Compose, imagens, redes e volumes do host por meio do socket do Docker. O ponto de entrada é "Docker" na barra lateral, e a página se divide em seis abas: "Contêineres", "Compose", "Imagens", "Redes", "Volumes" e "Sistema". Quando o Docker não pode ser acessado, o topo da página mostra os detalhes do erro e um botão de atalho para abrir as configurações.

## Ativação

Quando o Gately roda em um contêiner, monte o socket do Docker do host:

```bash
-v /var/run/docker.sock:/var/run/docker.sock
```

O endereço do Docker é `unix:///var/run/docker.sock` por padrão e pode ser alterado nas configurações da aba "Sistema".

::: warning Aviso de segurança
Montar o `docker.sock` equivale a conceder acesso de nível root ao host. Se você não precisa do gerenciamento do Docker, remova a montagem e proteja a interface de administração (autenticação em dois fatores, entrada segura, acesso apenas pela rede local).
:::

## Contêineres

- A lista pode ser filtrada por "Todos", "Em execução" e "Parados". Os cartões mostram imagem, rede, portas, montagens e status de atualização;
- Ações: iniciar, parar, reiniciar, pausar, retomar, forçar encerramento, renomear, excluir (opcionalmente com volumes anônimos), fazer commit como imagem;
- **Autoproteção**: o Gately reconhece o próprio contêiner e o projeto Compose, e rejeita operações que cortariam o acesso a ele: parar, forçar a parada, pausar ou excluir seu contêiner, e executar `down` no seu projeto. Editar e recriar, executar `up` no seu projeto ou excluí-lo exigem parar antes o contêiner antigo; o processo do Gately terminaria junto e não conseguiria concluir a recriação, então essas operações também são rejeitadas e os botões correspondentes ficam desativados na interface. A proteção também vale quando a API é chamada com o nome do contêiner ou um prefixo de ID.
- **Atualizar o próprio Gately**: ao atualizar o próprio contêiner, o Gately baixa primeiro a nova imagem e, só se houver uma nova versão, cria a partir da imagem atual um contêiner auxiliar temporário (`<nome do contêiner>-upgrade-<hora>`). O auxiliar para o contêiner antigo, cria e inicia o novo com a mesma configuração e remove o antigo; se o novo não iniciar, o antigo é restaurado. Enquanto isso a interface mostra uma tela de espera e recarrega quando o Gately volta; o novo Gately grava a saída do auxiliar no log do Docker e o remove. Atualizações em lote tratam o Gately por último e não o atualizam se outro contêiner falhou. O auxiliar acessa o socket do Docker pela mesma montagem do Gately (por exemplo `/var/run/docker.sock:/var/run/docker.sock`); sem essa montagem a atualização é rejeitada. Não recrie o Gately com `docker compose up -d`, `docker restart` ou comandos parecidos no shell local do Terminal web: numa implantação com Docker esse shell roda dentro do contêiner do Gately, então o comando termina junto com o Gately e deixa o contêiner antigo parado ao lado de um novo com nome temporário (`<prefixo do ID>_<nome do contêiner>`). Se isso acontecer, execute `docker compose up -d` mais uma vez no diretório do Compose no host para recuperar. Atualizar no host com `docker compose pull && docker compose up -d` continua funcionando como antes.
- **Informações sensíveis**: quando uma conta somente leitura ou um token de API com apenas `docker:read` visualiza um contêiner, as variáveis de ambiente mostram só o nome, e os valores, Command, Entrypoint, argumentos de inicialização e os argumentos dos comandos na lista de processos aparecem como `********`. Os detalhes da imagem também ocultam as variáveis de ambiente, Cmd, Entrypoint e os comandos do histórico de build (ao fazer commit de um contêiner como imagem, essas configurações são gravadas na imagem como estão). Administradores com permissão de escrita recebem o texto em claro automaticamente ao abrir o formulário de edição.
- **Criar e editar**: o formulário cobre nome, imagem (pode ser baixada antes da criação), iniciar após a criação, comando e Entrypoint, variáveis de ambiente, mapeamentos de portas (TCP/UDP, com IP do host opcional), montagens (bind de diretório e volume, opcionalmente somente leitura), rede principal e redes extras, IP fixo, aliases, política de reinício (`no`, `always`, `unless-stopped`, `on-failure` com número máximo de tentativas), modo privilegiado, adicionar ou remover capabilities, mapeamento de dispositivos, usuário, diretório de trabalho, hostname, DNS, labels, limite de memória (MB), número de CPUs, TTY e manter stdin aberto. A edição recria o contêiner com base na configuração original; ajustes que o formulário não cobre mantêm os valores originais, e uma recriação com falha sofre rollback automático;
- Abas da página de detalhes:
  - Visão geral: status, código de saída, número de reinícios, estado de saúde, detalhes de rede, montagens e o projeto e serviço Compose a que pertence;
  - Logs: veja os logs recentes, com timestamps opcionais;
  - Terminal: abra um terminal interativo dentro de um contêiner em execução (`/bin/sh` por padrão; comando e usuário podem ser informados; as cores seguem a preferência do terminal web);
  - Arquivos: navegue pelos arquivos do contêiner, edite texto online (até 1 MiB), envie, baixe (diretórios são empacotados em tar.gz), renomeie, crie diretórios e exclua. A exclusão executa `rm -rf` dentro do contêiner, então ele precisa estar em execução;
  - Monitoramento: amostras de CPU, memória, rede, E/S de disco e número de processos, além da lista de processos dentro do contêiner.

## Compose

Os projetos Compose são agregados a partir de labels dos contêineres, como `com.docker.compose.project`. Eles mostram um status (em execução, parcialmente em execução, parado) e seus serviços.

- Iniciar, parar, reiniciar, pausar, retomar, `down` (opcionalmente com volumes nomeados) e ver os logs combinados (cada linha com o nome do serviço como prefixo);
- Criar um projeto: cole o conteúdo do compose, e o Gately o grava em `compose/<name>/compose.yaml` no diretório de configuração e executa o up. Esses projetos são marcados como "managed"; você pode editar o arquivo Compose, salvar e aplicar, ou excluir o projeto inteiro;
- Em projetos existentes, o primeiro arquivo compose pode ser editado online (o arquivo precisa ser gravável), com a opção de executar o up logo depois de gravá-lo;
- `up`, download de imagens, builds e leitura e escrita de arquivos dependem da linha de comando `docker compose` e do acesso ao caminho do arquivo compose. Quando indisponíveis, a interface mostra "comando docker compose não encontrado".

## Imagens

- A lista mostra tags, tamanho, data de criação, número de contêineres que referenciam cada imagem e se ela é dangling. Os detalhes incluem histórico de camadas, variáveis de ambiente, portas expostas e mais;
- **Pull**: autentica com as contas de registry das configurações em "Sistema". Se o pull direto falhar e houver endereços de mirror configurados, eles são tentados em ordem e a imagem recebe de volta a tag com o nome original. A busca no Docker Hub é compatível;
- Push, adicionar tag, excluir (opcionalmente forçado), build (Dockerfile inline ou URL Git, com argumentos de build) e importar e exportar tar (formato `docker save`);
- **Verificação de atualizações**: verifique um único contêiner ou use "Verificar todas as atualizações"; os resultados são marcados em cada contêiner;
- **Upgrade em um clique**: baixa a nova imagem e recria o contêiner com a configuração existente; um upgrade com falha sofre rollback automático para o contêiner original. A seleção múltipla para upgrades em lote é compatível;
- "Limpar dangling" remove com um clique todas as imagens sem tag e não utilizadas.

## Redes e volumes

- Redes: a lista mostra driver, escopo, sub-rede, gateway e contêineres conectados. Você pode criar (driver, sub-rede, gateway, rede interna, IPv6) e excluir redes, além de conectar ou desconectar contêineres. As redes embutidas são protegidas;
- Volumes: mostra driver, ponto de montagem, tamanho, número de referências e os contêineres que usam cada volume. Você pode criar, excluir (opcionalmente forçado), exportar como tar.gz e importar de tar.gz. A exportação e a importação usam um contêiner auxiliar alpine temporário, e a imagem é baixada automaticamente se estiver ausente.

## Sistema

A aba "Sistema" contém:

- Informações do host e do Docker: versão, versão da API, SO e arquitetura, kernel, CPU e memória, driver de armazenamento, diretório de dados, quantidade de contêineres e imagens, versão do Compose CLI;
- Uso de disco: quantidade, tamanho e espaço recuperável de imagens, contêineres, volumes e cache de build;
- Limpeza: contêineres parados, imagens dangling, todas as imagens não utilizadas, redes não utilizadas, volumes não utilizados (os dados são perdidos de forma permanente), cache de build;
- Configurações:

| Campo | Descrição |
| --- | --- |
| `host` | Endereço do Docker, padrão `unix:///var/run/docker.sock` |
| `mirrors` | Lista de endereços de mirror, tentados em ordem quando o pull direto falha. Independentes de `registry-mirrors` em `/etc/docker/daemon.json`, que o Docker usa automaticamente e a página de configurações mostra somente leitura como "Espelhos do daemon" |
| `registries` | Credenciais de registries privados (`server`, `username`, `password`); a senha é mascarada na interface |
| `autoCheckHours` | Quando maior que 0, as atualizações de imagens dos contêineres são verificadas automaticamente nesse intervalo em horas; 0 desativa; imagens mais recentes publicam um evento `docker.update`, uma vez por imagem nova |

## Tarefas em segundo plano

Operações demoradas, como pull, push, build, verificação em lote, upgrade em lote e Compose up, rodam como tarefas em segundo plano. O painel de tarefas, na parte inferior da página, mostra a saída em tempo real e o status (em andamento, concluída, com erro), permite ver o histórico e limpar as tarefas concluídas. As tarefas ainda em execução são restauradas após atualizar a página.

## Notificações de eventos

O Gately assina o fluxo de eventos do Docker e publica um evento `docker.exited` quando um contêiner termina de forma anormal (código de saída diferente de zero) ou é encerrado pelo OOM killer. Você pode configurar os canais de recebimento na [Central de notificações](/pt-BR/docs/modules/notify). Uma parada normal com código de saída 0 não dispara notificação. Quando o Docker fica temporariamente indisponível, a assinatura é refeita a cada 30 segundos.

## API

Veja a referência completa da API na [API do Docker](/pt-BR/docs/reference/docker-api).
