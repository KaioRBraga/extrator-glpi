package glpi

import "testing"

func TestUsuarioRede(t *testing.T) {
	casos := []struct {
		nome      string
		descricao string
		esperado  string
	}{
		{"campo do formulario", formAplicativos, "f_detal"},
		{"formulario sem numeracao", "USUARIO DE REDE : j_regmon", "j_regmon"},
		{"rotulo solto no texto", "Bom dia, o Login de rede: joao.silva esta bloqueado.", "joao.silva"},
		{"rotulo com acento", "1) USUÁRIO DE REDE : m_lima", "m_lima"},
		{"login com nome junto", "1) USUARIO DE REDE : j_regmon (Joao Regis)", "j_regmon"},
		{"login seguido de traco", "1) LOGIN DE REDE : a_silva - Operacao", "a_silva"},
		{"sem nada", "Chamado sem formulario nem login.", ""},
		{"campo vazio cai para o proximo rotulo", "1) USUARIO DE REDE :\n2) LOGIN : b_souza", "b_souza"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := UsuarioRede(c.descricao, usuarioLabelsPadrao); got != c.esperado {
				t.Errorf("= %q, esperado %q", got, c.esperado)
			}
		})
	}
}

// A ordem dos rotulos manda: um formulario pode ter varios campos de usuario.
func TestUsuarioRedeRespeitaOrdemDosRotulos(t *testing.T) {
	desc := "1) USUARIO : chefe.silva\n2) USUARIO DE REDE : alvo.souza"
	if got := UsuarioRede(desc, usuarioLabelsPadrao); got != "alvo.souza" {
		t.Errorf("= %q, esperado alvo.souza", got)
	}
	if got := UsuarioRede(desc, []string{"USUARIO"}); got != "chefe.silva" {
		t.Errorf("com rotulo unico = %q, esperado chefe.silva", got)
	}
}
