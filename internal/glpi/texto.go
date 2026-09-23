package glpi

import (
	"html"
	"regexp"
	"strings"
)

var (
	reQuebras  = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/tr|/h[1-6])\s*/?\s*>`)
	reTags     = regexp.MustCompile(`<[^>]*>`)
	reEspacos  = regexp.MustCompile(`[ \t\x{00a0}]+`)
	reNLinhas  = regexp.MustCompile(`\n{3,}`)
	reEntidade = regexp.MustCompile(`&(#\d+|#x[0-9a-fA-F]+|[a-zA-Z]+);`)
)

// LimparTexto converte o conteudo HTML do GLPI em texto legivel, preservando
// as quebras de linha do formulario.
//
// O GLPI costuma devolver o conteudo com entidades codificadas mais de uma vez
// (&#60;p&#62;), por isso o unescape roda ate estabilizar (no maximo 3 voltas).
func LimparTexto(bruto string) string {
	txt := bruto
	for i := 0; i < 3; i++ {
		if !reEntidade.MatchString(txt) {
			break
		}
		novo := html.UnescapeString(txt)
		if novo == txt {
			break
		}
		txt = novo
	}

	txt = reQuebras.ReplaceAllString(txt, "\n")
	txt = reTags.ReplaceAllString(txt, " ")
	txt = html.UnescapeString(txt)
	txt = strings.ReplaceAll(txt, "\r\n", "\n")
	txt = strings.ReplaceAll(txt, "\r", "\n")
	txt = reEspacos.ReplaceAllString(txt, " ")

	var linhas []string
	for _, l := range strings.Split(txt, "\n") {
		linhas = append(linhas, strings.TrimSpace(l))
	}
	txt = strings.Join(linhas, "\n")
	txt = reNLinhas.ReplaceAllString(txt, "\n\n")
	return strings.TrimSpace(txt)
}

// TextoSimples desescapa entidades HTML e colapsa espacos, sem remover tags.
//
// Serve para rotulos vindos do GLPI (categoria, entidade, status), que chegam
// escapados mesmo com expand_dropdowns -- "Sistemas &gt; Internos". Remover
// tags aqui destruiria um valor legitimo como "<sem categoria>".
func TextoSimples(bruto string) string {
	txt := bruto
	for i := 0; i < 3; i++ {
		if !reEntidade.MatchString(txt) {
			break
		}
		novo := html.UnescapeString(txt)
		if novo == txt {
			break
		}
		txt = novo
	}
	txt = strings.ReplaceAll(txt, " ", " ")
	return strings.Join(strings.Fields(txt), " ")
}

var acentos = strings.NewReplacer(
	"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "õ", "o", "ô", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// normalizarTexto deixa o texto comparavel: minusculo, sem acentos e com
// espacos colapsados.
func normalizarTexto(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = acentos.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
