# Subir o ExtratorGLPI no 10.100.0.97

## Antes de começar

**A porta 8443 não pode ser usada neste servidor.** Já há uma aplicação Flask
com login respondendo nela (certificado `CN=10.100.0.97`). O pacote está
configurado para a **8030**, que estava livre e segue o padrão dos outros
serviços do host (DiscMotiva 8010, TaskMotiva 8020).

**Libere o IP do servidor no GLPI antes de subir.** O cliente da API tem
restrição por faixa de IPv4, e hoje quem consulta é a sua estação
(10.100.1.119). Sem liberar o **10.100.0.97**, o serviço sobe mas não
autentica.

> GLPI → Configurar → Geral → API → o cliente da API → *Intervalo de IPv4*.
> Coloque `10.100.0.97` (ou a faixa `10.100.0.0/21`, que cobre os dois).

## Passos

Do seu Windows, na pasta do projeto:

```powershell
scp extratorglpi-deploy.tar.gz SEU_USUARIO@10.100.0.97:/tmp/
ssh SEU_USUARIO@10.100.0.97
```

Já no servidor:

```bash
cd /tmp
tar xzf extratorglpi-deploy.tar.gz
cd extratorglpi-deploy
sudo bash instalar.sh
```

O script confere a porta, confere o Docker (e pergunta antes de instalar,
se faltar), copia tudo para `/opt/extratorglpi`, carrega a imagem do arquivo
— sem baixar nada da internet — e sobe o container.

No fim, acesse **https://10.100.0.97:8030**.

O certificado é autoassinado e cobre `10.100.0.97`, `localhost` e `127.0.0.1`,
válido até setembro de 2029. O navegador vai avisar na primeira vez, igual à
aplicação que já roda na 8443.

## O que o script não faz

Não mexe em nginx, systemd, firewall nem nas outras aplicações. Se o
`firewalld`/`ufw` estiver ativo no host, libere a porta à mão:

```bash
sudo ufw allow 8030/tcp          # Ubuntu/Debian
sudo firewall-cmd --add-port=8030/tcp --permanent && sudo firewall-cmd --reload
```

## Se o Docker não estiver instalado

O script pergunta antes de instalar. Vale saber o que isso muda no host: o
Docker cria a bridge `docker0` e escreve regras de `iptables`. Em um servidor
que já roda DiscMotiva, SFU, Redis e nginx, isso é de baixo risco, mas não é
nada — se preferir não instalar, dá para publicar o extrator como serviço
systemd com o binário Go, igual aos outros. Me avise que eu preparo.

No Ubuntu os pacotes são `docker.io` e **`docker-compose-v2`** (verificado:
2.40.3). O `docker-compose-plugin`, que aparece na maioria dos tutoriais, só
existe no repositório oficial do Docker — pedir esse nome faz o `apt` falhar
inteiro, sem instalar nem o que existia. O script tenta os nomes em ordem, um
de cada vez, justamente por isso. Para instalar à mão:

```bash
sudo apt-get install -y docker.io docker-compose-v2
sudo systemctl enable --now docker
```

## Se o container reiniciar em laço

Olhe o log: `docker logs --tail 20 extratorglpi`.

- `open /certs/server.key: permission denied` — o container roda como `nonroot`
  (UID 65532) e não consegue ler a chave. O `instalar.sh` já ajusta o dono; para
  corrigir à mão:

  ```bash
  sudo chown 65532:65532 /opt/extratorglpi/certs/server.crt /opt/extratorglpi/certs/server.key
  cd /opt/extratorglpi && sudo docker compose restart
  ```

- `resposta nao e JSON ... HTTP 400` — o serviço chegou em algo que não é a API
  do GLPI. Rode o `diagnostico.sh` do pacote: ele compara a resolução de nome e
  a chamada de `initSession` a partir do host e da rede do container.

## Verificação

```bash
cd /opt/extratorglpi
docker compose ps                                  # deve estar "healthy"
curl -sk https://127.0.0.1:8030/api/health          # status: ok
docker compose logs -f                             # acompanhar
```

Do seu computador: abra `https://10.100.0.97:8030`, filtre um período curto
(um ou dois dias) e confira os cards antes de pedir um mês inteiro.

## Operação

```bash
cd /opt/extratorglpi
docker compose restart      # reiniciar
docker compose down         # parar
docker compose up -d        # subir de novo
```

Os logs ficam no journal do Docker, com rotação de 10 MB × 5 arquivos.

## Atualizar depois

Gere um pacote novo na estação e repita os mesmos passos. O `instalar.sh`
preserva o `.env` que estiver no servidor: a versão nova chega como
`.env.novo`, para você comparar antes de trocar.

## Diferenças da configuração local

| Item | Estação | Servidor |
| --- | --- | --- |
| Porta | 8443 | **8030** (8443 ocupada lá) |
| `GLPI_PARALELISMO` | 20 | **12** — o host tem 4 núcleos e já roda DiscMotiva, SFU, Redis e nginx; a varredura também concorre com quem está usando o GLPI |
| Certificado | `localhost` | `10.100.0.97` |

## Cuidado com os dados

O CSV exportado pode conter **senhas em texto claro**: vários chamados de reset
registram a senha temporária no follow-up público ou na própria solução, e é de
lá que sai a coluna "Motivo de encerramento". Follow-ups privados ficam de fora
por decisão de projeto, mas os públicos não têm como ser filtrados sem perder o
motivo real do encerramento. Trate o arquivo como documento sensível.
