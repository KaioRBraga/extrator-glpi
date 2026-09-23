package glpi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IDs dos campos de busca do Ticket no GLPI.
const (
	campoTitulo         = "1"
	campoID             = "2"
	campoPrioridade     = "3"
	campoRequerente     = "4"
	campoTecnico        = "5"
	campoCategoria      = "7"
	campoStatus         = "12"
	campoDataAbertura   = "15"
	campoDataFechamento = "16"
	campoDataSolucao    = "17"
	campoDescricao      = "21"
	campoRecebedor      = "22"
	campoSolucao        = "24"
	campoEntidade       = "80"

	// Follow-ups vem embutidos na propria busca, como arrays paralelos
	// (conteudo, privado, data). Evita uma ida ao GLPI por chamado.
	campoFollowTexto   = "25"
	campoFollowPrivado = "91"
	campoFollowData    = "36"
)

// camposBase e o forcedisplay minimo de um chamado.
var camposBase = []string{
	campoID, campoTitulo, campoStatus, campoDataAbertura, campoDataFechamento,
	campoDataSolucao, campoDescricao, campoCategoria, campoPrioridade,
	campoRequerente, campoTecnico, campoRecebedor, campoSolucao, campoEntidade,
}

// camposExibidos acrescenta os follow-ups, que evitam uma ida por chamado.
var camposExibidos = append(append([]string{}, camposBase...),
	campoFollowTexto, campoFollowPrivado, campoFollowData)

// Tamanho de pagina. Medido contra o GLPI da Motiva: 1000 por pagina levou
// 18s para o mes inteiro contra 24s com 300, e 2000 derrubou o servidor com
// HTTP 500 -- dai o teto.
const (
	loteMaximo = 1000
	loteMinimo = 50
)

// Periodo e a janela de abertura dos chamados consultada no GLPI.
type Periodo struct {
	Inicio time.Time
	Fim    time.Time
}

// BuscarChamados devolve os chamados abertos dentro do periodo, ja com motivo
// de abertura e de encerramento resolvidos.
//
// O periodo e o unico filtro enviado ao GLPI: status e busca textual sao
// aplicados em memoria pela camada de API, o que permite reaproveitar o mesmo
// resultado em cache entre listagem, cards e exportacao.
func (c *Client) BuscarChamados(ctx context.Context, p Periodo, r Rotulos) ([]Chamado, error) {
	fatias := fatiarPeriodo(p, diasPorFatia)

	brutos, err := c.buscarFatias(ctx, fatias)
	if err != nil {
		return nil, err
	}

	// Fatias e paginas podem se sobrepor: chamados novos entrando durante a
	// varredura empurram os demais, e as bordas de data se tocam. Dai a
	// deduplicacao por ID.
	vistos := make(map[string]bool)
	var chamados []Chamado
	for _, item := range brutos {
		ch := mapearChamado(item, r)
		if !dentroDoPeriodo(ch.abertura, p) || (ch.ID != "" && vistos[ch.ID]) {
			continue
		}
		vistos[ch.ID] = true
		chamados = append(chamados, ch)
		if c.maxChamados > 0 && len(chamados) >= c.maxChamados {
			break
		}
	}

	c.preencherSolucoes(ctx, chamados)
	c.resolverUsuarios(ctx, chamados)
	return chamados, nil
}

// diasPorFatia: em quantos dias o periodo e recortado.
//
// O GLPI devolve HTTP 500 quando o deslocamento da busca fica grande (visto a
// partir de ~14 mil chamados, mesmo pedindo 31 por vez): a consulta e um
// OFFSET em cima de varios JOINs. Recortando por data, cada consulta comeca do
// zero e o deslocamento nunca cresce.
const diasPorFatia = 14

// fatiarPeriodo corta o periodo em janelas, da mais recente para a mais
// antiga. Periodo sem data (ou curto) sai inteiro, numa fatia so.
func fatiarPeriodo(p Periodo, dias int) []Periodo {
	if p.Inicio.IsZero() || p.Fim.IsZero() || dias <= 0 {
		return []Periodo{p}
	}
	janela := time.Duration(dias) * 24 * time.Hour
	if p.Fim.Sub(p.Inicio) <= janela {
		return []Periodo{p}
	}

	var fatias []Periodo
	fim := p.Fim
	for fim.After(p.Inicio) {
		inicio := fim.Add(-janela)
		if inicio.Before(p.Inicio) {
			inicio = p.Inicio
		}
		fatias = append(fatias, Periodo{Inicio: inicio, Fim: fim})
		fim = inicio.Add(-time.Second)
	}
	return fatias
}

// buscarFatias baixa cada janela em paralelo e devolve os itens crus na ordem
// das fatias (da mais recente para a mais antiga).
func (c *Client) buscarFatias(ctx context.Context, fatias []Periodo) ([]map[string]any, error) {
	if len(fatias) == 1 {
		return c.buscarPeriodo(ctx, fatias[0])
	}

	resultados := make([][]map[string]any, len(fatias))
	erros := make([]error, len(fatias))

	fila := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < c.simultaneasBusca(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range fila {
				resultados[i], erros[i] = c.buscarPeriodo(ctx, fatias[i])
			}
		}()
	}
	for i := range fatias {
		select {
		case fila <- i:
		case <-ctx.Done():
			close(fila)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(fila)
	wg.Wait()

	var todos []map[string]any
	for i, err := range erros {
		if err != nil {
			return nil, err
		}
		todos = append(todos, resultados[i]...)
	}
	return todos, nil
}

// buscarPeriodo varre uma janela inteira, pagina a pagina.
func (c *Client) buscarPeriodo(ctx context.Context, p Periodo) ([]map[string]any, error) {
	// A primeira pagina informa o total, que diz quantas ainda faltam, e
	// tambem qual tamanho de lote este GLPI aguenta.
	primeira, total, lote, err := c.buscarPrimeiraPagina(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(primeira) == 0 {
		return nil, nil
	}

	itens := primeira
	if len(primeira) == lote && total > lote {
		restantes, err := c.buscarPaginasRestantes(ctx, p, total, lote)
		if err != nil {
			return nil, err
		}
		for _, pagina := range restantes {
			itens = append(itens, pagina...)
		}
	}
	return itens, nil
}

// simultaneasBusca e o tamanho do time de goroutines; o teto real de
// requisicoes simultaneas fica no semaforo do cliente.
func (c *Client) simultaneasBusca() int {
	return cap(c.vagasBusca)
}

// buscarPaginasRestantes baixa as paginas 2..N em paralelo.
//
// Sequencialmente sao dezenas de idas ao GLPI enfileiradas -- num mes cheio
// isso sozinho passava de 45 segundos.
func (c *Client) buscarPaginasRestantes(ctx context.Context, p Periodo, total, lote int) ([][]map[string]any, error) {
	limite := total
	if c.maxChamados > 0 && c.maxChamados < limite {
		limite = c.maxChamados
	}

	var inicios []int
	for i := lote; i < limite; i += lote {
		inicios = append(inicios, i)
	}
	if len(inicios) == 0 {
		return nil, nil
	}

	paginas := make([][]map[string]any, len(inicios))
	erros := make([]error, len(inicios))

	simultaneas := c.simultaneasBusca()

	fila := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < simultaneas; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range fila {
				dados, err := c.buscarPaginaComQuebra(ctx, p, inicios[i], lote)
				paginas[i], erros[i] = dados, err
			}
		}()
	}
	for i := range inicios {
		select {
		case fila <- i:
		case <-ctx.Done():
			close(fila)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(fila)
	wg.Wait()

	for _, err := range erros {
		if err != nil {
			return nil, err
		}
	}
	return paginas, nil
}

// esperasRetentativa: quanto esperar antes de cada nova tentativa depois de um
// erro 5xx do GLPI.
//
// Sob varias consultas ao mesmo tempo o GLPI passa a recusar requisicoes que
// sozinhas funcionariam -- inclusive lotes de 31 chamados em deslocamentos
// rasos. Nesse caso reduzir o lote nao adianta: o que resolve e dar um tempo
// ao servidor.
var esperasRetentativa = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// buscarPaginaComRetentativa insiste na mesma faixa enquanto o GLPI devolver
// 5xx, antes de partir para lotes menores.
func (c *Client) buscarPaginaComRetentativa(ctx context.Context, p Periodo, inicio, tamanho int, comFollowups bool) ([]map[string]any, int, error) {
	dados, total, err := c.buscarPagina(ctx, p, inicio, tamanho, comFollowups)
	for tentativa := 0; err != nil && tentativa < len(c.esperas); tentativa++ {
		var erroGLPI *Erro
		if !errors.As(err, &erroGLPI) || erroGLPI.Status < 500 {
			return nil, 0, err
		}

		espera := c.esperas[tentativa]
		c.log.Warn("GLPI devolveu erro, aguardando para tentar de novo",
			"inicio", inicio, "tamanho", tamanho, "espera", espera)
		select {
		case <-time.After(espera):
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
		dados, total, err = c.buscarPagina(ctx, p, inicio, tamanho, comFollowups)
	}
	return dados, total, err
}

// semFollowups repete a faixa sem os campos de follow-up.
//
// Um unico chamado com follow-up gigante (uma thread de e-mail colada inteira,
// por exemplo) estoura a memoria do PHP e derruba a pagina toda, por menor que
// ela seja. Sem esses campos a faixa passa, e o encerramento desses chamados e
// buscado depois, um a um.
func (c *Client) semFollowups(ctx context.Context, p Periodo, inicio, tamanho int) ([]map[string]any, bool) {
	dados, _, err := c.buscarPaginaComRetentativa(ctx, p, inicio, tamanho, false)
	if err != nil {
		return nil, false
	}
	c.log.Warn("faixa so passou sem os follow-ups; o encerramento dela sera buscado chamado a chamado",
		"inicio", inicio, "tamanho", tamanho)
	return dados, true
}

// buscarPrimeiraPagina baixa a pagina inicial, reduzindo o lote enquanto o
// GLPI recusar por tamanho. Devolve tambem o lote que funcionou, para que as
// paginas seguintes ja nasçam no tamanho certo.
func (c *Client) buscarPrimeiraPagina(ctx context.Context, p Periodo) ([]map[string]any, int, int, error) {
	tamanho := c.lote
	for {
		dados, total, err := c.buscarPaginaComRetentativa(ctx, p, 0, tamanho, true)
		if err == nil {
			return dados, total, tamanho, nil
		}

		var erroGLPI *Erro
		if !errors.As(err, &erroGLPI) || erroGLPI.Status < 500 {
			return nil, 0, 0, err
		}
		if tamanho <= loteMinimo {
			if dados, total, err := c.buscarPaginaComRetentativa(ctx, p, 0, tamanho, false); err == nil {
				c.log.Warn("primeira pagina so passou sem os follow-ups", "tamanho", tamanho)
				return dados, total, tamanho, nil
			}
			return nil, 0, 0, err
		}

		tamanho /= 2
		if tamanho < loteMinimo {
			tamanho = loteMinimo
		}
		c.log.Warn("GLPI recusou o lote na primeira pagina, reduzindo", "novo_tamanho", tamanho)
	}
}

// buscarPaginaComQuebra busca uma faixa e, se o GLPI engasgar com o tamanho
// (HTTP 500 por memoria), divide a faixa ao meio e tenta de novo.
//
// Instalacoes menores nao aguentam o mesmo lote; melhor ficar mais lento do
// que falhar a consulta inteira.
func (c *Client) buscarPaginaComQuebra(ctx context.Context, p Periodo, inicio, tamanho int) ([]map[string]any, error) {
	dados, _, err := c.buscarPaginaComRetentativa(ctx, p, inicio, tamanho, true)
	if err == nil {
		return dados, nil
	}

	var erroGLPI *Erro
	if !errors.As(err, &erroGLPI) || erroGLPI.Status < 500 {
		return nil, err
	}
	if tamanho <= loteMinimo {
		if dados, ok := c.semFollowups(ctx, p, inicio, tamanho); ok {
			return dados, nil
		}
		return nil, err
	}

	metade := tamanho / 2
	c.log.Warn("GLPI recusou o lote, tentando pedacos menores",
		"inicio", inicio, "tamanho", tamanho, "novo_tamanho", metade)

	var juntos []map[string]any
	for deslocamento := 0; deslocamento < tamanho; deslocamento += metade {
		parte, err := c.buscarPaginaComQuebra(ctx, p, inicio+deslocamento, metade)
		if err != nil {
			return nil, err
		}
		juntos = append(juntos, parte...)
		if len(parte) < metade {
			break
		}
	}
	return juntos, nil
}

// buscarPagina devolve um lote de chamados crus e o total da consulta.
func (c *Client) buscarPagina(ctx context.Context, p Periodo, inicio, tamanho int, comFollowups bool) ([]map[string]any, int, error) {
	campos := camposExibidos
	if !comFollowups {
		campos = camposBase
	}

	params := url.Values{}
	for i, campo := range campos {
		params.Set(fmt.Sprintf("forcedisplay[%d]", i), campo)
	}
	params.Set("range", fmt.Sprintf("%d-%d", inicio, inicio+tamanho-1))
	params.Set("sort", campoDataAbertura)
	params.Set("order", "DESC")
	params.Set("expand_dropdowns", "1")
	aplicarCriterioPeriodo(params, p)

	select {
	case c.vagasBusca <- struct{}{}:
		defer func() { <-c.vagasBusca }()
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}

	corpo, status, err := c.get(ctx, "/search/Ticket", params)
	if err != nil {
		return nil, 0, err
	}
	if status == 204 || len(corpo) == 0 {
		return nil, 0, nil
	}

	var resposta struct {
		TotalCount int              `json:"totalcount"`
		Data       []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(corpo, &resposta); err != nil {
		return nil, 0, &Erro{Operacao: "interpretando resposta da busca", Detalhe: trecho(corpo)}
	}
	return resposta.Data, resposta.TotalCount, nil
}

// aplicarCriterioPeriodo monta os criteria de data. As bordas sao alargadas em
// um segundo porque morethan/lessthan do GLPI sao exclusivos; o recorte exato
// fica por conta de dentroDoPeriodo.
func aplicarCriterioPeriodo(params url.Values, p Periodo) {
	n := 0
	add := func(searchtype, valor string) {
		if n > 0 {
			params.Set(fmt.Sprintf("criteria[%d][link]", n), "AND")
		}
		params.Set(fmt.Sprintf("criteria[%d][field]", n), campoDataAbertura)
		params.Set(fmt.Sprintf("criteria[%d][searchtype]", n), searchtype)
		params.Set(fmt.Sprintf("criteria[%d][value]", n), valor)
		n++
	}
	if !p.Inicio.IsZero() {
		add("morethan", p.Inicio.Add(-time.Second).Format("2006-01-02 15:04:05"))
	}
	if !p.Fim.IsZero() {
		add("lessthan", p.Fim.Add(time.Second).Format("2006-01-02 15:04:05"))
	}
}

func dentroDoPeriodo(t time.Time, p Periodo) bool {
	if t.IsZero() {
		// Sem data legivel, e melhor manter o chamado do que perde-lo em silencio.
		return true
	}
	if !p.Inicio.IsZero() && t.Before(p.Inicio) {
		return false
	}
	if !p.Fim.IsZero() && t.After(p.Fim) {
		return false
	}
	return true
}

// Rotulos diz quais campos do formulario alimentam cada coluna.
type Rotulos struct {
	Motivo  []string
	Usuario []string
}

func mapearChamado(item map[string]any, r Rotulos) Chamado {
	descricao := LimparTexto(valor(item, campoDescricao, "content"))
	titulo := strings.TrimSpace(LimparTexto(valor(item, campoTitulo, "name")))
	// Rotulos de dropdown chegam escapados ("Sistemas &gt; Internos").
	categoria := TextoSimples(valor(item, campoCategoria, "itilcategories_id"))
	statusBruto := TextoSimples(valor(item, campoStatus, "status"))

	abertura := parseData(valor(item, campoDataAbertura, "date"))
	solucao := parseData(valor(item, campoDataSolucao, "solvedate"))
	fechamento := parseData(valor(item, campoDataFechamento, "closedate"))

	// A data de solucao e a fonte mais confiavel de "resolvido": nao depende do
	// idioma do rotulo de status devolvido com expand_dropdowns.
	dataFim := solucao
	if dataFim.IsZero() {
		dataFim = fechamento
	}
	resolvido := !dataFim.IsZero() || statusResolvido(statusBruto)

	motivo, origem := MotivoAbertura(descricao, categoria, titulo, r.Motivo)
	textoFim, origemBruta, pendente := encerramentoEmbutido(item)
	encerramento, origemFim := MotivoEncerramento(textoFim, resolvido, origemBruta)
	prioridadeBruta := valor(item, campoPrioridade, "priority")

	requerente := valor(item, campoRequerente, "users_id_requester")
	if requerente == "" {
		requerente = valor(item, campoRecebedor, "users_id_recipient")
	}

	return Chamado{
		ID:                 valor(item, campoID, "id"),
		Titulo:             titulo,
		Status:             rotularStatus(statusBruto),
		DataAbertura:       formatarData(abertura),
		HoraAbertura:       formatarHora(abertura),
		DataSolucao:        formatarData(dataFim),
		HoraSolucao:        formatarHora(dataFim),
		Atendimento:        formatarAtendimento(abertura, dataFim),
		MotivoAbertura:     motivo,
		OrigemMotivo:       origem,
		MotivoEncerramento: encerramento,
		OrigemEncerramento: origemFim,
		UsuarioRede:        UsuarioRede(descricao, r.Usuario),
		Requerente:         limparLista(requerente),
		Tecnico:            limparLista(valor(item, campoTecnico, "users_id_assign")),
		Categoria:          categoria,
		Prioridade:         rotularPrioridade(prioridadeBruta),
		Entidade:           TextoSimples(valor(item, campoEntidade, "entities_id")),
		Resolvido:          resolvido,
		AltaPrioridade:     ehAltaPrioridade(prioridadeBruta),

		pendenteEncerramento: pendente && resolvido,
		abertura:             abertura,
		descricao:            descricao,
	}
}

// encerramentoEmbutido tira o desfecho do proprio resultado da busca: a
// solucao formal ou, na falta dela, o ultimo follow-up publico.
//
// pendente = true quando os arrays de follow-up vierem inconsistentes; nesse
// caso o chamado cai no caminho lento, chamado a chamado.
func encerramentoEmbutido(item map[string]any) (texto, origem string, pendente bool) {
	if s := valor(item, campoSolucao, "solution"); strings.TrimSpace(s) != "" {
		return s, OrigemSolucao, false
	}

	conteudos := listaDeValores(item, campoFollowTexto)
	if len(conteudos) == 0 {
		// Nem solucao nem follow-up na busca. Pode ser um chamado sem registro
		// mesmo, ou um GLPI que nao preenche esses campos na busca -- so a
		// consulta individual distingue os dois casos.
		return "", OrigemSemRegistro, true
	}

	privados := listaDeValores(item, campoFollowPrivado)
	datas := listaDeValores(item, campoFollowData)
	if len(privados) != len(conteudos) {
		// Sem saber quais sao privados, nao da para escolher com seguranca.
		return "", OrigemFollowup, true
	}

	melhor, melhorData := "", ""
	for i, conteudo := range conteudos {
		if privados[i] == "1" || strings.TrimSpace(conteudo) == "" {
			continue
		}
		data := ""
		if i < len(datas) {
			data = datas[i]
		}
		// Sem data utilizavel, a ordem do array ja e cronologica.
		if melhor == "" || data == "" || data >= melhorData {
			melhor, melhorData = conteudo, data
		}
	}
	if melhor == "" {
		// Havia follow-ups, mas todos privados ou vazios: a consulta
		// individual chegaria na mesma conclusao.
		return "", OrigemSemRegistro, false
	}
	return melhor, OrigemFollowup, false
}

// listaDeValores le um campo que pode vir como valor unico ou array.
func listaDeValores(item map[string]any, chave string) []string {
	bruto, ok := item[chave]
	if !ok || bruto == nil {
		return nil
	}
	if lista, ok := bruto.([]any); ok {
		valores := make([]string, 0, len(lista))
		for _, v := range lista {
			valores = append(valores, strings.TrimSpace(textoDe(v)))
		}
		return valores
	}
	if s := strings.TrimSpace(textoDe(bruto)); s != "" {
		return []string{s}
	}
	return nil
}

// preencherSolucoes completa o motivo de encerramento dos chamados resolvidos
// em que a busca nao trouxe texto.
//
// Na pratica a maioria dos chamados e encerrada sem preencher a solucao
// formal: o desfecho fica no ultimo follow-up ("Normalizado.", "Acessos
// modificados..."), que e o "follow que entra na categoria de solucao" citado
// no SDD. Por isso a ordem e solucao -> ultimo follow-up publico.
func (c *Client) preencherSolucoes(ctx context.Context, chamados []Chamado) {
	var pendentes []int
	for i, ch := range chamados {
		if ch.pendenteEncerramento && ch.ID != "" {
			pendentes = append(pendentes, i)
		}
	}
	if len(pendentes) == 0 {
		return
	}
	c.log.Info("buscando solucao individual", "chamados", len(pendentes))

	fila := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for w := 0; w < c.paralelismo; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range fila {
				texto, origem, err := c.buscarEncerramento(ctx, chamados[i].ID)
				if err != nil {
					c.log.Warn("falha ao buscar encerramento", "chamado", chamados[i].ID, "erro", err)
					continue
				}
				if texto == "" {
					continue
				}
				encerramento, origemFinal := MotivoEncerramento(texto, true, origem)
				mu.Lock()
				chamados[i].MotivoEncerramento = encerramento
				chamados[i].OrigemEncerramento = origemFinal
				mu.Unlock()
			}
		}()
	}

	for _, i := range pendentes {
		select {
		case fila <- i:
		case <-ctx.Done():
			close(fila)
			wg.Wait()
			return
		}
	}
	close(fila)
	wg.Wait()
}

// buscarEncerramento tenta a solucao formal e, se nao houver, o ultimo
// follow-up publico do chamado.
func (c *Client) buscarEncerramento(ctx context.Context, id string) (texto, origem string, err error) {
	solucao, err := c.buscarSolucao(ctx, id)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(solucao) != "" {
		return solucao, OrigemSolucao, nil
	}

	followup, err := c.buscarUltimoFollowup(ctx, id)
	if err != nil {
		return "", "", err
	}
	return followup, OrigemFollowup, nil
}

// buscarUltimoFollowup devolve o follow-up publico mais recente.
//
// Follow-ups privados ficam de fora de proposito: sao notas internas da
// equipe e acabariam num CSV que circula fora dela.
func (c *Client) buscarUltimoFollowup(ctx context.Context, id string) (string, error) {
	corpo, status, err := c.get(ctx, "/Ticket/"+url.PathEscape(id)+"/ITILFollowup", nil)
	if err != nil {
		return "", err
	}
	if status == 204 || len(corpo) == 0 {
		return "", nil
	}

	lista, err := listaDeItens(corpo)
	if err != nil {
		return "", nil
	}

	var melhor string
	var melhorData time.Time
	for _, item := range lista {
		if ehPrivado(item) {
			continue
		}
		conteudo := valor(item, "content")
		if strings.TrimSpace(conteudo) == "" {
			continue
		}
		d := parseData(valor(item, "date_creation", "date_mod", "date"))
		if melhor == "" || !d.Before(melhorData) {
			melhor, melhorData = conteudo, d
		}
	}
	return melhor, nil
}

func ehPrivado(item map[string]any) bool {
	switch valor(item, "is_private") {
	case "1", "true":
		return true
	}
	return false
}

// listaDeItens aceita tanto o array puro quanto o envelope {"data": [...]}.
func listaDeItens(corpo []byte) ([]map[string]any, error) {
	var lista []map[string]any
	if err := json.Unmarshal(corpo, &lista); err == nil {
		return lista, nil
	}
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(corpo, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

func (c *Client) buscarSolucao(ctx context.Context, id string) (string, error) {
	corpo, status, err := c.get(ctx, "/Ticket/"+url.PathEscape(id)+"/ITILSolution", nil)
	if err != nil {
		return "", err
	}
	if status == 204 || len(corpo) == 0 {
		return "", nil
	}

	lista, err := listaDeItens(corpo)
	if err != nil {
		return "", nil
	}

	// Vale a solucao mais recente.
	var melhor string
	var melhorData time.Time
	for _, item := range lista {
		conteudo := valor(item, "content")
		if strings.TrimSpace(conteudo) == "" {
			continue
		}
		d := parseData(valor(item, "date_creation", "date_mod", "date"))
		if melhor == "" || d.After(melhorData) {
			melhor, melhorData = conteudo, d
		}
	}
	return melhor, nil
}

// ── Leitura tolerante do JSON do GLPI ────────────────────────────────────────

// valor devolve a primeira chave preenchida. A API /search usa chaves
// numericas; os endpoints de item usam nomes.
func valor(item map[string]any, chaves ...string) string {
	for _, k := range chaves {
		bruto, ok := item[k]
		if !ok {
			continue
		}
		if s := textoDe(bruto); strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func textoDe(bruto any) string {
	switch v := bruto.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []any:
		var partes []string
		for _, item := range v {
			if s := strings.TrimSpace(textoDe(item)); s != "" {
				partes = append(partes, s)
			}
		}
		return strings.Join(partes, " | ")
	case map[string]any:
		return valor(v, "name", "completename", "content")
	default:
		return fmt.Sprint(v)
	}
}

// limparLista normaliza campos multivalorados ("Fulano <br>Ciclano").
func limparLista(s string) string {
	var limpas []string
	for _, p := range strings.Split(LimparTexto(s), "\n") {
		if p = strings.TrimSpace(p); p != "" {
			limpas = append(limpas, p)
		}
	}
	return strings.Join(limpas, " | ")
}

var formatosData = []string{
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"02-01-2006 15:04:05",
	"02-01-2006 15:04",
	"02-01-2006",
	"02/01/2006 15:04:05",
	"02/01/2006 15:04",
	"02/01/2006",
}

// parseData aceita os formatos que o GLPI devolve conforme a preferencia de
// exibicao do usuario da API.
func parseData(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "0000-00-00") {
		return time.Time{}
	}
	for _, f := range formatosData {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func formatarData(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("02/01/2006")
}

func formatarHora(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04")
}

// formatarAtendimento devolve o tempo decorrido entre a abertura e a solucao
// como HH:MM, sem virar dia: um chamado de tres dias sai como "72:15". E o
// formato que o Excel soma e faz media com a mascara [h]:mm.
//
// Vazio quando o chamado nao foi resolvido -- assim media e soma na planilha
// nao misturam quem terminou com quem ainda esta em aberto. Vazio tambem
// quando a solucao e anterior a abertura, que e dado inconsistente no GLPI e
// nao um atendimento negativo.
func formatarAtendimento(abertura, fim time.Time) string {
	if abertura.IsZero() || fim.IsZero() || fim.Before(abertura) {
		return ""
	}
	d := fim.Sub(abertura)
	return fmt.Sprintf("%02d:%02d", int(d/time.Hour), int(d/time.Minute)%60)
}

var rotulosResolvido = map[string]bool{
	"5": true, "6": true,
	"solucionado": true, "fechado": true, "encerrado": true,
	"solved": true, "closed": true,
}

func statusResolvido(status string) bool {
	return rotulosResolvido[normalizarTexto(status)]
}
