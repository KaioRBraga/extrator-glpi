package glpi

import "testing"

func amostra() []Chamado {
	return []Chamado{
		{ID: "1", Titulo: "Aplicativos", MotivoAbertura: "ERRO NA FERRAMENTA", Resolvido: true, descricao: "NETSALES fora do ar"},
		{ID: "2", Titulo: "Infraestrutura", MotivoAbertura: "ESTRUTURA DE REDE", Resolvido: false, descricao: "cabo de rede"},
		{ID: "3", Titulo: "Equipamento", MotivoAbertura: "BOTAO MUTE EMPERRADO", Resolvido: true, Requerente: "Ana Carolina"},
	}
}

func TestFiltrarStatus(t *testing.T) {
	casos := map[string]int{
		StatusTodos:      3,
		StatusResolvidos: 2,
		StatusAbertos:    1,
		"fechados":       2, // apelido
		"":               3, // vazio = todos
	}
	for status, esperado := range casos {
		if got := len(Filtrar(amostra(), Selecao{Status: status})); got != esperado {
			t.Errorf("status %q devolveu %d, esperado %d", status, got, esperado)
		}
	}
}

func TestFiltrarBusca(t *testing.T) {
	casos := []struct {
		busca    string
		esperado []string
	}{
		{"netsales", []string{"1"}},     // descricao
		{"REDE", []string{"2"}},         // motivo e descricao
		{"ana carolina", []string{"3"}}, // requerente
		{"aplicativos", []string{"1"}},  // titulo
		{"inexistente", nil},            //
		{"botao", []string{"3"}},        // sem acento no dado
		{"BOTÃO", []string{"3"}},        // com acento na busca
	}
	for _, c := range casos {
		var ids []string
		for _, ch := range Filtrar(amostra(), Selecao{Busca: c.busca}) {
			ids = append(ids, ch.ID)
		}
		if len(ids) != len(c.esperado) {
			t.Errorf("busca %q devolveu %v, esperado %v", c.busca, ids, c.esperado)
			continue
		}
		for i := range ids {
			if ids[i] != c.esperado[i] {
				t.Errorf("busca %q devolveu %v, esperado %v", c.busca, ids, c.esperado)
				break
			}
		}
	}
}

func TestFiltrarCombinado(t *testing.T) {
	r := Filtrar(amostra(), Selecao{Status: StatusResolvidos, Busca: "e"})
	if len(r) != 2 {
		t.Fatalf("esperava 2 chamados, veio %d", len(r))
	}
	stats := CalcularStats(3, r)
	if stats.Total != 3 || stats.Filtrados != 2 || stats.PercentualResolvidos != 100 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestPaginar(t *testing.T) {
	c := amostra()
	if got := Paginar(c, 1, 2); len(got) != 2 || got[0].ID != "1" {
		t.Errorf("pagina 1 = %+v", got)
	}
	if got := Paginar(c, 2, 2); len(got) != 1 || got[0].ID != "3" {
		t.Errorf("pagina 2 = %+v", got)
	}
	if got := Paginar(c, 9, 2); got != nil {
		t.Errorf("pagina fora do intervalo = %+v", got)
	}
	if got := Paginar(c, 0, 0); len(got) != 3 {
		t.Errorf("sem paginacao = %+v", got)
	}
}
