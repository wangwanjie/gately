# Terminal Web

Conecte-se a servidores SSH e Telnet ou ao shell local pelo navegador. As sessões são mantidas no servidor e podem ser reconectadas depois de uma desconexão. O ponto de entrada é "Terminal Web" na barra lateral.

## Tipos de conexão

| Tipo | Descrição |
| --- | --- |
| SSH | Compatível com autenticação por senha e por chave privada (cole uma chave privada ou informe o caminho `keyFile` no servidor; senhas de chave são compatíveis). Você pode definir o `workDir` a ser aberto após o login, o `keepAlive`, o timeout de conexão (1 a 120 segundos, padrão 15) e um proxy |
| Telnet | Porta padrão 23. Quando prompts como `login:`, `username:` e `password:` são detectados, o campo correspondente é preenchido automaticamente uma vez cada |
| Shell local | Abre um shell diretamente no dispositivo em que o Gately roda. Você pode informar o caminho do shell, o diretório de trabalho e variáveis de ambiente extras |

Cada conexão pode ter uma observação. A lista de conexões permite ordenar por arrastar e soltar e "Testar conexão": no SSH, retorna a versão do servidor e a latência; no Telnet, apenas verifica a acessibilidade TCP (inclusive pelo proxy); conexões de shell local não podem ser testadas. Ao editar uma conexão, a senha, a chave privada, a senha da chave e a senha do proxy aparecem mascaradas, e os valores originais são mantidos se você não os alterar.

Em «Configurações» você pode gerenciar «predefinições de credenciais SSH» (senha ou chave privada, com nome de usuário padrão opcional). Ao adicionar ou editar uma conexão SSH, escolha uma predefinição para reutilizá-la: várias conexões podem compartilhar a mesma credencial e as alterações na predefinição valem na próxima conexão. Uma predefinição ainda referenciada por uma conexão não pode ser excluída.

### Proxy

Conexões SSH e Telnet podem usar, cada uma, o seu próprio proxy de saída do tipo `socks5` ou `http`, com usuário e senha de proxy opcionais.

### Trust on first use para chaves de host

Na primeira conexão SSH, a impressão digital do host do servidor é registrada e passa a ser confiável, sendo verificada em todas as conexões seguintes. Depois que um servidor é reinstalado ou suas chaves mudam, execute "Redefinir impressão digital do host" na conexão para confiar nele novamente.

## Sessões

- As sessões são mantidas no servidor. Após atualizar o navegador ou se desconectar, você pode reconectar pela lista de sessões; ao reconectar, o buffer de saída é reproduzido primeiro e depois a saída é transmitida ao vivo;
- Vários clientes podem se conectar à mesma sessão ao mesmo tempo, com entrada compartilhada e saída transmitida a todos;
- As sessões podem ter uma observação. A lista de sessões mostra a data de criação, a hora da última atividade, o tamanho da janela e os bytes enviados e recebidos;
- Após atualizar a página, as sessões ainda ativas são restauradas como abas, junto com o layout dividido;
- O terminal tem pesquisa embutida (digite uma palavra-chave, pressione Enter para a próxima ocorrência e Shift+Enter para a anterior), copiar, colar, limpar a tela, ajuste do tamanho da fonte, tela cheia e vários esquemas de cores (por padrão seguem o tema da interface, ou um esquema fixo).

### Configurações

Ajuste em "Configurações", no canto superior direito da página:

| Campo | Padrão | Descrição |
| --- | --- | --- |
| `allowLocalShell` | Desativado | Define se é permitido abrir o shell local |
| `maxSessions` | 20 | Máximo de sessões simultâneas, até 1000. Criar uma sessão além do limite retorna HTTP 429 |
| `idleTimeout` | 0 | A sessão termina automaticamente após esta quantidade de minutos sem entrada nem saída; 0 significa sem limite |
| `bufferSize` | 256 | Buffer de reprodução mantido por sessão (KB), até 16384 |
| `heartbeatInterval` | 30 | Intervalo de heartbeat do WebSocket (segundos), até 600 |
| `sessionKeepAlive` | 0 | Minutos em que uma sessão é mantida depois que o navegador se desconecta; 0 encerra imediatamente na desconexão, até 10080 |

::: tip Dica
Para poder reconectar depois de uma desconexão, defina `sessionKeepAlive` com um valor maior que 0. Com 0, a sessão termina assim que o navegador se desconecta.
:::

## Gerenciador de arquivos SFTP

O ícone de pasta na barra de ferramentas do terminal abre o painel do gerenciador de arquivos. Conexões SSH operam em arquivos remotos por SFTP, e conexões de shell local operam diretamente no sistema de arquivos do dispositivo em que o Gately roda. Conexões Telnet não têm gerenciador de arquivos. O gerenciador de arquivos local exige que "Permitir shell local" esteja ativado.

| Recurso | Descrição |
| --- | --- |
| Navegar | Breadcrumb e navegação para o diretório pai, diretórios primeiro e ordenados por nome, com ordem alternável; mostra permissões, tamanho, data de modificação e proprietário |
| Criar | Nova pasta, novo arquivo vazio |
| Enviar | Envio de vários arquivos; no desktop, pastas inteiras podem ser enviadas (a estrutura de subdiretórios é mantida). Arquivos com o mesmo nome são sobrescritos, e o painel mostra o progresso do envio |
| Baixar | Download em streaming com suporte a retomada (`Range`) |
| Edição online | Visualize e salve arquivos de texto; arquivos maiores que 1 MB ou binários não podem ser visualizados |
| Renomear, copiar, colar | Ao copiar para o mesmo diretório, o arquivo é renomeado automaticamente para `nome copy.ext`, `nome copy 2.ext` |
| Excluir | Exclusão em lote com seleção múltipla; diretórios são excluídos recursivamente; o diretório raiz é protegido |
| Compactar e extrair | Compacte em `zip` ou `tar.gz`; extraia `zip`, `tar`, `tar.gz` e `tgz`; o conteúdo do arquivo (até 10000 entradas) pode ser visualizado antes de extrair. A compactação e a extração são feitas pelo próprio Gately, então nenhum comando precisa estar instalado no host remoto |
| Alterar permissões | Permissões em octal (0 a 7777), com opção recursiva para diretórios |
| Acesso rápido | Salve diretórios usados com frequência para cada conexão e vá até eles com um clique |

Ao extrair, um arquivo que contenha qualquer entrada com `..` é rejeitado por inteiro para evitar directory traversal, entradas de links simbólicos e hard links são ignoradas, e apenas os bits rwx das permissões dos arquivos são mantidos. No celular, o painel oferece um modo de seleção múltipla e uma folha de ações.

## Atalhos

O menu "Atalhos" lista os atalhos globais, e clicar em um deles o envia ao terminal atual. Cada atalho tem um `name`, um `command` e `sendEnter` (se deve pressionar Enter automaticamente após o envio). Adicione, edite e exclua atalhos em "Gerenciar atalhos", na parte inferior do menu.

## Visualização dividida

O menu "Dividir" na barra de ferramentas oferece layouts lado a lado, empilhado e em grade, com até 4 painéis por vez. Com a divisão ativada, qualquer conexão que você abrir entra automaticamente na divisão, e você também pode clicar em outra aba para adicioná-la. A barra de título de um painel permite remover uma sessão da divisão. O layout dividido é armazenado no servidor e restaurado depois de atualizar a página. Em telas estreitas, a divisão é desativada automaticamente, e o layout salvo não é apagado.

## Suporte a dispositivos móveis

- Em telas estreitas, a lista de conexões e o terminal são visualizações separadas, e o botão voltar no canto superior esquerdo da visualização do terminal retorna à lista;
- A área do terminal acompanha o viewport visual e diminui quando o teclado virtual aparece;
- A barra de teclas virtuais é uma única linha que pode ser deslizada para os lados. Ela oferece `Esc`, `Tab`, `Ctrl`, `Alt` (fixas), teclas de seta (segure para repetir), símbolos comuns e um acesso aos atalhos, com um botão de teclado fixo à direita que mostra ou oculta o teclado. Por padrão, a barra aparece e some junto com o teclado virtual; com o teclado fechado, um botão flutuante no canto inferior direito o abre de novo. Para usar setas, paginação e outras teclas sem o teclado, ative **Sempre mostrar a barra de teclas** no menu **Mais**.

## Segurança

Um terminal equivale a executar comandos arbitrários no host de destino, por isso o Gately aplica estas restrições por padrão:

- **O shell local fica desativado por padrão** e precisa ser ativado explicitamente nas configurações;
- **Confirmação do termo de uso**: na primeira vez que você ativa o shell local, cria uma sessão, se conecta a uma sessão ou usa o gerenciador de arquivos, um termo de uso aparece e você precisa marcá-lo para continuar;
- **Autenticação em dois fatores (2FA)**: depois de vincular um autenticador TOTP escaneando o QR code nas configurações de segurança, as ações acima exigem um código de 6 dígitos. Após a verificação, o token de login atual não precisa verificar de novo por 30 minutos; após 5 códigos incorretos consecutivos, esse token de login fica bloqueado por 1 minuto;
- Senhas de conexão, chaves privadas e senhas de proxy são mascaradas na saída da API.

::: warning Atenção
Antes de ativar o shell local, ative a autenticação em dois fatores na interface de administração e use-o junto com o "Acesso apenas pela rede local" ou uma VPN.
:::

## API

Veja a referência completa da API na [API do Terminal Web](/pt-BR/docs/reference/webterminal-api).
