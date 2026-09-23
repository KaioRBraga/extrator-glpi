# Extrator de chamados - GLPI

Serviço web que consulta os chamados do GLPI pela API REST, mostra os
indicadores do período e exporta um `.CSV` organizado por data de abertura,
data de solução, motivo de abertura e motivo de encerramento.

Escrito conforme o `SDD/SDD.docx`: back-end em Go, front-end em React +
Tailwind, servido **só** em HTTPS, com URL e credenciais lidas exclusivamente
do `.env`.

---

## Como funciona

- **Somente leitura.** O serviço nunca altera nada no GLPI.
- Consulta `apirest.php`: `initSession` (usuário/senha + `App-Token`) →
  `search/Ticket` paginado → `Ticket/{id}/ITILSolution` quando a solução não
  vem junto da busca → `killSession` ao encerrar.
- O **período** é o único filtro enviado ao GLPI. Status e busca textual são
  aplicados em memória, então filtrar de novo ou exportar não repete a
  varredura (cache com TTL configurável).
- **Motivo de abertura**: os chamados nascem de formulários cujo corpo é uma
  lista numerada (`9) SOLICITAÇÃO : ERRO NA FERRAMENTA`). O extrator lê esses
  campos e usa o primeiro rótulo configurado em `MOTIVO_ABERTURA_LABELS`. Se o
  formulário não tiver nenhum deles, cai para a categoria do GLPI e, por
  último, para o título — a coluna `Origem do motivo` no CSV diz de onde o
  valor veio.
- **Motivo de encerramento**: procurado em cascata — solução formal do chamado
  e, se não houver, o **último follow-up público**. Ambos vêm embutidos na
  própria busca (campos 24, 25 e 91), sem uma ida ao GLPI por chamado; só os
  poucos casos em que a busca não traz nada caem na consulta individual. Na prática
  a maioria dos chamados é encerrada sem preencher a solução: o desfecho fica
  no follow-up ("Normalizado.", "Acessos modificados..."), que é o "follow que
  entra na categoria de solução" citado no SDD. A coluna `Origem do
  encerramento` diz de onde o texto veio (`solucao`, `followup`, `em aberto`,
  `sem registro`).
  Chamado ainda não encerrado sai como **`Chamado em aberto`**; encerrado sem
  solução nem follow-up sai como `Encerrado sem descricao de solucao`.
  Follow-ups **privados** são ignorados de propósito: são notas internas da
  equipe e acabariam num CSV que circula fora dela.
- **Resolvido** é decidido pela data de solução/fechamento, não pelo rótulo de
  status — assim o cálculo não quebra se o GLPI estiver em outro idioma.
- **Usuário de rede**: o login alvo do chamado, lido do campo
  `USUÁRIO DE REDE` do formulário (rótulos configuráveis em
  `USUARIO_REDE_LABELS`). Não é o mesmo que o requerente — quem abre costuma
  ser o supervisor, e o chamado trata do login de outra pessoa. Sem formulário,
  o extrator procura o rótulo solto no texto (`Login de rede: joao.silva`).
- **Requerente, técnico e prioridade** vêm da busca como ID numérico mesmo com
  `expand_dropdowns`. O extrator traduz: prioridade por tabela e usuários por
  um cadastro carregado uma vez a cada 30 min (`search/User`).
- A tela abre no **mês corrente**. Limpar as datas traz o histórico inteiro,
  mas é uma varredura pesada — são centenas de chamados por dia.
- **O período é recortado em janelas de 14 dias**, buscadas em paralelo. Não é
  só velocidade: o GLPI devolve HTTP 500 quando o deslocamento da busca passa
  de ~14 mil chamados (a consulta é um `OFFSET` sobre vários `JOIN`s), mesmo
  pedindo poucos por vez. Recortando por data, cada consulta começa do zero.
  Se ainda assim o GLPI recusar um lote, o serviço espera e tenta de novo,
  depois reduz o tamanho e, em último caso, refaz a faixa **sem os campos de
  follow-up** — basta um chamado com uma thread de e-mail colada inteira para
  estourar a memória do PHP e derrubar a página toda. Esses poucos chamados
  têm o encerramento buscado um a um, e a consulta termina em vez de falhar.

## Configuração (`.env`)

Copie `.env.example` para `.env` e preencha. **Não há valor padrão para as
variáveis do GLPI e do TLS**: faltando qualquer uma, o serviço não sobe e diz
qual falta.

| Variável | Obrigatória | Para que serve |
| --- | --- | --- |
| `GLPI_URL` | sim | Raiz do GLPI (`https://glpi.empresa.com.br`). O `/apirest.php` é acrescentado sozinho. |
| `GLPI_APP_TOKEN` | sim | App-Token da API (GLPI: Configurar → Geral → API → adicionar cliente API). |
| `GLPI_USUARIO` / `GLPI_SENHA` | sim | Usuário de serviço do GLPI com perfil de leitura nos chamados. |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | sim | Certificado e chave do HTTPS. |
| `GLPI_VERIFY_SSL` | não (`true`) | `false` quando o GLPI usa certificado de CA interna. |
| `SERVER_PORT` | não (`8443`) | Porta HTTPS do serviço. |
| `MOTIVO_ABERTURA_LABELS` | não | Rótulos do formulário usados como motivo, em ordem de prioridade. |
| `USUARIO_REDE_LABELS` | não | Rótulos do formulário que contêm o login alvo, em ordem de prioridade. |
| `GLPI_PARALELISMO` | não (`16`) | Consultas simultâneas ao GLPI. Subir acelera a primeira carga e pressiona mais o GLPI. |
| `GLPI_LOTE_BUSCA` | não (`1000`) | Chamados por página da busca. Se o GLPI recusar o tamanho (HTTP 500), o serviço reduz o lote sozinho e continua. |
| `CACHE_TTL` | não (`5m`) | Validade do cache das consultas (`0s` desliga). Períodos passados não mudam: vale usar `30m` ou mais. |
| `AQUECER_CACHE` | não (`true`) | Mantém o mês corrente pronto no cache, em segundo plano. Com isso a tela abre instantânea; o custo é uma varredura por ciclo, sempre, mesmo sem ninguém usando. |
| `MAX_CHAMADOS` | não (`5000`) | Teto de chamados por consulta. Ao atingir, a tela avisa que o período foi truncado. |
| `TEMPO_MAXIMO_CONSULTA` | não (`10m`) | Tempo máximo de uma consulta antes de desistir. |
| `LOG_LEVEL` | não (`info`) | `debug`, `info`, `warn`, `error`. |

## Subir com Docker

```bash
cp .env.example .env         # preencha as credenciais
cp /caminho/do/certificado.crt certs/server.crt
cp /caminho/da/chave.key      certs/server.key
docker compose up --build
```

O serviço fica em `https://localhost:8443`. O `docker-compose.yml` monta
`./certs` como somente leitura e injeta o `.env` — nenhum segredo entra na
imagem.

Certificado só para desenvolvimento:

```bash
go run ./cmd/gerarcert -saida ./certs -host localhost,127.0.0.1
```

## Rodar sem Docker

```bash
cd web && npm ci && npm run build && cd ..   # gera web/dist (embutido no binário)
go build -o extrator ./cmd/extrator
./extrator                                   # lê ./.env; use -env para outro caminho
```

Durante o desenvolvimento do front, `cd web && npm run dev` sobe o Vite em
`http://localhost:5173` com proxy de `/api` para o backend em HTTPS.

## API

| Rota | Descrição |
| --- | --- |
| `GET /api/chamados` | Página de chamados + indicadores. Parâmetros: `inicio`, `fim` (`AAAA-MM-DD`), `status` (`todos`/`resolvidos`/`abertos`), `q`, `pagina`, `por_pagina`, `atualizar=1` (ignora o cache). |
| `GET /api/export.csv` | Mesmos filtros, sem paginação. Devolve o CSV. |
| `GET /api/health` | Estado da sessão com o GLPI. |

Erros de parâmetro voltam como `400`; falha ou recusa do GLPI volta como `502`
com a mensagem em português.

O CSV sai em UTF-8 com BOM e separador `;` — o Excel em pt-BR abre direto, sem
assistente de importação.

## Desempenho

Medido contra o GLPI da Motiva (~200 chamados/dia), primeira consulta sem cache:

| Período | Chamados | Primeira consulta | Depois (cache) |
| --- | --- | --- | --- |
| 1 mês | 4.813 | ~24 s | instantâneo |
| 3 meses | 18.643 | ~83 s | instantâneo |
| 6 meses | 37.508 | ~6 min | instantâneo |

Depois da primeira carga o `CACHE_TTL` assume: filtrar de novo, mudar o status
ou exportar o CSV não repete a varredura (o CSV de 6 meses, 11 MB, sai em
menos de 1 s). Para períodos maiores, suba `MAX_CHAMADOS` e
`TEMPO_MAXIMO_CONSULTA` juntos.

O limite não é o extrator, é o GLPI: acima de uns 15 mil chamados ele começa a
recusar consultas, e uma varredura longa concorre com quem está usando o
sistema. Se precisar aliviar, baixe `GLPI_PARALELISMO`.

## Testes

```bash
go test ./...
```

Cobrem o parser dos formulários (com casos tirados de chamados reais), a
leitura das respostas do GLPI, a cascata solução → follow-up (incluindo o
descarte de follow-ups privados), o filtro de status e busca, o cache e a
geração do CSV com a marcação `Chamado em aberto`.

## Estrutura

```
cmd/extrator      binário do serviço (TLS, encerramento limpo, -healthcheck)
cmd/gerarcert     certificado autoassinado para desenvolvimento
internal/config   leitura estrita do .env
internal/glpi     cliente da API, parser de motivo, filtros e estatísticas
internal/api      rotas HTTP, CSV e middlewares
internal/cache    cache com TTL das consultas
web               front-end React + Tailwind (embutido no binário via go:embed)
```
