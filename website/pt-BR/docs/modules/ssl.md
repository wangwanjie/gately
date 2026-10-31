# Certificados SSL

Emita e renove certificados automaticamente para uso pelos [Serviços Web](/pt-BR/docs/modules/web) ou exporte-os para outros softwares.

Quando vários certificados cobrem o mesmo domínio, é usado o que expira por último; a ordem e os grupos da lista de certificados afetam apenas a exibição.

## Recursos

- Emissão e renovação automáticas via **ACME DNS-01**, reaproveitando os provedores de DNS do [DNS dinâmico](/pt-BR/docs/modules/ddns), sem precisar abrir as portas 80 / 443;
- **ACME HTTP-01**: dispensa provedor de DNS, mas exige que a porta 80 de cada domínio seja acessível pela internet; não suporta wildcard;
- Certificados wildcard e EAB (por exemplo, ZeroSSL);
- **Certificados de curta duração e de endereço IP**: com o perfil de curta duração do Let's Encrypt (validade de cerca de 6 dias), é possível emitir certificados para endereços IP públicos;
- Upload de um certificado existente ou leitura a partir de arquivos;
- Mapeamento para um diretório: quando um certificado muda, os arquivos são gravados no diretório indicado e um script é executado, para que outros serviços possam recarregar;
- Implantação automática: quando um certificado muda, ele é copiado para vários diretórios locais, enviado por SSH a outros hosts com a execução de um comando, ou os contêineres Docker que o usam são reiniciados.

## Validação DNS-01

Escolha DNS-01 em "Método de validação" do certificado e selecione um provedor de DNS. Na emissão, o Gately cria o registro TXT `_acme-challenge` pela API do provedor e o remove depois da validação.

"Domínio raiz" é a zona DNS que contém o registro. Quando fica vazio, o Gately descobre a zona autoritativa do registro de validação com uma consulta SOA, então um subdomínio delegado a outro provedor de DNS (por exemplo, `web.example.com` hospedado separado de `example.com`) e sufixos de vários níveis como `example.com.cn` são identificados corretamente. Se a consulta falhar, a zona é deduzida a partir do nome de domínio e o motivo fica registrado no log. Também é possível preencher o domínio raiz manualmente, por exemplo `web.example.com`.

## Rede de emissão

A opção "Rede" do certificado restringe a rede de saída usada na emissão: "Automática" deixa a escolha para o sistema, e "Somente IPv4" ou "Somente IPv6" faz com que o acesso à CA, as chamadas à API do provedor de DNS e a verificação de propagação do registro TXT usem apenas essa família de endereços. Se o host tiver endereço IPv6 mas o IPv6 não funcionar de fato, a verificação de propagação no modo automático pode expirar ao consultar os servidores DNS autoritativos; nesse caso, escolha "Somente IPv4".

## Validação HTTP-01

Escolha HTTP-01 em "Método de validação" do certificado. Na emissão, a CA acessa `http://<domínio>/.well-known/acme-challenge/<token>`:

- Se um [Serviço Web](/pt-BR/docs/modules/web) já escuta na porta 80, ele responde diretamente, sem afetar os sites existentes;
- Se a porta 80 estiver livre, o Gately escuta nela temporariamente durante a emissão e a fecha logo em seguida;
- Se o roteador redireciona a porta 80 pública para outra porta deste host, informe essa porta em "Porta temporária".

Muitos provedores de banda larga residencial bloqueiam a entrada na porta 80; nesse caso, use DNS-01.

## Certificados de curta duração e de endereço IP

"Perfil do certificado" corresponde ao profile do ACME:

- **Padrão**: definido pela CA; no Let's Encrypt são 90 dias;
- **Curta duração (certificado IP)**: o perfil `shortlived` do Let's Encrypt, com validade de cerca de 6 dias. O Gately o renova automaticamente antes do vencimento conforme "Antecedência para certificados de curta duração (horas)" em "Configurações de renovação" (24 horas por padrão) e, em caso de falha, tenta de novo a cada hora;
- Também é possível informar outro nome de perfil suportado pela CA.

Para solicitar um certificado de endereço IP, informe um endereço IP público (IPv4 ou IPv6, por exemplo `203.0.113.10` ou `2001:db8::1`) nos domínios; IPs e domínios podem ficar no mesmo certificado. Requisitos:

- Usar o Let's Encrypt e escolher o perfil de curta duração e HTTP-01 em "Método de validação". A CA acessa `http://<IP>/.well-known/acme-challenge/<token>`, por isso a porta 80 desse IP precisa ser acessível pela internet. Ao informar um IP, o editor mostra um aviso; basta clicar em "Configurar automaticamente";
- Endereços privados, de loopback e `100.64.0.0/10` não passam na validação de uma CA pública; uma CA personalizada (como um servidor ACME próprio na rede local) não tem essas restrições;
- ZeroSSL e Google Trust Services não emitem certificados de endereço IP via ACME.

Ao acessar HTTPS por endereço IP, o navegador não envia nome de domínio (SNI), então o Gately escolhe o certificado IP correspondente ao endereço local da conexão; atrás de NAT ou redirecionamento de portas, quando o endereço local difere do IP público do certificado, é usado o certificado IP que expira por último.

Ao alternar o perfil de um certificado já emitido entre padrão e curta duração, ele é reemitido imediatamente com o novo perfil. Nos certificados de curta duração, o lembrete de vencimento só é enviado quando o tempo restante fica abaixo da metade da antecedência, o que indica que a renovação falhou várias vezes.

## Mapeamento de certificados

Com um certificado ativado, o Gately grava `.crt`, `.key`, `.pem` e `_issuerCertificate.crt` no diretório de mapeamento e executa o script de alteração.

## Implantação automática

Em "Implantação automática" do certificado, você pode adicionar vários destinos, executados automaticamente depois que o certificado é emitido, renovado ou atualizado:

- **Copiar para diretório local**: grava o certificado em um diretório desta máquina e executa localmente o comando "Executar após copiar". É possível adicionar vários diretórios, cada um com um comando diferente. Se os arquivos de um diretório forem apagados ou alterados, a próxima verificação os copia de novo. Quando o Gately roda no Docker, o diretório e o comando ficam dentro do contêiner;
- **Enviar para host SSH**: escolha uma conexão SSH já existente no [Terminal Web](/pt-BR/docs/modules/terminal), reaproveitando seu endereço, método de autenticação e proxy. Os arquivos do certificado (`<observação>.crt`, `.key`, `.pem` e `_issuerCertificate.crt`, iguais aos do mapeamento de certificados, por isso a observação é obrigatória) são gravados no diretório remoto e, em seguida, o comando de "Executar após enviar" roda no host remoto, por exemplo `systemctl reload nginx`. O comando pode usar `$certName`, `$certDir`, `$certFile` e `$keyFile`. Cada arquivo é gravado primeiro em um arquivo temporário com as permissões definidas e depois renomeado para substituir o original; a chave privada fica com permissão 0600;
- **Reiniciar contêineres locais**: quando o certificado muda, os contêineres Docker selecionados são reiniciados em sequência. Útil para serviços que montam o diretório de mapeamento e precisam reiniciar para carregar o novo certificado.

"Copiar para diretório local" e "Enviar para host SSH" aceitam nomes de arquivo para o certificado (cadeia completa) e a chave privada, como `fullchain.pem` e `privkey.pem`. Com ambos preenchidos, apenas esses dois arquivos são gravados e a observação pode ficar vazia; vazios, são usados os nomes do mapeamento de certificados. `$certFile` e `$keyFile` no comando apontam para os arquivos realmente gravados.

A implantação automática nunca confia em um host desconhecido: conecte-se uma vez pela conexão SSH escolhida no Terminal Web para registrar a chave do host antes do primeiro envio. Implantações com falha são repetidas a cada hora e geram o evento "Certificado: falha na implantação", que pode ser assinado na [Central de notificações](/pt-BR/docs/modules/notify). O botão "Implantar agora" no cartão do certificado ignora o intervalo de nova tentativa, implanta imediatamente em todos os destinos ativados e mostra o resultado de cada um.

Ao editar o certificado, destinos com configuração inalterada não são implantados de novo; alterar o diretório remoto, o comando ou os contêineres faz uma nova implantação.

## Nota de segurança

As chaves privadas ficam no `ssl.json` do diretório de configuração. Guarde esse arquivo com cuidado e, se for versioná-lo, confirme que o repositório é privado.
