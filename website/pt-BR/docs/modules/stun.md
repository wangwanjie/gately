# Travessia STUN

Quando a sua operadora não oferece uma porta pública fixa, a travessia STUN usa NAT hole punching para obter um endereço e uma porta públicos mapeados, permitindo que serviços da rede local sejam acessados pela internet.

## Recursos

- Travessia TCP / UDP com keep-alive;
- UPnP / NAT-PMP cria mapeamentos de porta no roteador automaticamente;
- O encaminhamento embutido envia a porta pública obtida pela travessia diretamente para um destino da rede local;
- Webhooks e scripts são disparados quando o endereço ou a porta pública muda, e podem ser usados para atualizar o DNS, avisar clientes e mais.
- Controle de acesso por IP de origem (lista de permissões / de bloqueio) para o encaminhamento integrado.

## Uso combinado com outros módulos

A porta pública obtida via STUN costuma mudar. Combine com:

- [DNS dinâmico](/pt-BR/docs/modules/ddns) ou um Webhook / script para sincronizar o novo endereço com os registros DNS; os valores de registro do DNS dinâmico também podem usar `{STUN_nome da regra_PORT}` e variáveis semelhantes, sincronizadas assim que o endereço da regra muda, veja [Variáveis no valor do registro](/pt-BR/docs/modules/ddns#record-vars);
- [Serviços Web](/pt-BR/docs/modules/web) para oferecer um ponto de entrada HTTPS.
- Os sites de redirecionamento dos Serviços Web podem referenciar o endereço público atual da regra com variáveis como `{STUN_NomeRegra_PORT}` e seguem a porta nova automaticamente quando ela muda; veja [Variáveis de redirecionamento](/pt-BR/docs/modules/web#redirect-vars).

## Limitações e NAT em várias camadas

- **NAT de operadora (CGNAT)**: quando o WAN do roteador recebe um endereço interno como `100.64.0.0/10`, a travessia depende do tipo de NAT da operadora. Com cone completo (qualquer endereço externo alcança a porta mapeada) funciona; com NAT de porta restrita ou simétrico não, e nesse caso use [Tailscale, frp ou Cloudflare Tunnel](/pt-BR/docs/modules/tunnels).
- **UPnP atrás de vários NATs**: com UPnP / NAT-PMP ativado, o Gately mapeia no roteador a porta do túnel para este host. Se o endereço externo do roteador for diferente do endereço público obtido pelo STUN, há outro NAT do lado de fora e a porta pública pertence a ele, então o Gately mantém só o mapeamento da porta do túnel e deixa de adicionar um mapeamento para a porta pública.
- **Fixe a porta local e libere-a no firewall**: uma porta local vazia é escolhida ao acaso a cada início. Se o host usa firewall (por exemplo, o firewall do sistema do fnOS), fixe a porta local da regra e libere-a no firewall (você pode reservar uma faixa de portas para as regras STUN); caso contrário o firewall descarta as conexões de entrada e o endereço fica inacessível.
- **TCP e o NAT de cone do roteador**: em alguns roteadores, “NAT1 / cone completo” vale só para UDP e a porta de origem TCP é reescrita na saída. Regras TCP precisam de UPnP ou de um mapeamento da porta do túnel para este host no roteador.

## Liberar porta no firewall {#auto-firewall}

Com "Liberar porta no firewall" ativado em uma regra, o Gately libera a porta do canal no firewall deste host (e a porta interna de UPnP / NAT-PMP, se informada) e remove a liberação quando a regra para ou é excluída. Use quando o Gately roda em um roteador como o OpenWrt ou em um host Linux com firewall ativo. Somente Linux.

- nftables: uma regra de liberação é inserida no topo de cada cadeia de filtro no hook input (por exemplo, OpenWrt fw4 e firewalld);
- iptables: uma regra de liberação é inserida no topo da cadeia INPUT da tabela filter (por exemplo, OpenWrt fw3 e ufw);
- as regras levam o comentário `gately-open:`; recarregar o firewall as apaga, e o Gately verifica a cada 30 segundos e as adiciona de novo;
- no Docker é preciso a rede host e `cap_add: NET_ADMIN`; o cartão da regra mostra o resultado ou o motivo da falha;
- firewalls que não passam por nftables / iptables, como o firewall do sistema do fnOS, continuam exigindo liberar a porta manualmente nas configurações deles.

## Controle de acesso por IP de origem

Com o encaminhamento integrado, cada regra pode usar uma lista de permissões ou de bloqueio que decide pelo IP de origem se o tráfego é encaminhado ao destino. As listas e as listas de IP assinadas funcionam como no [Redirecionamento de portas](/pt-BR/docs/modules/portforward#controle-de-acesso-por-ip-de-origem), e os banimentos da Prevenção de intrusão também têm prioridade. A lista vale só para o encaminhamento; consultas STUN e a verificação do canal não são afetadas. Editar a lista não refaz a travessia, então a porta pública continua a mesma. O UDP é verificado por pacote. Sem o encaminhamento integrado, as conexões externas não passam pelo Gately e a lista não tem efeito.

## Dica de implantação

O STUN precisa escutar diretamente nas portas do host. Em implantações com Docker, recomendamos `--network host`.
