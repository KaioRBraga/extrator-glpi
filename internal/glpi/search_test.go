package glpi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func clienteDeTeste(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NovoCliente(Opcoes{
		URL:       srv.URL,
		Usuario:   "robo",
		Senha:     "segredo",
		AppToken:  "app-token",
		VerifySSL: false,
		Timeout:   5 * time.Second,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	// Esperas de verdade deixariam a suite em dezenas de segundos.
	c.esperas = []time.Duration{time.Millisecond, time.Millisecond}
	return c
}

func TestNormalizarURL(t *testing.T) {
	casos := map[string]string{
		"https://glpi.empresa.com.br":                  "https://glpi.empresa.com.br/apirest.php",
		"https://glpi.empresa.com.br/":                 "https://glpi.empresa.com.br/apirest.php",
		"https://glpi.empresa.com.br/apirest.php":      "https://glpi.empresa.com.br/apirest.php",
		"https://glpi.empresa.com.br/front/ticket.php": "https://glpi.empresa.com.br/apirest.php",
		"http://10.0.0.5/glpi":                         "http://10.0.0.5/glpi/apirest.php",
		"http://10.0.0.5/glpi/apirest.php":             "http://10.0.0.5/glpi/apirest.php",
		"http://10.0.0.5/glpi/front/central.php?tab=1": "http://10.0.0.5/glpi/apirest.php",
	}
	for entrada, esperado := range casos {
		if got := NormalizarURL(entrada); got != esperado {
			t.Errorf("NormalizarURL(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}

// respostaBusca imita o retorno de /search/Ticket com expand_dropdowns=1.
var respostaBusca = map[string]any{
	"totalcount": 4,
	"count":      4,
	"data": []map[string]any{
		{ // resolvido, com solucao ja na propria busca
			"2":  850214,
			"1":  "Aplicativos e Sistemas Internos e Externos",
			"12": "Fechado",
			"15": "2025-06-27 13:06:00",
			"17": "2025-07-02 11:20:00",
			"21": "&#60;p&#62;DADOS DO CHAMADO&#60;/p&#62;1) LOCALIZA&#199;&#195;O : MOTIVA ASIA 7) USU&#193;RIO DE REDE : s_rodgon 9) SOLICITA&#199;&#195;O : ERRO NA FERRAMENTA",
			"7":  "Aplicativos",
			"3":  "Media",
			"4":  "1955",
			"5":  []any{"1003", "1595"},
			"24": "<p>Ferramenta reinstalada</p>",
			"80": "Root > Motiva > Asia",
		},
		{ // em aberto
			"2":  850300,
			"1":  "Problemas com Equipamentos",
			"12": "Em atendimento (atribuido)",
			"15": "2025-06-28 09:00:00",
			"21": "DADOS DO CHAMADO 6) EQUIPAMENTO : HEADSET 7) PROBLEMA : BOTAO MUTE EMPERRADO",
			"3":  "Alta",
			"4":  "2306",
		},
		{ // resolvido sem solucao nenhuma -> desfecho so no follow-up
			"2":  850500,
			"1":  "Acesso",
			"12": "Fechado",
			"15": "2025-06-30 07:00:00",
			"17": "2025-06-30 16:00:00",
			"21": "DADOS DO CHAMADO 6) SOLICITACAO : RESET DE SENHA",
			"3":  "Baixa",
		},
		{ // resolvido, sem o campo 24 -> exige ITILSolution
			"2":  850400,
			"1":  "Infraestrutura",
			"12": "Solucionado",
			"15": "2025-06-29 08:10:00",
			"17": "2025-06-30 10:00:00",
			"21": "DADOS DO CHAMADO 6) SOLICITACAO : ESTRUTURA DE REDE",
			"3":  "Muito alta",
		},
	},
}

func servidorGLPI(t *testing.T, chamadasBusca *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		usuario, senha, ok := r.BasicAuth()
		if !ok || usuario != "robo" || senha != "segredo" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"erro":"credencial invalida"}`))
			return
		}
		if r.Header.Get("App-Token") != "app-token" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"erro":"App-Token ausente"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok-123"})
	})

	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Session-Token") != "tok-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		n := atomic.AddInt32(chamadasBusca, 1)
		if n > 1 {
			// segunda pagina vazia
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Query().Get("expand_dropdowns") != "1" {
			t.Errorf("expand_dropdowns ausente: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("criteria[0][field]") != campoDataAbertura {
			t.Errorf("criterio de periodo ausente: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(respostaBusca)
	})

	mux.HandleFunc("/apirest.php/Ticket/850400/ITILSolution", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"content": "<p>Solucao antiga</p>", "date_creation": "2025-06-29 09:00:00"},
			{"content": "<p>Cabo&nbsp;substituido</p>", "date_creation": "2025-06-30 10:00:00"},
		})
	})

	mux.HandleFunc("/apirest.php/search/User", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalcount": 3,
			"data": []map[string]any{
				{"2": 1955, "1": "f_detal", "9": "Fulano", "34": "de Tal"},
				{"2": 1003, "1": "t_um", "9": "Tecnico", "34": "Um"},
				{"2": 1595, "1": "t_dois", "9": "Tecnico", "34": "Dois"},
				// sem nome: cai para o login
				{"2": 2306, "1": "c_ciclano"},
			},
		})
	})

	mux.HandleFunc("/apirest.php/Ticket/850500/ITILSolution", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/apirest.php/Ticket/850500/ITILFollowup", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"content": "@S.I, favor verificar.", "date_creation": "2025-06-30 08:00:00", "is_private": 0},
			{"content": "<p>Normalizado.</p>", "date_creation": "2025-06-30 15:55:00", "is_private": 0},
			{"content": "senha temporaria Ab#12345", "date_creation": "2025-06-30 15:59:00", "is_private": 1},
		})
	})

	return httptest.NewServer(mux)
}

func TestBuscarChamados(t *testing.T) {
	var chamadas int32
	srv := servidorGLPI(t, &chamadas)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	periodo := Periodo{
		Inicio: time.Date(2025, 6, 1, 0, 0, 0, 0, time.Local),
		Fim:    time.Date(2025, 6, 30, 23, 59, 59, 0, time.Local),
	}
	chamados, err := c.BuscarChamados(context.Background(), periodo, rotulosPadrao)
	if err != nil {
		t.Fatalf("BuscarChamados: %v", err)
	}
	if len(chamados) != 4 {
		t.Fatalf("esperava 4 chamados, veio %d", len(chamados))
	}

	fechado := chamados[0]
	if fechado.ID != "850214" {
		t.Errorf("ID = %q", fechado.ID)
	}
	if fechado.MotivoAbertura != "ERRO NA FERRAMENTA" || fechado.OrigemMotivo != OrigemFormulario {
		t.Errorf("motivo de abertura = %q (%s)", fechado.MotivoAbertura, fechado.OrigemMotivo)
	}
	if fechado.MotivoEncerramento != "Ferramenta reinstalada" {
		t.Errorf("motivo de encerramento = %q", fechado.MotivoEncerramento)
	}
	if fechado.DataAbertura != "27/06/2025 13:06" || fechado.DataSolucao != "02/07/2025 11:20" {
		t.Errorf("datas = %q / %q", fechado.DataAbertura, fechado.DataSolucao)
	}
	// A busca devolve IDs; o cadastro de usuarios vira nome.
	if fechado.Tecnico != "Tecnico Um | Tecnico Dois" {
		t.Errorf("tecnico = %q", fechado.Tecnico)
	}
	if fechado.Requerente != "Fulano de Tal" {
		t.Errorf("requerente = %q", fechado.Requerente)
	}
	// O login alvo do formulario nao e quem abriu o chamado.
	if fechado.UsuarioRede != "s_rodgon" {
		t.Errorf("usuario de rede = %q", fechado.UsuarioRede)
	}
	if !fechado.Resolvido || fechado.AltaPrioridade {
		t.Errorf("flags = resolvido:%v alta:%v", fechado.Resolvido, fechado.AltaPrioridade)
	}

	aberto := chamados[1]
	if aberto.Resolvido {
		t.Errorf("chamado %s nao deveria estar resolvido", aberto.ID)
	}
	if aberto.MotivoEncerramento != MotivoEmAberto {
		t.Errorf("motivo de encerramento = %q, esperado %q", aberto.MotivoEncerramento, MotivoEmAberto)
	}
	if aberto.DataSolucao != "" {
		t.Errorf("chamado em aberto com data de solucao %q", aberto.DataSolucao)
	}
	if !aberto.AltaPrioridade {
		t.Errorf("prioridade Alta deveria contar como alta prioridade")
	}
	if aberto.MotivoAbertura != "BOTAO MUTE EMPERRADO" {
		t.Errorf("motivo de abertura = %q", aberto.MotivoAbertura)
	}
	// Usuario sem nome preenchido cai para o login.
	if aberto.Requerente != "c_ciclano" {
		t.Errorf("requerente sem nome = %q", aberto.Requerente)
	}

	// Encerrado sem solucao formal: o desfecho vem do ultimo follow-up
	// publico, e o follow-up privado (com senha) fica de fora.
	viaFollowup := chamados[2]
	if viaFollowup.MotivoEncerramento != "Normalizado." {
		t.Errorf("encerramento via follow-up = %q", viaFollowup.MotivoEncerramento)
	}
	if viaFollowup.OrigemEncerramento != OrigemFollowup {
		t.Errorf("origem = %q, esperada %q", viaFollowup.OrigemEncerramento, OrigemFollowup)
	}

	// Sem o campo 24, o texto vem do endpoint ITILSolution.
	viaEndpoint := chamados[3]
	if viaEndpoint.MotivoEncerramento != "Cabo substituido" {
		t.Errorf("solucao via ITILSolution = %q", viaEndpoint.MotivoEncerramento)
	}
	if viaEndpoint.OrigemEncerramento != OrigemSolucao {
		t.Errorf("origem = %q, esperada %q", viaEndpoint.OrigemEncerramento, OrigemSolucao)
	}

	stats := CalcularStats(len(chamados), chamados)
	if stats.Fechados != 3 || stats.Abertos != 1 {
		t.Errorf("stats = %+v", stats)
	}
	if stats.PercentualResolvidos != 75 {
		t.Errorf("percentual = %v, esperado 75", stats.PercentualResolvidos)
	}
	if stats.AltaPrioridade != 2 {
		t.Errorf("alta prioridade = %d, esperado 2", stats.AltaPrioridade)
	}
}

// Sessao expirada deve ser renovada de forma transparente.
func TestReautenticaEm401(t *testing.T) {
	var sessoes, buscas int32
	mux := http.NewServeMux()
	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&sessoes, 1)
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok-123"})
	})
	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&buscas, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	if _, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao); err != nil {
		t.Fatalf("BuscarChamados: %v", err)
	}
	if sessoes != 2 {
		t.Errorf("initSession chamado %d vez(es), esperado 2", sessoes)
	}
	if buscas != 2 {
		t.Errorf("search chamado %d vez(es), esperado 2", buscas)
	}
}

// Credencial invalida precisa chegar ao usuario com o codigo HTTP.
func TestErroDeCredencial(t *testing.T) {
	var chamadas int32
	srv := servidorGLPI(t, &chamadas)
	defer srv.Close()

	c := NovoCliente(Opcoes{
		URL:      srv.URL,
		Usuario:  "robo",
		Senha:    "errada",
		AppToken: "app-token",
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	_, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao)
	if err == nil {
		t.Fatal("esperava erro de autenticacao")
	}
	var erroGLPI *Erro
	if !asErro(err, &erroGLPI) || erroGLPI.Status != http.StatusUnauthorized {
		t.Fatalf("erro = %v", err)
	}
}

func asErro(err error, alvo **Erro) bool {
	e, ok := err.(*Erro)
	if ok {
		*alvo = e
	}
	return ok
}
