package api

import (
	"context"
	"time"

	"extratorglpi/internal/glpi"
)

// Aquecer mantem o mes corrente pronto no cache.
//
// A primeira consulta de um periodo custa uma varredura inteira no GLPI (~23s
// para um mes). Como a tela abre justamente no mes corrente, sem aquecimento
// todo mundo que abre depois do cache vencer paga essa espera. Aqui o serviço
// paga por todos, em segundo plano.
//
// O custo para o GLPI e fixo e pequeno: uma varredura a cada ciclo, nao uma
// por pessoa que abre a pagina.
func (s *Servico) Aquecer(ctx context.Context) {
	if !s.cfg.AquecerCache || s.cfg.CacheTTL <= 0 {
		s.log.Info("aquecimento de cache desligado")
		return
	}

	// Um pouco antes do cache vencer, para nao existir janela fria.
	intervalo := s.cfg.CacheTTL * 4 / 5
	if intervalo < time.Minute {
		intervalo = time.Minute
	}
	s.log.Info("aquecimento de cache ligado", "intervalo", intervalo.String())

	for {
		s.aquecerMesCorrente(ctx)

		select {
		case <-time.After(intervalo):
		case <-ctx.Done():
			return
		}
	}
}

func (s *Servico) aquecerMesCorrente(ctx context.Context) {
	f := filtrosDoMesCorrente(time.Now())

	inicio := time.Now()
	chamados, err := s.carregar(ctx, f)
	if err != nil {
		// Falha aqui nao afeta ninguem: a consulta do usuario tenta de novo.
		s.log.Warn("nao consegui aquecer o cache", "erro", err)
		return
	}
	s.log.Info("cache aquecido",
		"periodo", f.inicioTxt+".."+f.fimTxt,
		"chamados", len(chamados),
		"duracao", time.Since(inicio).Round(time.Millisecond),
	)
}

// filtrosDoMesCorrente monta exatamente o mesmo periodo que a tela pede ao
// abrir -- se divergir, o aquecimento cai numa chave de cache diferente e nao
// serve para nada.
func filtrosDoMesCorrente(agora time.Time) filtros {
	primeiro := time.Date(agora.Year(), agora.Month(), 1, 0, 0, 0, 0, agora.Location())
	fim := time.Date(agora.Year(), agora.Month(), agora.Day(), 23, 59, 59, 0, agora.Location())

	return filtros{
		periodo:   glpi.Periodo{Inicio: primeiro, Fim: fim},
		selecao:   glpi.Selecao{Status: glpi.StatusTodos},
		pagina:    1,
		porPagina: porPaginaPadrao,
		inicioTxt: primeiro.Format(formatoDataISO),
		fimTxt:    agora.Format(formatoDataISO),
	}
}
