package glpi

import (
	"testing"
	"time"
)

func data(dia int) time.Time {
	return time.Date(2026, 9, dia, 0, 0, 0, 0, time.Local)
}

func TestFatiarPeriodo(t *testing.T) {
	casos := []struct {
		nome    string
		periodo Periodo
		dias    int
		fatias  int
	}{
		{"sem datas fica inteiro", Periodo{}, 7, 1},
		{"so inicio fica inteiro", Periodo{Inicio: data(1)}, 7, 1},
		{"so fim fica inteiro", Periodo{Fim: data(30)}, 7, 1},
		{"periodo curto nao e cortado", Periodo{Inicio: data(1), Fim: data(5)}, 7, 1},
		{"exatamente uma janela", Periodo{Inicio: data(1), Fim: data(8)}, 7, 1},
		{"um mes vira cinco janelas", Periodo{Inicio: data(1), Fim: data(30)}, 7, 5},
		{"janela diaria", Periodo{Inicio: data(1), Fim: data(5)}, 1, 4},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			fatias := fatiarPeriodo(c.periodo, c.dias)
			if len(fatias) != c.fatias {
				t.Fatalf("%d fatias, esperado %d: %+v", len(fatias), c.fatias, fatias)
			}
		})
	}
}

// As fatias precisam cobrir o periodo inteiro, sem buraco entre elas e sem
// passar das bordas -- senao chamados somem do relatorio.
func TestFatiasCobremOPeriodoSemBuraco(t *testing.T) {
	p := Periodo{Inicio: data(1), Fim: data(30)}
	fatias := fatiarPeriodo(p, 7)

	// Vem da mais recente para a mais antiga.
	if !fatias[0].Fim.Equal(p.Fim) {
		t.Errorf("primeira fatia termina em %v, esperado %v", fatias[0].Fim, p.Fim)
	}
	ultima := fatias[len(fatias)-1]
	if !ultima.Inicio.Equal(p.Inicio) {
		t.Errorf("ultima fatia comeca em %v, esperado %v", ultima.Inicio, p.Inicio)
	}

	for i, f := range fatias {
		if f.Inicio.Before(p.Inicio) || f.Fim.After(p.Fim) {
			t.Errorf("fatia %d (%v..%v) sai do periodo", i, f.Inicio, f.Fim)
		}
		if !f.Inicio.Before(f.Fim) {
			t.Errorf("fatia %d invertida: %v..%v", i, f.Inicio, f.Fim)
		}
		if i == 0 {
			continue
		}
		anterior := fatias[i-1]
		vao := anterior.Inicio.Sub(f.Fim)
		if vao != time.Second {
			t.Errorf("buraco de %v entre a fatia %d e a %d", vao, i, i-1)
		}
	}
}
