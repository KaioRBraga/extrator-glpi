package api

import (
	"net/http"
	"strconv"
	"time"

	"extratorglpi/internal/glpi"
)

type periodoResposta struct {
	Inicio string `json:"inicio"`
	Fim    string `json:"fim"`
}

type respostaChamados struct {
	Dados        []glpi.Chamado  `json:"dados"`
	Stats        glpi.Stats      `json:"stats"`
	Pagina       int             `json:"pagina"`
	PorPagina    int             `json:"por_pagina"`
	TotalPaginas int             `json:"total_paginas"`
	Truncado     bool            `json:"truncado"`
	Periodo      periodoResposta `json:"periodo"`
}

// handleChamados devolve a pagina pedida mais os indicadores dos cards.
//
// Os cards sempre refletem o conjunto filtrado inteiro, nao apenas a pagina.
func (s *Servico) handleChamados(w http.ResponseWriter, r *http.Request) {
	f, err := lerFiltros(r)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	todos, err := s.carregar(r.Context(), f)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	filtrados := glpi.Filtrar(todos, f.selecao)
	pagina := glpi.Paginar(filtrados, f.pagina, f.porPagina)
	if pagina == nil {
		pagina = []glpi.Chamado{}
	}

	totalPaginas := 0
	if f.porPagina > 0 {
		totalPaginas = (len(filtrados) + f.porPagina - 1) / f.porPagina
	}

	responderJSON(w, http.StatusOK, respostaChamados{
		Dados:        pagina,
		Stats:        glpi.CalcularStats(len(todos), filtrados),
		Pagina:       f.pagina,
		PorPagina:    f.porPagina,
		TotalPaginas: totalPaginas,
		Truncado:     s.cfg.MaxChamados > 0 && len(todos) >= s.cfg.MaxChamados,
		Periodo:      periodoResposta{Inicio: f.inicioTxt, Fim: f.fimTxt},
	})
}

// handleExportar devolve o CSV com todos os chamados que passam pelos filtros
// correntes -- sem paginacao.
func (s *Servico) handleExportar(w http.ResponseWriter, r *http.Request) {
	f, err := lerFiltros(r)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}

	todos, err := s.carregar(r.Context(), f)
	if err != nil {
		s.responderErro(w, r, err)
		return
	}
	filtrados := glpi.Filtrar(todos, f.selecao)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nomeArquivo(f)+`"`)
	w.Header().Set("X-Total-Chamados", strconv.Itoa(len(filtrados)))

	if err := EscreverCSV(w, filtrados); err != nil {
		// O cabecalho ja foi enviado; so resta registrar.
		s.log.Error("falha ao gerar CSV", "erro", err)
	}
}

// handleSaude confirma que o servico fala com o GLPI (usado no healthcheck do
// container).
func (s *Servico) handleSaude(w http.ResponseWriter, r *http.Request) {
	corpo := map[string]any{
		"servico": "extratorglpi",
		"glpi":    s.cli.BaseURL(),
	}
	if err := s.cli.Ping(r.Context()); err != nil {
		corpo["status"] = "degradado"
		corpo["erro"] = err.Error()
		responderJSON(w, http.StatusServiceUnavailable, corpo)
		return
	}
	corpo["status"] = "ok"
	responderJSON(w, http.StatusOK, corpo)
}

func nomeArquivo(f filtros) string {
	inicio, fim := f.inicioTxt, f.fimTxt
	if inicio == "" {
		inicio = "inicio"
	}
	if fim == "" {
		fim = time.Now().Format(formatoDataISO)
	}
	sufixo := ""
	switch f.selecao.Status {
	case glpi.StatusResolvidos:
		sufixo = "_resolvidos"
	case glpi.StatusAbertos:
		sufixo = "_em-aberto"
	}
	return "chamados_" + inicio + "_a_" + fim + sufixo + ".csv"
}
