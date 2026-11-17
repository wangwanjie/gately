# Auditoria e histórico de configuração

O Gately guarda chaves de DNS e chaves privadas de certificados, então deve ser possível rastrear quem fez o quê e quando, como a configuração mudou, e desfazer essas mudanças.

## Log de auditoria

Na aba "Log de auditoria" da página "Sistema → Logs", na barra lateral, as operações abaixo aparecem em ordem cronológica inversa, com filtro por categoria e busca por caminho, usuário ou IP:

| Categoria | O que é registrado |
| --- | --- |
| Login | Logins bem-sucedidos e com falha por senha, passkey e login de terceiros, além de logouts |
| Alteração | Todas as operações de escrita feitas pela interface de administração e por tokens de API (criar, alterar, excluir, iniciar/parar, reverter etc.) |
| Acesso sensível | Download de backups da configuração, pacotes de diagnóstico e chaves privadas de certificados, abertura do Terminal Web ou do terminal de contêiner etc. |

Cada registro contém horário, autor (nome de usuário ou `api-token:<nome do token>`), IP de origem, método, caminho e código de status. **O corpo das requisições e quaisquer segredos nunca são registrados.**

- O log é gravado em `audit.log` (JSON Lines) no diretório de configuração; ao passar de 5 MiB o arquivo é rotacionado, mantendo 3 arquivos anteriores;
- Quando a interface de administração fica atrás de um proxy reverso neste mesmo host, a origem real é registrada e o endereço do proxy intermediário é anotado à parte; o funcionamento está descrito em [Modelo de segurança](/pt-BR/docs/guide/security);
- Alterações e acessos sensíveis são emitidos como eventos `audit.change` e `audit.access`, que podem ser enviados pela [Central de notificações](/pt-BR/docs/modules/notify) com os modelos "Alterações de configuração" e "Segurança e logins";
- Contas somente leitura e tokens de API não podem ver o log de auditoria.

## Histórico de configuração e reversão

Antes de sobrescrever um arquivo de configuração, o Gately salva o conteúdo antigo como uma versão em `history/<nome>/` dentro do diretório de configuração, mantendo as 30 versões mais recentes de cada configuração. `account.json` (credenciais da conta) e `tokens.json` (tokens) não têm histórico.

Na aba "Histórico de configuração" da página "Sistema → Logs":

1. Escolha o arquivo de configuração e veja o horário e o tamanho de cada versão salva;
2. Clique em "Comparar" para ver as diferenças linha a linha entre essa versão e a configuração atual;
3. Clique em "Reverter" para restaurar essa versão; em seguida o Gately reinicia automaticamente para aplicá-la.

O conteúdo atual antes da reversão também é salvo como uma nova versão, então a própria reversão pode ser desfeita.
