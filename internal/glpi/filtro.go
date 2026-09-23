package glpi

import "strings"

// Valores aceitos no filtro de status.
const (
	StatusTodos      = "todos"
	StatusResolvidos = "resolvidos"
	StatusAbertos    = "abertos"
)

// Selecao sao os filtros aplicados em memoria sobre o resultado do GLPI.
type Selecao struct {
	Status string // todos | resolvidos | abertos
	Busca  string // texto livre
}

// NormalizarStatus aceita apelidos comuns e devolve um dos valores validos.
func NormalizarStatus(s string) string {
	switch normalizarTexto(s) {
	case "resolvidos", "resolvido", "fechados", "fechado", "solucionados":
		return StatusResolvidos
	case "abertos", "aberto", "em aberto", "pendentes":
		return StatusAbertos
	default:
		return StatusTodos
	}
}

// Filtrar aplica status e busca textual. A busca ignora acentos e maiusculas e
// percorre os campos visiveis mais a descricao do chamado.
func Filtrar(chamados []Chamado, s Selecao) []Chamado {
	status := NormalizarStatus(s.Status)
	busca := normalizarTexto(s.Busca)

	filtrados := make([]Chamado, 0, len(chamados))
	for _, c := range chamados {
		if status == StatusResolvidos && !c.Resolvido {
			continue
		}
		if status == StatusAbertos && c.Resolvido {
			continue
		}
		if busca != "" && !c.combina(busca) {
			continue
		}
		filtrados = append(filtrados, c)
	}
	return filtrados
}

func (c Chamado) combina(buscaNorm string) bool {
	campos := []string{
		c.ID, c.Titulo, c.Status, c.MotivoAbertura, c.MotivoEncerramento,
		c.UsuarioRede, c.Requerente, c.Tecnico, c.Categoria, c.Prioridade,
		c.Entidade, c.descricao,
	}
	for _, campo := range campos {
		if campo == "" {
			continue
		}
		if strings.Contains(normalizarTexto(campo), buscaNorm) {
			return true
		}
	}
	return false
}

// Paginar recorta a pagina pedida (1-based) sem estourar os limites.
func Paginar(chamados []Chamado, pagina, tamanho int) []Chamado {
	if tamanho <= 0 {
		return chamados
	}
	if pagina < 1 {
		pagina = 1
	}
	inicio := (pagina - 1) * tamanho
	if inicio >= len(chamados) {
		return nil
	}
	fim := inicio + tamanho
	if fim > len(chamados) {
		fim = len(chamados)
	}
	return chamados[inicio:fim]
}
