package api

import (
	"testing"
	"time"
)

// O aquecimento so serve se cair na mesma chave de cache que a tela usa ao
// abrir. Divergindo um segundo, o usuario paga a varredura assim mesmo.
func TestFiltrosDoMesCorrenteBatemComOsDaTela(t *testing.T) {
	agora := time.Date(2026, 9, 23, 14, 35, 12, 0, time.Local)
	aquecimento := filtrosDoMesCorrente(agora)

	// O que a tela envia: inicio = dia 1 do mes, fim = hoje.
	daTela, err := lerFiltrosDeTexto("2026-09-01", "2026-09-23")
	if err != nil {
		t.Fatalf("lendo filtros da tela: %v", err)
	}

	if aquecimento.chaveCache() != daTela.chaveCache() {
		t.Errorf("chaves diferentes:\n  aquecimento: %s\n  tela:        %s",
			aquecimento.chaveCache(), daTela.chaveCache())
	}
	if aquecimento.inicioTxt != "2026-09-01" || aquecimento.fimTxt != "2026-09-23" {
		t.Errorf("datas = %q..%q", aquecimento.inicioTxt, aquecimento.fimTxt)
	}
}

func TestFiltrosDoMesCorrenteNoPrimeiroDia(t *testing.T) {
	agora := time.Date(2026, 10, 1, 0, 5, 0, 0, time.Local)
	f := filtrosDoMesCorrente(agora)

	if f.inicioTxt != "2026-10-01" || f.fimTxt != "2026-10-01" {
		t.Errorf("datas = %q..%q", f.inicioTxt, f.fimTxt)
	}
	if f.periodo.Fim.Before(f.periodo.Inicio) {
		t.Errorf("periodo invertido: %v..%v", f.periodo.Inicio, f.periodo.Fim)
	}
}
