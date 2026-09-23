package api

import (
	"encoding/csv"
	"io"
	"strings"

	"extratorglpi/internal/glpi"
)

// Colunas do arquivo exportado, na ordem pedida pelo SDD.
var cabecalhoCSV = []string{
	"ID",
	"Título",
	"Status",
	"Data de abertura",
	"Hora de abertura",
	"Data de solução",
	"Hora de solução",
	"Atendimento",
	"Motivo de abertura",
	"Origem do motivo",
	"Motivo de encerramento",
	"Origem do encerramento",
	"Usuário de rede",
	"Requerente",
	"Técnico",
	"Categoria",
	"Prioridade",
	"Entidade",
}

// EscreverCSV gera o arquivo no formato que o Excel pt-BR abre sem ajuste:
// UTF-8 com BOM e ponto e virgula como separador (mesmo padrao do export
// nativo do GLPI).
func EscreverCSV(w io.Writer, chamados []glpi.Chamado) error {
	// BOM em bytes explicitos: e o que faz o Excel reconhecer UTF-8.
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	if err := cw.Write(cabecalhoCSV); err != nil {
		return err
	}

	for _, c := range chamados {
		linha := []string{
			c.ID,
			c.Titulo,
			c.Status,
			c.DataAbertura,
			c.HoraAbertura,
			c.DataSolucao,
			c.HoraSolucao,
			c.Atendimento,
			c.MotivoAbertura,
			c.OrigemMotivo,
			c.MotivoEncerramento,
			c.OrigemEncerramento,
			c.UsuarioRede,
			c.Requerente,
			c.Tecnico,
			c.Categoria,
			c.Prioridade,
			c.Entidade,
		}
		for i, v := range linha {
			linha[i] = sanitizar(v)
		}
		if err := cw.Write(linha); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// sanitizar protege contra injecao de formula: o Excel executaria uma celula
// que comeca com =, +, - ou @. Tambem achata quebras de linha vindas do GLPI.
func sanitizar(v string) string {
	v = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(v)
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if strings.ContainsRune("=+-@\t", rune(v[0])) {
		return "'" + v
	}
	return v
}
