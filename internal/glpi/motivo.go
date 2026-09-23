package glpi

import (
	"regexp"
	"strings"
)

// Os chamados do GLPI nascem de formularios cujo corpo e uma lista numerada:
//
//	DADOS DO CHAMADO
//	1) LOCALIZACAO : MOTIVA ASIA
//	...
//	9) SOLICITACAO : ERRO NA FERRAMENTA
//
// reCampoForm localiza o inicio de cada item ("9)"), e o valor vai dali ate o
// proximo item -- valores de multiplas linhas (horarios, observacoes) sao comuns.
var reCampoForm = regexp.MustCompile(`(?m)(?:^|\s)(\d{1,2})\s*\)\s*`)

// CampoForm e um par rotulo/valor extraido do formulario.
type CampoForm struct {
	Rotulo     string // como aparece no chamado
	RotuloNorm string // minusculo, sem acentos
	Valor      string
}

// ExtrairCamposForm devolve os campos do formulario na ordem em que aparecem.
func ExtrairCamposForm(descricao string) []CampoForm {
	marcas := reCampoForm.FindAllStringIndex(descricao, -1)
	if len(marcas) == 0 {
		return nil
	}

	campos := make([]CampoForm, 0, len(marcas))
	for i, m := range marcas {
		fim := len(descricao)
		if i+1 < len(marcas) {
			fim = marcas[i+1][0]
		}
		trecho := descricao[m[1]:fim]

		sep := strings.Index(trecho, ":")
		if sep < 0 {
			continue
		}
		rotulo := strings.TrimSpace(trecho[:sep])
		valor := strings.TrimSpace(trecho[sep+1:])
		if rotulo == "" {
			continue
		}
		// Um rotulo real e curto; textos longos sem ":" proprio nao sao campo.
		if len(rotulo) > 60 || strings.Contains(rotulo, "\n") {
			continue
		}
		campos = append(campos, CampoForm{
			Rotulo:     rotulo,
			RotuloNorm: normalizarTexto(rotulo),
			Valor:      strings.Join(strings.Fields(valor), " "),
		})
	}
	return campos
}

// Origens possiveis do motivo de abertura.
const (
	OrigemFormulario = "formulario"
	OrigemCategoria  = "categoria"
	OrigemTitulo     = "titulo"
	OrigemIndefinida = "indefinido"
)

// MotivoAbertura escolhe o motivo de abertura do chamado.
//
// Prioridade: primeiro rotulo do formulario presente em labels (na ordem de
// labels, nao na ordem do formulario); se nada casar, cai para a categoria do
// GLPI e, por ultimo, para o titulo.
func MotivoAbertura(descricao, categoria, titulo string, labels []string) (motivo, origem string) {
	campos := ExtrairCamposForm(descricao)

	porRotulo := make(map[string]string, len(campos))
	for _, c := range campos {
		if c.Valor == "" {
			continue
		}
		if _, existe := porRotulo[c.RotuloNorm]; !existe {
			porRotulo[c.RotuloNorm] = c.Valor
		}
	}

	for _, label := range labels {
		if v, ok := porRotulo[normalizarTexto(label)]; ok {
			return v, OrigemFormulario
		}
	}

	if v := strings.TrimSpace(categoria); v != "" {
		return v, OrigemCategoria
	}
	if v := strings.TrimSpace(titulo); v != "" {
		return v, OrigemTitulo
	}
	return "", OrigemIndefinida
}

// MotivoEncerramento devolve o texto de encerramento do chamado e de onde ele
// veio, ou a marcacao exigida pelo SDD quando o chamado ainda nao foi
// encerrado.
//
// Chamado encerrado sem nenhum texto recebe marcacao propria: dizer
// "Chamado em aberto" nesse caso seria falso.
func MotivoEncerramento(texto string, resolvido bool, origem string) (string, string) {
	if !resolvido {
		return MotivoEmAberto, OrigemEmAberto
	}
	s := strings.Join(strings.Fields(LimparTexto(texto)), " ")
	if s == "" {
		return MotivoSemDescricao, OrigemSemRegistro
	}
	return s, origem
}
