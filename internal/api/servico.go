// Package api expoe o extrator como servico HTTP: listagem, cards e CSV.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"extratorglpi/internal/cache"
	"extratorglpi/internal/config"
	"extratorglpi/internal/glpi"
)

// Servico junta cliente do GLPI, cache e configuracao.
type Servico struct {
	cli   *glpi.Client
	cfg   *config.Config
	cache *cache.Cache[[]glpi.Chamado]
	log   *slog.Logger
}

// Novo monta o servico.
func Novo(cli *glpi.Client, cfg *config.Config, log *slog.Logger) *Servico {
	if log == nil {
		log = slog.Default()
	}
	return &Servico{
		cli:   cli,
		cfg:   cfg,
		cache: cache.Novo[[]glpi.Chamado](cfg.CacheTTL),
		log:   log,
	}
}

// ── Filtros ──────────────────────────────────────────────────────────────────

const (
	porPaginaPadrao = 50
	porPaginaMaximo = 500
	formatoDataISO  = "2006-01-02"
)

type filtros struct {
	periodo   glpi.Periodo
	selecao   glpi.Selecao
	pagina    int
	porPagina int
	inicioTxt string
	fimTxt    string
	semCache  bool
}

// erroUsuario marca erros de parametro (HTTP 400), separando-os de falhas do GLPI.
type erroUsuario struct{ msg string }

func (e *erroUsuario) Error() string { return e.msg }

func lerFiltros(r *http.Request) (filtros, error) {
	q := r.URL.Query()
	f, err := lerFiltrosDeTexto(strings.TrimSpace(q.Get("inicio")), strings.TrimSpace(q.Get("fim")))
	if err != nil {
		return f, err
	}

	f.selecao = glpi.Selecao{Status: glpi.NormalizarStatus(q.Get("status")), Busca: strings.TrimSpace(q.Get("q"))}
	f.pagina = inteiro(q.Get("pagina"), 1)
	f.porPagina = inteiro(q.Get("por_pagina"), porPaginaPadrao)
	f.semCache = q.Get("atualizar") == "1"
	if f.porPagina > porPaginaMaximo {
		f.porPagina = porPaginaMaximo
	}
	return f, nil
}

// lerFiltrosDeTexto interpreta o periodo. Fica separado para o aquecimento de
// cache montar exatamente o mesmo periodo que a tela envia.
func lerFiltrosDeTexto(inicioTxt, fimTxt string) (filtros, error) {
	f := filtros{inicioTxt: inicioTxt, fimTxt: fimTxt, pagina: 1, porPagina: porPaginaPadrao}

	if f.inicioTxt != "" {
		d, err := time.ParseInLocation(formatoDataISO, f.inicioTxt, time.Local)
		if err != nil {
			return f, &erroUsuario{"data inicial invalida: use AAAA-MM-DD"}
		}
		f.periodo.Inicio = d
	}
	if f.fimTxt != "" {
		d, err := time.ParseInLocation(formatoDataISO, f.fimTxt, time.Local)
		if err != nil {
			return f, &erroUsuario{"data final invalida: use AAAA-MM-DD"}
		}
		f.periodo.Fim = d.Add(24*time.Hour - time.Second)
	}
	if !f.periodo.Inicio.IsZero() && !f.periodo.Fim.IsZero() && f.periodo.Fim.Before(f.periodo.Inicio) {
		return f, &erroUsuario{"a data final e anterior a data inicial"}
	}
	return f, nil
}

func inteiro(s string, padrao int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return padrao
	}
	return n
}

// chaveCache identifica o periodo consultado; status e busca sao aplicados
// depois, sobre o mesmo conjunto em cache.
func (f filtros) chaveCache() string {
	return fmt.Sprintf("%s..%s", f.periodo.Inicio.Format(time.RFC3339), f.periodo.Fim.Format(time.RFC3339))
}

// carregar busca os chamados do periodo, usando o cache quando possivel.
func (s *Servico) carregar(ctx context.Context, f filtros) ([]glpi.Chamado, error) {
	buscar := func(ctx context.Context) ([]glpi.Chamado, error) {
		inicio := time.Now()
		chamados, err := s.cli.BuscarChamados(ctx, f.periodo, glpi.Rotulos{
			Motivo:  s.cfg.MotivoLabels,
			Usuario: s.cfg.UsuarioLabels,
		})
		if err != nil {
			return nil, err
		}
		s.log.Info("busca concluida no GLPI",
			"chamados", len(chamados),
			"periodo", f.chaveCache(),
			"duracao", time.Since(inicio).Round(time.Millisecond),
		)
		return chamados, nil
	}

	if f.semCache {
		s.cache.Limpar()
	}
	return s.cache.Obter(ctx, f.chaveCache(), buscar)
}

// ── Respostas ────────────────────────────────────────────────────────────────

func responderJSON(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(corpo); err != nil {
		slog.Default().Error("falha ao escrever resposta", "erro", err)
	}
}

// responderErro traduz o erro para um status HTTP e uma mensagem em portugues.
// Falha de credencial ou GLPI fora do ar sao 502: o problema e no upstream.
func (s *Servico) responderErro(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadGateway
	msg := err.Error()

	var eu *erroUsuario
	var eg *glpi.Erro
	switch {
	case comoErro(err, &eu):
		status = http.StatusBadRequest
	case comoErro(err, &eg):
		if eg.Status == http.StatusUnauthorized || eg.Status == http.StatusForbidden {
			msg = "o GLPI recusou as credenciais: confira GLPI_USUARIO, GLPI_SENHA e GLPI_APP_TOKEN no .env"
		}
	case r.Context().Err() != nil:
		status = http.StatusRequestTimeout
		msg = "a consulta ao GLPI passou do tempo limite"
	}

	s.log.Error("requisicao falhou", "rota", r.URL.Path, "status", status, "erro", err)
	responderJSON(w, status, map[string]string{"erro": msg})
}

// comoErro e um errors.As simplificado para os dois tipos usados aqui.
func comoErro[T error](err error, alvo *T) bool {
	for err != nil {
		if e, ok := err.(T); ok {
			*alvo = e
			return true
		}
		desembrulhar, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = desembrulhar.Unwrap()
	}
	return false
}
