package glpi

import (
	"regexp"
	"strings"
)

// reUsuarioSolto cobre as descricoes que nao seguem a lista numerada do
// formulario ("Usuario de rede: j_regmon", "Login: joao.silva"), mesmo padrao
// ja usado no RoboMotiva.
var reUsuarioSolto = []*regexp.Regexp{
	regexp.MustCompile(`(?i)usu[aá]rio\s*de\s*rede\s*[:\-]\s*([A-Za-z0-9._\-]+)`),
	regexp.MustCompile(`(?i)login\s*de\s*rede\s*[:\-]\s*([A-Za-z0-9._\-]+)`),
	regexp.MustCompile(`(?i)login\s*[:\-]\s*([A-Za-z0-9._\-]+)`),
	regexp.MustCompile(`(?i)usu[aá]rio\s*[:\-]\s*([A-Za-z0-9._\-]+)`),
	regexp.MustCompile(`(?i)user\s*[:\-]\s*([A-Za-z0-9._\-]+)`),
}

// UsuarioRede extrai o login alvo do chamado -- o "USUARIO DE REDE" do
// formulario, que e de quem o chamado trata, nem sempre quem abriu.
//
// Procura primeiro nos campos do formulario, na ordem de labels; se o chamado
// nao tiver formulario, tenta os mesmos rotulos soltos no texto.
func UsuarioRede(descricao string, labels []string) string {
	porRotulo := make(map[string]string)
	for _, c := range ExtrairCamposForm(descricao) {
		if c.Valor == "" {
			continue
		}
		if _, existe := porRotulo[c.RotuloNorm]; !existe {
			porRotulo[c.RotuloNorm] = c.Valor
		}
	}

	for _, label := range labels {
		if v, ok := porRotulo[normalizarTexto(label)]; ok {
			return primeiroToken(v)
		}
	}

	for _, re := range reUsuarioSolto {
		if m := re.FindStringSubmatch(descricao); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

// primeiroToken devolve so o login. Ha formularios que escrevem
// "j_regmon (Joao Regis)" ou "j_regmon - Operacao" no mesmo campo.
func primeiroToken(valor string) string {
	campos := strings.Fields(valor)
	if len(campos) == 0 {
		return ""
	}
	return strings.Trim(campos[0], ",;.()[]")
}
