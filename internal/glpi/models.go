package glpi

import (
	"strconv"
	"strings"
	"time"
)

// Chamado e um ticket do GLPI ja normalizado para exibicao e exportacao.
type Chamado struct {
	ID                 string `json:"id"`
	Titulo             string `json:"titulo"`
	Status             string `json:"status"`
	DataAbertura       string `json:"data_abertura"`       // DD/MM/AAAA HH:MM
	DataSolucao        string `json:"data_solucao"`        // vazio se em aberto
	MotivoAbertura     string `json:"motivo_abertura"`     //
	OrigemMotivo       string `json:"origem_motivo"`       // formulario | categoria | titulo
	MotivoEncerramento string `json:"motivo_encerramento"` // "Chamado em aberto" se nao resolvido
	OrigemEncerramento string `json:"origem_encerramento"` // solucao | followup | em aberto | sem registro
	UsuarioRede        string `json:"usuario_rede"`        // login alvo do formulario
	Requerente         string `json:"requerente"`
	Tecnico            string `json:"tecnico"`
	Categoria          string `json:"categoria"`
	Prioridade         string `json:"prioridade"`
	Entidade           string `json:"entidade"`

	// Campos de apoio, nao exportados para o CSV.
	Resolvido      bool      `json:"resolvido"`
	AltaPrioridade bool      `json:"alta_prioridade"`
	abertura       time.Time // usado para o filtro de periodo local
	descricao      string    // texto limpo do formulario

	// pendenteEncerramento marca o chamado que precisa do caminho lento
	// (uma consulta so dele) porque a busca nao trouxe follow-up utilizavel.
	pendenteEncerramento bool
}

// Marcacoes da coluna "Motivo de encerramento".
const (
	// MotivoEmAberto e o texto exigido pelo SDD para chamados nao encerrados.
	MotivoEmAberto = "Chamado em aberto"
	// MotivoSemDescricao cobre o chamado encerrado que nao registrou nem
	// solucao nem follow-up.
	MotivoSemDescricao = "Encerrado sem descricao de solucao"
)

// De onde saiu o texto da coluna "Motivo de encerramento".
const (
	OrigemSolucao     = "solucao"
	OrigemFollowup    = "followup"
	OrigemEmAberto    = "em aberto"
	OrigemSemRegistro = "sem registro"
)

// Stats alimenta os cards de "Estatisticas rapidas".
type Stats struct {
	Total                int     `json:"total"`                 // chamados do periodo, antes dos filtros locais
	Filtrados            int     `json:"filtrados"`             // apos status + busca
	Fechados             int     `json:"fechados"`              // resolvidos entre os filtrados
	Abertos              int     `json:"abertos"`               // nao resolvidos entre os filtrados
	AltaPrioridade       int     `json:"alta_prioridade"`       //
	PercentualResolvidos float64 `json:"percentual_resolvidos"` // 0-100, sobre os filtrados
}

// CalcularStats monta os indicadores a partir do conjunto filtrado.
func CalcularStats(total int, filtrados []Chamado) Stats {
	s := Stats{Total: total, Filtrados: len(filtrados)}
	for _, c := range filtrados {
		if c.Resolvido {
			s.Fechados++
		}
		if c.AltaPrioridade {
			s.AltaPrioridade++
		}
	}
	s.Abertos = s.Filtrados - s.Fechados
	if s.Filtrados > 0 {
		s.PercentualResolvidos = arredondar(float64(s.Fechados)*100/float64(s.Filtrados), 1)
	}
	return s
}

func arredondar(v float64, casas int) float64 {
	f, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', casas, 64), 64)
	return f
}

// rotulosAltaPrioridade cobre os valores que o GLPI devolve com
// expand_dropdowns ligado, em pt-BR e en.
var rotulosAltaPrioridade = map[string]bool{
	"alta":       true,
	"muito alta": true,
	"altissima":  true,
	"major":      true,
	"critica":    true,
	"high":       true,
	"very high":  true,
	"critical":   true,
}

// ehAltaPrioridade aceita tanto o codigo numerico (1-6) quanto o rotulo.
func ehAltaPrioridade(valor string) bool {
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return false
	}
	if n, err := strconv.Atoi(valor); err == nil {
		return n >= 4
	}
	return rotulosAltaPrioridade[normalizarTexto(valor)]
}

// rotulosStatus traduz o codigo numerico do GLPI. Com expand_dropdowns o
// proprio GLPI ja manda o rotulo; este mapa cobre o caso contrario.
var rotulosStatus = map[string]string{
	"1": "Novo",
	"2": "Em atendimento (atribuido)",
	"3": "Em atendimento (planejado)",
	"4": "Pendente",
	"5": "Solucionado",
	"6": "Fechado",
}

// rotulosPrioridade traduz o codigo numerico. Diferente de status e
// categoria, o GLPI devolve a prioridade como numero mesmo com
// expand_dropdowns ligado.
var rotulosPrioridade = map[string]string{
	"1": "Muito baixa",
	"2": "Baixa",
	"3": "Media",
	"4": "Alta",
	"5": "Muito alta",
	"6": "Critica",
}

func rotularPrioridade(valor string) string {
	valor = strings.TrimSpace(valor)
	if r, ok := rotulosPrioridade[valor]; ok {
		return r
	}
	return valor
}

func rotularStatus(valor string) string {
	valor = strings.TrimSpace(valor)
	if r, ok := rotulosStatus[valor]; ok {
		return r
	}
	return valor
}
