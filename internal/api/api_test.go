package api

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"extratorglpi/internal/config"
	"extratorglpi/internal/glpi"
)

// glpiFalso responde /initSession e /search/Ticket com dois chamados: um
// fechado com solucao e um em aberto.
func glpiFalso(t *testing.T, credencialOK bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		if !credencialOK {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"erro":"senha invalida"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok"})
	})

	primeira := true
	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		if !primeira {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		primeira = false
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalcount": 2,
			"data": []map[string]any{
				{
					"2": 101, "1": "Aplicativos", "12": "Fechado",
					"15": "2025-06-10 08:00:00", "17": "2025-06-11 09:30:00",
					"21": "DADOS DO CHAMADO 9) SOLICITACAO : ERRO NA FERRAMENTA",
					"3":  "Media", "4": "Fulano de Tal", "24": "<p>Reinstalado</p>",
				},
				{
					"2": 102, "1": "Infraestrutura", "12": "Novo",
					"15": "2025-06-12 10:00:00",
					"21": "DADOS DO CHAMADO 6) SOLICITACAO : ESTRUTURA DE REDE",
					"3":  "Alta", "4": "Ciclano",
				},
			},
		})
	})

	mux.HandleFunc("/apirest.php/getMyProfiles", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"myprofiles": []any{}})
	})

	return httptest.NewServer(mux)
}

func servicoDeTeste(t *testing.T, credencialOK bool) (*httptest.Server, func()) {
	t.Helper()
	glpiSrv := glpiFalso(t, credencialOK)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		GLPIURL:      glpiSrv.URL,
		GLPIUsuario:  "robo",
		GLPISenha:    "segredo",
		GLPIAppToken: "app",
		MotivoLabels: []string{"SOLICITACAO", "PROBLEMA"},
		CacheTTL:     time.Minute,
		MaxChamados:  5000,
	}
	cliente := glpi.NovoCliente(glpi.Opcoes{
		URL: cfg.GLPIURL, Usuario: cfg.GLPIUsuario, Senha: cfg.GLPISenha,
		AppToken: cfg.GLPIAppToken, Log: log,
	})
	// fstest.MapFS vazio simula o front-end ainda nao compilado.
	srv := httptest.NewServer(Novo(cliente, cfg, log).Rotas(fstest.MapFS{}))
	return srv, func() {
		srv.Close()
		glpiSrv.Close()
	}
}

func TestListarChamados(t *testing.T) {
	srv, fechar := servicoDeTeste(t, true)
	defer fechar()

	resp, err := http.Get(srv.URL + "/api/chamados?inicio=2025-06-01&fim=2025-06-30")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		corpo, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, corpo)
	}

	var r respostaChamados
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	if len(r.Dados) != 2 {
		t.Fatalf("dados = %d chamados", len(r.Dados))
	}
	if r.Stats.Total != 2 || r.Stats.Fechados != 1 || r.Stats.Abertos != 1 {
		t.Errorf("stats = %+v", r.Stats)
	}
	if r.Stats.PercentualResolvidos != 50 {
		t.Errorf("percentual = %v, esperado 50", r.Stats.PercentualResolvidos)
	}
	if r.Stats.AltaPrioridade != 1 {
		t.Errorf("alta prioridade = %d", r.Stats.AltaPrioridade)
	}
	if r.Dados[1].MotivoEncerramento != glpi.MotivoEmAberto {
		t.Errorf("chamado aberto = %q", r.Dados[1].MotivoEncerramento)
	}
}

func TestFiltroDeStatusAfetaCardsEDados(t *testing.T) {
	srv, fechar := servicoDeTeste(t, true)
	defer fechar()

	resp, err := http.Get(srv.URL + "/api/chamados?status=resolvidos")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var r respostaChamados
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	if len(r.Dados) != 1 || r.Dados[0].ID != "101" {
		t.Fatalf("dados = %+v", r.Dados)
	}
	if r.Stats.Total != 2 || r.Stats.Filtrados != 1 || r.Stats.PercentualResolvidos != 100 {
		t.Errorf("stats = %+v", r.Stats)
	}
}

func TestExportarCSV(t *testing.T) {
	srv, fechar := servicoDeTeste(t, true)
	defer fechar()

	resp, err := http.Get(srv.URL + "/api/export.csv?inicio=2025-06-01&fim=2025-06-30")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "chamados_2025-06-01_a_2025-06-30.csv") {
		t.Errorf("content-disposition = %q", cd)
	}

	bruto, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(bruto), "\xEF\xBB\xBF") {
		t.Error("CSV sem BOM UTF-8: o Excel abriria com acentos quebrados")
	}

	leitor := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(bruto), "\xEF\xBB\xBF")))
	leitor.Comma = ';'
	linhas, err := leitor.ReadAll()
	if err != nil {
		t.Fatalf("CSV invalido: %v", err)
	}
	if len(linhas) != 3 {
		t.Fatalf("esperava cabecalho + 2 linhas, veio %d", len(linhas))
	}
	if linhas[0][0] != "ID" || linhas[0][5] != "Motivo de abertura" || linhas[0][7] != "Motivo de encerramento" {
		t.Errorf("cabecalho = %v", linhas[0])
	}
	if linhas[1][5] != "ERRO NA FERRAMENTA" || linhas[1][7] != "Reinstalado" || linhas[1][8] != glpi.OrigemSolucao {
		t.Errorf("linha do chamado fechado = %v", linhas[1])
	}
	if linhas[1][3] != "10/06/2025 08:00" || linhas[1][4] != "11/06/2025 09:30" {
		t.Errorf("datas = %v", linhas[1][3:5])
	}
	if linhas[2][7] != glpi.MotivoEmAberto || linhas[2][8] != glpi.OrigemEmAberto {
		t.Errorf("chamado em aberto = %q / %q", linhas[2][7], linhas[2][8])
	}
}

func TestDataInvalidaDevolve400(t *testing.T) {
	srv, fechar := servicoDeTeste(t, true)
	defer fechar()

	resp, err := http.Get(srv.URL + "/api/chamados?inicio=31-12-2025")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", resp.StatusCode)
	}
}

func TestPeriodoInvertidoDevolve400(t *testing.T) {
	srv, fechar := servicoDeTeste(t, true)
	defer fechar()

	resp, err := http.Get(srv.URL + "/api/chamados?inicio=2025-06-30&fim=2025-06-01")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", resp.StatusCode)
	}
}

func TestCredencialInvalidaDevolve502(t *testing.T) {
	srv, fechar := servicoDeTeste(t, false)
	defer fechar()

	resp, err := http.Get(srv.URL + "/api/chamados")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, esperado 502", resp.StatusCode)
	}
	var corpo map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&corpo); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(corpo["erro"], "GLPI_USUARIO") {
		t.Errorf("mensagem pouco util: %q", corpo["erro"])
	}
}

func TestSanitizarProtegeContraFormula(t *testing.T) {
	casos := map[string]string{
		"=SOMA(A1)":     "'=SOMA(A1)",
		"+1":            "'+1",
		"-1":            "'-1",
		"@cmd":          "'@cmd",
		"texto normal":  "texto normal",
		"linha\nquebra": "linha quebra",
	}
	for entrada, esperado := range casos {
		if got := sanitizar(entrada); got != esperado {
			t.Errorf("sanitizar(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}
