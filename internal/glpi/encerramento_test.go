package glpi

import "testing"

// A busca devolve os follow-ups como arrays paralelos (conteudo, privado,
// data). Escolher o certo dali evita uma ida ao GLPI por chamado.
func TestEncerramentoEmbutido(t *testing.T) {
	casos := []struct {
		nome     string
		item     map[string]any
		texto    string
		origem   string
		pendente bool
	}{
		{
			nome:   "solucao formal vence o follow-up",
			item:   map[string]any{"24": "<p>Configurado head OK</p>", "25": []any{"depois disso"}, "91": []any{"0"}},
			texto:  "<p>Configurado head OK</p>",
			origem: OrigemSolucao,
		},
		{
			nome: "ultimo follow-up publico",
			item: map[string]any{
				"25": []any{"@S.I, favor verificar.", "Liberado", "Normalizado."},
				"91": []any{"0", "0", "0"},
				"36": []any{"2026-09-22 08:00:00", "2026-09-22 09:00:00", "2026-09-22 10:00:00"},
			},
			texto:  "Normalizado.",
			origem: OrigemFollowup,
		},
		{
			nome: "follow-up privado nao entra",
			item: map[string]any{
				"25": []any{"Normalizado.", "senha temporaria Ab#12345"},
				"91": []any{"0", "1"},
				"36": []any{"2026-09-22 10:00:00", "2026-09-22 11:00:00"},
			},
			texto:  "Normalizado.",
			origem: OrigemFollowup,
		},
		{
			nome:   "follow-up unico vem como valor solto",
			item:   map[string]any{"25": "Liberado", "91": "0", "36": "2026-09-22 20:42:07"},
			texto:  "Liberado",
			origem: OrigemFollowup,
		},
		{
			nome: "fora de ordem: vence a data mais recente",
			item: map[string]any{
				"25": []any{"Normalizado.", "Validando"},
				"91": []any{"0", "0"},
				"36": []any{"2026-09-22 15:00:00", "2026-09-22 09:00:00"},
			},
			texto:  "Normalizado.",
			origem: OrigemFollowup,
		},
		{
			nome:     "arrays desalinhados caem no caminho lento",
			item:     map[string]any{"25": []any{"a", "b", "c"}, "91": []any{"0", "0"}},
			texto:    "",
			origem:   OrigemFollowup,
			pendente: true,
		},
		{
			nome:     "sem solucao e sem follow-up: confirma individualmente",
			item:     map[string]any{"2": 992728},
			texto:    "",
			origem:   OrigemSemRegistro,
			pendente: true,
		},
		{
			nome:   "todos privados: nao adianta consultar de novo",
			item:   map[string]any{"25": []any{"nota interna"}, "91": []any{"1"}},
			texto:  "",
			origem: OrigemSemRegistro,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			texto, origem, pendente := encerramentoEmbutido(c.item)
			if texto != c.texto {
				t.Errorf("texto = %q, esperado %q", texto, c.texto)
			}
			if origem != c.origem {
				t.Errorf("origem = %q, esperada %q", origem, c.origem)
			}
			if pendente != c.pendente {
				t.Errorf("pendente = %v, esperado %v", pendente, c.pendente)
			}
		})
	}
}

func TestListaDeValores(t *testing.T) {
	item := map[string]any{
		"lista":  []any{"a", "b"},
		"unico":  "so um",
		"nulo":   nil,
		"numero": float64(3),
	}
	if got := listaDeValores(item, "lista"); len(got) != 2 || got[1] != "b" {
		t.Errorf("lista = %v", got)
	}
	if got := listaDeValores(item, "unico"); len(got) != 1 || got[0] != "so um" {
		t.Errorf("unico = %v", got)
	}
	if got := listaDeValores(item, "nulo"); got != nil {
		t.Errorf("nulo = %v", got)
	}
	if got := listaDeValores(item, "ausente"); got != nil {
		t.Errorf("ausente = %v", got)
	}
	if got := listaDeValores(item, "numero"); len(got) != 1 || got[0] != "3" {
		t.Errorf("numero = %v", got)
	}
}
