package glpi

import "testing"

// Fixtures modeladas nos chamados reais exportados do GLPI (nomes e logins
// trocados). A estrutura -- "DADOS DO CHAMADO" seguido de itens numerados --
// e a mesma dos chamados 850214, 849237 e 850080.
const (
	formAplicativos = `DADOS DO CHAMADO

APLICATIVOS E SISTEMAS - INTERNOS E EXTERNOS

1) LOCALIZAÇÃO : MOTIVA ASIA
2) DEPARTAMENTO : OPERAÇÃO
3) CAMPANHA : CLARO MPLAY
4) HORÁRIO EXPEDIENTE :
08:40 - 15:00
5) NÚMERO DE PA : 3114
6) NOME : FULANO DE TAL
7) USUÁRIO DE REDE : f_detal
8) ATENDIMENTO : APLICATIVOS
9) SOLICITAÇÃO : ERRO NA FERRAMENTA
10) APLICATIVO : NETSALES
11) PRINT DO ERRO : Documento anexado
12) OBSERVAÇÃO : Erro na checagem de crédito`

	formInfraestrutura = `DADOS DO CHAMADO

INFRAESTRUTURA

1) LOCALIZAÇÃO : MOTIVA HOME OFFICE
2) DEPARTAMENTO : OPERAÇÃO
3) CAMPANHA : CLARO MPLAY
4) NÚMERO DE PA : 0
5) USUÁRIO DE REDE : f_detal
6) SOLICITAÇÃO : ESTRUTURA DE REDE
7) PROBLEMA : CABEAMENTO NOVO
8) LOCAL :
Operadora necessita de um cabo de rede maior, no tamanho de 15 metros.`

	formEquipamento = `DADOS DO CHAMADO

PROBLEMAS COM EQUIPAMENTOS

1) LOCALIZAÇÃO : MOTIVA ASIA
2) DEPARTAMENTO : OPERAÇÃO
3) CAMPANHA : TELEVENDAS AGIL
4) NÚMERO DE PA : 3141
5) USUÁRIO DE REDE : f_detal
6) EQUIPAMENTO : HEADSET
7) PROBLEMA : BOTAO MUTE EMPERRADO`
)

var labelsPadrao = []string{"SOLICITACAO", "PROBLEMA", "ATENDIMENTO", "MOTIVO"}

var usuarioLabelsPadrao = []string{"USUARIO DE REDE", "LOGIN DE REDE", "USUARIO", "LOGIN", "USER"}

var rotulosPadrao = Rotulos{Motivo: labelsPadrao, Usuario: usuarioLabelsPadrao}

func TestMotivoAbertura(t *testing.T) {
	casos := []struct {
		nome      string
		descricao string
		categoria string
		titulo    string
		labels    []string
		motivo    string
		origem    string
	}{
		{
			nome:      "solicitacao do formulario vence a categoria",
			descricao: formAplicativos,
			categoria: "Aplicativos",
			titulo:    "Aplicativos e Sistemas Internos e Externos",
			labels:    labelsPadrao,
			motivo:    "ERRO NA FERRAMENTA",
			origem:    OrigemFormulario,
		},
		{
			nome:      "com solicitacao e problema vence a ordem de labels",
			descricao: formInfraestrutura,
			titulo:    "Infraestrutura",
			labels:    labelsPadrao,
			motivo:    "ESTRUTURA DE REDE",
			origem:    OrigemFormulario,
		},
		{
			nome:      "sem solicitacao cai para problema",
			descricao: formEquipamento,
			titulo:    "Problemas com Equipamentos",
			labels:    labelsPadrao,
			motivo:    "BOTAO MUTE EMPERRADO",
			origem:    OrigemFormulario,
		},
		{
			nome:      "ordem de labels e respeitada mesmo invertida no formulario",
			descricao: formInfraestrutura,
			labels:    []string{"PROBLEMA", "SOLICITACAO"},
			motivo:    "CABEAMENTO NOVO",
			origem:    OrigemFormulario,
		},
		{
			nome:      "rotulo com acento casa com label sem acento",
			descricao: "1) SOLICITAÇÃO : RESET DE SENHA",
			labels:    []string{"solicitacao"},
			motivo:    "RESET DE SENHA",
			origem:    OrigemFormulario,
		},
		{
			nome:      "sem formulario usa a categoria",
			descricao: "Bom dia, meu computador nao liga.",
			categoria: "Hardware > Desktop",
			titulo:    "Computador nao liga",
			labels:    labelsPadrao,
			motivo:    "Hardware > Desktop",
			origem:    OrigemCategoria,
		},
		{
			nome:      "sem formulario e sem categoria usa o titulo",
			descricao: "Bom dia, meu computador nao liga.",
			titulo:    "Computador nao liga",
			labels:    labelsPadrao,
			motivo:    "Computador nao liga",
			origem:    OrigemTitulo,
		},
		{
			nome:   "sem nada devolve indefinido",
			labels: labelsPadrao,
			origem: OrigemIndefinida,
		},
		{
			nome:      "campo vazio nao e considerado preenchido",
			descricao: "1) SOLICITACAO :\n2) PROBLEMA : MOUSE QUEBRADO",
			labels:    labelsPadrao,
			motivo:    "MOUSE QUEBRADO",
			origem:    OrigemFormulario,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			motivo, origem := MotivoAbertura(c.descricao, c.categoria, c.titulo, c.labels)
			if motivo != c.motivo {
				t.Errorf("motivo = %q, esperado %q", motivo, c.motivo)
			}
			if origem != c.origem {
				t.Errorf("origem = %q, esperado %q", origem, c.origem)
			}
		})
	}
}

// O valor de um campo pode ocupar varias linhas (horario, local). O parser nao
// pode deixar esse texto vazar para o campo seguinte.
func TestExtrairCamposFormValorMultilinha(t *testing.T) {
	campos := ExtrairCamposForm(formAplicativos)
	if len(campos) != 12 {
		t.Fatalf("esperava 12 campos, veio %d: %+v", len(campos), campos)
	}
	if got := campos[3]; got.RotuloNorm != "horario expediente" || got.Valor != "08:40 - 15:00" {
		t.Errorf("campo 4 = %+v", got)
	}
	if got := campos[4]; got.RotuloNorm != "numero de pa" || got.Valor != "3114" {
		t.Errorf("campo 5 = %+v", got)
	}
}

// A descricao chega do GLPI como HTML, as vezes com entidades codificadas
// duas vezes.
func TestMotivoAberturaComHTMLCodificado(t *testing.T) {
	bruto := "&#60;p&#62;DADOS DO CHAMADO&#60;/p&#62;&#60;p&#62;1) SOLICITA&#199;&#195;O : TROCA DE MONITOR&#60;/p&#62;"
	motivo, origem := MotivoAbertura(LimparTexto(bruto), "", "", labelsPadrao)
	if motivo != "TROCA DE MONITOR" || origem != OrigemFormulario {
		t.Errorf("motivo = %q (%s), esperado %q", motivo, origem, "TROCA DE MONITOR")
	}
}

func TestMotivoEncerramento(t *testing.T) {
	casos := []struct {
		nome        string
		texto       string
		resolvido   bool
		origem      string
		esperado    string
		origemFinal string
	}{
		{"chamado em aberto", "", false, OrigemSolucao, MotivoEmAberto, OrigemEmAberto},
		{"aberto ignora texto residual", "<p>rascunho</p>", false, OrigemSolucao, MotivoEmAberto, OrigemEmAberto},
		{"resolvido com html", "<p>Cabo&nbsp;substituido&#60;br&#62;pelo tecnico</p>", true, OrigemSolucao, "Cabo substituido pelo tecnico", OrigemSolucao},
		{"resolvido via follow-up", "Normalizado.", true, OrigemFollowup, "Normalizado.", OrigemFollowup},
		{"resolvido sem texto", "   ", true, OrigemFollowup, MotivoSemDescricao, OrigemSemRegistro},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, origem := MotivoEncerramento(c.texto, c.resolvido, c.origem)
			if got != c.esperado {
				t.Errorf("texto = %q, esperado %q", got, c.esperado)
			}
			if origem != c.origemFinal {
				t.Errorf("origem = %q, esperada %q", origem, c.origemFinal)
			}
		})
	}
}

// O GLPI devolve a prioridade como numero mesmo com expand_dropdowns, e manda
// rotulos de dropdown com entidades HTML. Ambos apareceram em dados reais.
func TestRotularPrioridade(t *testing.T) {
	casos := map[string]string{
		"1": "Muito baixa",
		"2": "Baixa",
		"3": "Media",
		"4": "Alta",
		"5": "Muito alta",
		"6": "Critica",
		"":  "",
		// Se algum dia vier ja rotulado, passa direto.
		"Muito alta": "Muito alta",
	}
	for entrada, esperado := range casos {
		if got := rotularPrioridade(entrada); got != esperado {
			t.Errorf("rotularPrioridade(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}

func TestEhAltaPrioridadeComCodigoNumerico(t *testing.T) {
	for _, baixa := range []string{"1", "2", "3", ""} {
		if ehAltaPrioridade(baixa) {
			t.Errorf("prioridade %q nao deveria contar como alta", baixa)
		}
	}
	for _, alta := range []string{"4", "5", "6", "Alta", "Muito alta"} {
		if !ehAltaPrioridade(alta) {
			t.Errorf("prioridade %q deveria contar como alta", alta)
		}
	}
}

func TestTextoSimples(t *testing.T) {
	casos := map[string]string{
		"Sistemas &gt; Internos e Externos": "Sistemas > Internos e Externos",
		"Telefonia > Entrega de Chamadas":   "Telefonia > Entrega de Chamadas",
		"Root &gt; Motiva &gt; Asia":        "Root > Motiva > Asia",
		"Seguran&#231;a":                    "Segurança",
		"  espacos   demais  ":              "espacos demais",
		// Diferente de LimparTexto, nao remove o que parece tag.
		"&lt;sem categoria&gt;": "<sem categoria>",
	}
	for entrada, esperado := range casos {
		if got := TextoSimples(entrada); got != esperado {
			t.Errorf("TextoSimples(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}
