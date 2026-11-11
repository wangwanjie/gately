# Firewall WAF

Um firewall de aplicações web baseado no [Coraza](https://coraza.io/) e no OWASP CRS v4, que pode ser associado a sites dos [Serviços Web](/pt-BR/docs/modules/web).

## Recursos

- Conjunto de regras OWASP CRS v4 embutido, com arquivos de regras que podem ser ativados individualmente;
- Regras SecLang personalizadas;
- Os grupos de regras podem ser reordenados; os logs de detecção registram IP do cliente, domínio, caminho e ID da regra, e podem ser filtrados por domínio, IP ou palavras-chave;
- Modo de bloqueio e modo apenas de detecção;
- Limite de inspeção do corpo da requisição ajustável: acima do limite, a requisição é rejeitada ou só a parte inicial é inspecionada; a inspeção do corpo pode ser pulada por caminho;
- Modo de compatibilidade com WebDAV: libera os métodos, os cabeçalhos e qualquer tipo de conteúdo de upload usados por nuvens pessoais e WebDAV;
- Pode usar um mecanismo de detecção SafeLine auto-hospedado, sozinho ou junto com grupos de regras;
- Várias instâncias, referenciadas pelos sites conforme necessário.

## Recomendações

1. Em um site novo, comece com "Apenas detectar" e acompanhe os logs por algum tempo para confirmar que não há falsos positivos;
2. Ajuste os arquivos de regras ou adicione exceções personalizadas para os falsos positivos;
3. Depois mude para "Bloquear".

## Requisições do painel de administração

Quando o painel de administração é acessado por proxy reverso através de um site com WAF, as requisições com uma sessão de administrador válida desta instância não passam pelo WAF (SafeLine incluído). As configurações salvas costumam conter caminhos de arquivo (como as observações gravadas pela importação do nginx), HTML (páginas de erro personalizadas) e scripts (implantação de certificados), que as regras do CRS contra travessia de caminho, XSS e injeção de comandos bloqueariam com 403. A sessão é assinada pelo servidor e só o papel de administrador conta; requisições de contas somente leitura, visitantes e painéis de outras instâncias continuam sendo verificadas.

## Ordem dos grupos de regras

Os grupos de regras de uma instância são carregados de cima para baixo; use os botões mover para cima e para baixo para reordená-los. Na mesma fase, as regras anteriores executam primeiro:

- Regras de bloqueio (deny e afins) funcionam em qualquer posição;
- Regras de liberação ou exclusão em tempo de execução (`allow`, `ctl:ruleRemoveById`, `ctl:ruleRemoveTargetById` etc.) devem ficar acima do grupo OWASP CRS, senão o CRS pode bloquear a requisição antes;
- Diretivas de configuração como `SecRuleRemoveById` e `SecRuleUpdateTargetById` só afetam regras já carregadas, então devem ficar abaixo do grupo CRS; acima delas não têm efeito.

## Sites com upload de arquivos grandes

Por padrão, o WAF só inspeciona corpos de requisição de até 12,5 MB e rejeita requisições maiores (413), o que impede o upload de arquivos grandes em nuvens pessoais, WebDAV e gerenciadores de arquivos. Ajuste isso em "Inspeção do corpo da requisição" da instância:

- **Limite de inspeção**: 0 significa o padrão de 12,5 MB, até 1024 MB;
- **Acima do limite**: "Rejeitar a requisição" mantém o comportamento anterior; "Inspecionar só a parte inicial" inspeciona apenas a parte dentro do limite e envia o resto inalterado ao backend, o que é indicado para sites com uploads grandes;
- **Compatível com WebDAV e uploads**: por padrão, o OWASP CRS só permite GET, HEAD, POST e OPTIONS e bloqueia os cabeçalhos `Lock-Token`, `If` e `Content-Range` e os tipos de conteúdo fora da sua lista permitida, por isso clientes WebDAV não conseguem listar pastas, fazer upload nem bloquear arquivos. Ao ativar, PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK e outros métodos, além desses cabeçalhos, são liberados, e o tipo de conteúdo dos uploads deixa de ser restringido; o corpo de PUT e de `application/octet-stream` é tratado como conteúdo de arquivo e deixa de ser inspecionado como parâmetros de formulário. As verificações de injeção, path traversal e outros ataques continuam ativas;
- **Caminhos sem inspeção do corpo**: um prefixo de caminho por linha (como `/api/upload` ou `/remote.php/dav/`). Requisições correspondentes não têm o corpo inspecionado; cabeçalhos, URL e parâmetros continuam sendo inspecionados normalmente.

Recomendamos criar uma instância separada para nuvens pessoais e sites WebDAV, com "Compatível com WebDAV e uploads" ativado e "Inspecionar só a parte inicial" selecionado; se necessário, adicione as rotas de upload em "Caminhos sem inspeção do corpo". Mantenha os demais sites com as configurações padrão.

curl, rclone e outros clientes enviam o cabeçalho `Expect: 100-continue` ao fazer upload de conteúdos maiores, e o CRS 4 o bloqueia por padrão. Todas as instâncias do Gately liberam esse cabeçalho (o próprio Gately trata o 100-continue), sem necessidade de configuração extra.

## Usar o SafeLine

O [SafeLine](https://waf.chaitin.com/) é um firewall de aplicações web da Chaitin Tech que detecta ataques com um mecanismo de análise semântica e gera poucos falsos positivos. O Gately pode enviar as requisições ao seu mecanismo de detecção SafeLine auto-hospedado (safeline-detector) e permiti-las ou bloqueá-las conforme o resultado; pontos de entrada, certificados, autenticação e banimentos continuam no Gately, então o SafeLine não precisa mais ser uma camada extra de proxy reverso.

1. Instale o SafeLine Community Edition seguindo a documentação oficial; o diretório de instalação padrão é `/data/safeline`.
2. Escolha como conectar ao mecanismo de detecção:
   - **Socket Unix** (Gately e SafeLine na mesma máquina): o mecanismo escuta por padrão em `/data/safeline/resources/detector/snserver.sock`, então use o endereço `unix:/data/safeline/resources/detector/snserver.sock`. Se o Gately roda no Docker, monte o diretório `/data/safeline/resources/detector` no contêiner do Gately e use o caminho dentro do contêiner.
   - **TCP**: adicione as duas linhas abaixo ao `/data/safeline/resources/detector/detector.yml` e execute `docker restart safeline-detector`. Depois de mudar para TCP, o mecanismo deixa de escutar no socket Unix.

     ```yaml
     bind_addr: 0.0.0.0
     listen_port: 8000
     ```

     Use como endereço o IP e a porta do contêiner safeline-detector, por exemplo `172.22.222.5:8000`; `docker inspect safeline-detector | grep IPAddress` mostra o IP do contêiner. Se o Gately roda em uma rede bridge do Docker, conecte-o à rede do SafeLine com `docker network connect safeline-ce <nome do contêiner do Gately>`. O mecanismo de detecção não tem autenticação; não exponha a porta 8000 na internet.
3. Na instância de WAF, ative "Usar SafeLine", informe o endereço e clique em "Testar conexão". O Gately envia um heartbeat e depois uma requisição de injeção SQL de exemplo para inspeção; "o ataque de exemplo foi detectado" indica que o mecanismo funciona. Esse ataque de exemplo aparece nos registros de ataques do console do SafeLine com origem 127.0.0.1 e domínio gately-test.invalid.
4. Referencie a instância em um site do Serviço Web.

Observações:

- Com o SafeLine conectado, os grupos de regras podem ficar vazios; se você os mantiver, as requisições passam primeiro pelo SafeLine e depois pelos grupos de regras. No modo "Somente detectar", os ataques detectados pelo SafeLine apenas são registrados e a requisição segue.
- Requisições bloqueadas: navegadores veem uma página de bloqueio com o ID do evento e os demais clientes recebem o mesmo JSON dos plugins oficiais do SafeLine (com `event_id`). O código de status é o sugerido pelo mecanismo, normalmente 403. Os bloqueios contam nos logs de acesso (`deny=waf`), nas estatísticas do site e nos eventos de bloqueio do WAF, e a predefinição "Bloqueios do WAF" da Proteção contra intrusões também se aplica.
- Os ataques também são registrados no console do SafeLine e podem ser encontrados pelo ID do evento; esses registros não pertencem a nenhum site do SafeLine.
- Por padrão, até 1024 KB do corpo da requisição são enviados para inspeção; o restante não é inspecionado e é encaminhado ao backend sem alterações. "Caminhos sem inspeção do corpo" e "Compatível com WebDAV e uploads" também valem para o SafeLine; corpos gRPC de tamanho desconhecido sobre HTTP/2 não são inspecionados.
- O tempo limite de detecção é de 1 segundo por padrão. Quando o mecanismo está indisponível (falha de conexão ou tempo esgotado), as requisições são permitidas por padrão, a instância mostra "SafeLine fora do ar" e um erro é registrado; com "Rejeitar requisições", essas requisições recebem 503. Logo após iniciar, o mecanismo pode retornar resultados não reconhecidos por alguns segundos, tratados da mesma forma.
- Apenas as requisições são inspecionadas; as respostas não são enviadas ao SafeLine.

## Verificação

O WAF é testado com payloads de ataque reais e bloqueia ataques como path traversal.
