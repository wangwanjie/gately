# Backup da configuração e backup agendado

Toda a configuração do Gately fica em arquivos `*.json` no diretório de configuração. O recurso de backup empacota esses arquivos e permite download manual, gravação agendada em vários locais e restauração por sobrescrita a partir de um backup. A interface de administração fica no grupo "Sistema" da barra lateral → "Configurações" → aba "Backup e migração".

## Conteúdo do backup

Um backup é um arquivo zip com todos os `*.json` da raiz do diretório de configuração (conta, regras dos módulos, certificados, hashes de tokens e assim por diante). Ele inclui hashes de senha, segredos, hashes de tokens de API e chaves privadas de certificados, então guarde-o com cuidado. Os logs de execução (`logs/`) e o log de auditoria (`audit.log`) não são incluídos.

O formato do nome do arquivo é `gately-backup-YYYYMMDD-HHMMSS.zip`, e os backups criptografados usam `gately-backup-YYYYMMDD-HHMMSS.zip.enc`.

## Backup e restauração manuais

### Baixar um backup

O botão "Baixar backup" no cartão "Fazer backup da configuração do Gately" baixa um zip da configuração atual. Os backups baixados manualmente não são criptografados.

### Restaurar

No cartão "Restaurar configuração", escolha um arquivo de backup, clique em "Restaurar" e digite a palavra "restaurar" na janela de confirmação.

- A restauração sobrescreve toda a configuração atual. O limite de tamanho do envio é 64 MB.
- Antes de sobrescrever, a configuração existente é copiada para uma pasta `backup-<hora>` no diretório de configuração (formato de hora `YYYYMMDD-HHMMSS`), e a interface mostra esse caminho após uma restauração bem-sucedida.
- O Gately reinicia automaticamente após a restauração.
- O backup precisa conter a configuração da conta; um backup sem ela é rejeitado.
- Do backup, só são aceitos arquivos `*.json` localizados na raiz do zip, que não sejam ocultos e que contenham JSON válido. Todas as demais entradas são ignoradas.
- Ao escolher um arquivo `.enc`, aparece o campo "Senha do backup". Informe a senha definida quando o backup foi criado. Uma senha errada ou um arquivo danificado faz a restauração falhar.

As APIs de download e restauração de backup aceitam apenas sessões com login, e os [tokens de API](/pt-BR/docs/reference/api) não podem chamá-las.

## Backup agendado

No cartão "Backup agendado", abaixo da aba "Backup e migração", a chave no canto superior direito ativa ou desativa o recurso. O ID do módulo é `backup`, e os escopos de token são `backup:read` e `backup:write`.

| Configuração | Descrição |
| --- | --- |
| Agendamento | Uma expressão cron de 5 campos, padrão `0 3 * * *` (todo dia às 03:00). Descritores como `@daily` e `@weekly` também funcionam |
| Retenção | Mantém os N backups mais recentes de cada destino, de 1 a 365, padrão 7 |
| Senha de criptografia | Deixe em branco para não criptografar; quando definida, são gerados backups `.zip.enc`. Se já houver uma senha definida, deixar o campo em branco a mantém inalterada |
| Diretório local | Chave, ativada por padrão. Grava na pasta `backups` do diretório de configuração |

Clique em "Salvar configurações" para aplicar o agendamento e em "Fazer backup agora" para executar um imediatamente. O cartão mostra a hora da última execução, como ela foi disparada (manual ou agendada), a hora da próxima execução, o resultado de sucesso ou falha em cada destino e o número de backups antigos removidos. Os arquivos de backup locais são listados no cartão e podem ser baixados.

### Destinos de backup

Além do diretório local, você pode adicionar vários destinos remotos. Cada um grava em uma montagem de "Montagens de armazenamento" (um diretório local, um WebDAV remoto ou um compartilhamento SMB):

| Campo | Descrição |
| --- | --- |
| Nome | Obrigatório |
| Montagem de armazenamento | Escolha uma montagem existente em "Montagens de armazenamento". Montagens somente leitura ou desativadas não podem ser selecionadas |
| Diretório | Subdiretório dentro da montagem; deixe em branco para a raiz |
| Ativado | Chave |

Se você ainda não tem montagens, adicione uma primeiro em "Montagens de armazenamento". Excluir um destino não exclui os backups já gravados.

### Fluxo de execução

1. Empacota o diretório de configuração e o criptografa se houver uma senha definida.
2. Grava no diretório local (se ativado) e em cada destino ativado, com o mesmo nome de arquivo em todos.
3. Após uma gravação bem-sucedida, remove os backups mais antigos naquele local de acordo com a quantidade de retenção. A limpeza só afeta arquivos cujos nomes começam com `gately-backup-` e terminam com `.zip` ou `.zip.enc`; os demais arquivos ficam como estão.
4. Resume os resultados, grava no log do módulo e publica um evento de notificação.

Se nem o diretório local nem nenhum destino estiver ativado, o backup é registrado como falha. Apenas um backup roda por vez.

### Eventos de notificação

| Evento | Nível | Gatilho |
| --- | --- | --- |
| `backup.done` | Info | Todos os locais foram gravados com sucesso; a mensagem lista o nome do arquivo e o resultado de cada local |
| `backup.failed` | Erro | O empacotamento falhou ou a gravação em algum local falhou; a mensagem lista o resultado de cada local |

Crie na [Central de notificações](/pt-BR/docs/modules/notify) uma regra que assine `backup.*` para receber os resultados dos backups por Telegram, e-mail e outros canais.

## Criptografia

Quando uma senha é definida, os backups são criptografados da seguinte forma:

- Derivação de chave: scrypt (N=32768, r=8, p=1), gerando uma chave de 32 bytes, com um salt aleatório de 16 bytes.
- Criptografia: AES-256-GCM, com um nonce aleatório de 12 bytes.
- Formato do arquivo: o magic `GATELYBK1`, o salt, o nonce e o texto cifrado concatenados nessa ordem. O magic também é usado como dado autenticado adicional do GCM.

A senha é armazenada somente no `backup.json` do diretório de configuração. Se a senha for perdida, o backup não pode ser descriptografado. A frase secreta só pode ser vista e alterada depois de entrar na interface de administração; tokens de API não podem lê-la, alterá-la nem apagá-la, mas as demais configurações de backup continuam podendo ser alteradas com um token.

::: warning Atenção
Quando o `backup.json` e os arquivos de backup ficam na mesma máquina, a proteção por senha agrega pouco valor. Grave os backups criptografados em uma montagem de outro dispositivo e registre a senha separadamente.
:::
