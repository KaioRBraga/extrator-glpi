package glpi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// loteTeste mantem os testes rapidos: paginas pequenas, varias delas.
const loteTeste = 100

// servidorPaginado devolve `total` chamados em lotes de loteTeste.
//
// Com `deslocar`, a cada pagina os IDs sao empurrados em um -- imita um
// chamado novo entrando no meio da varredura, que faz paginas paralelas se
// sobreporem.
func servidorPaginado(t *testing.T, total int, deslocar bool, concorrentes *int32, pico *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok"})
	})
	mux.HandleFunc("/apirest.php/search/User", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		agora := atomic.AddInt32(concorrentes, 1)
		defer atomic.AddInt32(concorrentes, -1)
		for {
			anterior := atomic.LoadInt32(pico)
			if agora <= anterior || atomic.CompareAndSwapInt32(pico, anterior, agora) {
				break
			}
		}

		// Sem uma resposta lenta, as requisicoes terminam antes de se
		// sobreporem e a medicao de paralelismo fica sem sentido.
		time.Sleep(30 * time.Millisecond)

		faixa := r.URL.Query().Get("range")
		inicio, _ := strconv.Atoi(strings.Split(faixa, "-")[0])
		if inicio >= total {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		deslocamento := 0
		if deslocar && inicio > 0 {
			deslocamento = inicio / loteTeste
		}

		var dados []map[string]any
		for i := inicio; i < inicio+loteTeste && i < total; i++ {
			id := i + 1 - deslocamento
			dados = append(dados, map[string]any{
				"2":  id,
				"1":  fmt.Sprintf("Chamado %d", id),
				"12": "Novo",
				"15": "2026-09-10 08:00:00",
				"21": "DADOS DO CHAMADO 1) SOLICITACAO : TESTE",
				"3":  "3",
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"totalcount": total, "data": dados})
	})

	return httptest.NewServer(mux)
}

func TestBuscaPaginadaEmParalelo(t *testing.T) {
	var concorrentes, pico int32
	srv := servidorPaginado(t, loteTeste*2+50, false, &concorrentes, &pico)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	c.lote = loteTeste
	chamados, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao)
	if err != nil {
		t.Fatalf("BuscarChamados: %v", err)
	}
	total := loteTeste*2 + 50
	if len(chamados) != total {
		t.Fatalf("esperava %d chamados, veio %d", total, len(chamados))
	}
	// Ordem preservada apesar do paralelismo.
	if chamados[0].ID != "1" || chamados[total-1].ID != strconv.Itoa(total) {
		t.Errorf("ordem quebrada: primeiro=%s ultimo=%s", chamados[0].ID, chamados[total-1].ID)
	}
	if pico < 2 {
		t.Errorf("paginas nao foram buscadas em paralelo (pico de %d requisicao simultanea)", pico)
	}
}

// Chamado novo durante a varredura nao pode gerar duplicata no CSV.
func TestBuscaPaginadaDeduplica(t *testing.T) {
	var concorrentes, pico int32
	srv := servidorPaginado(t, loteTeste*3, true, &concorrentes, &pico)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	c.lote = loteTeste
	chamados, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao)
	if err != nil {
		t.Fatalf("BuscarChamados: %v", err)
	}

	vistos := map[string]bool{}
	for _, ch := range chamados {
		if vistos[ch.ID] {
			t.Fatalf("chamado %s duplicado", ch.ID)
		}
		vistos[ch.ID] = true
	}
}

// O teto de chamados tambem limita quantas paginas sao baixadas.
func TestBuscaRespeitaMaxChamados(t *testing.T) {
	var concorrentes, pico int32
	srv := servidorPaginado(t, loteTeste*4, false, &concorrentes, &pico)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	c.lote = loteTeste
	c.maxChamados = loteTeste + 10

	chamados, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao)
	if err != nil {
		t.Fatalf("BuscarChamados: %v", err)
	}
	if len(chamados) < c.maxChamados || len(chamados) > c.maxChamados+loteTeste {
		t.Errorf("esperava parar perto de %d, veio %d", c.maxChamados, len(chamados))
	}
}

// Instalacoes menores devolvem HTTP 500 quando o lote e grande demais --
// visto no GLPI da Motiva com 2000 por pagina. A busca precisa se adaptar
// sozinha, em vez de falhar inteira.
func TestBuscaQuebraLoteQuandoGLPIRecusa(t *testing.T) {
	const limiteDoServidor = 60

	var recusas, aceites int32
	mux := http.NewServeMux()
	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok"})
	})
	mux.HandleFunc("/apirest.php/search/User", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		faixa := strings.Split(r.URL.Query().Get("range"), "-")
		inicio, _ := strconv.Atoi(faixa[0])
		fim, _ := strconv.Atoi(faixa[1])
		tamanho := fim - inicio + 1

		if tamanho > limiteDoServidor {
			atomic.AddInt32(&recusas, 1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"erro":"allowed memory size exhausted"}`))
			return
		}
		atomic.AddInt32(&aceites, 1)

		const total = 250
		if inicio >= total {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var dados []map[string]any
		for i := inicio; i < inicio+tamanho && i < total; i++ {
			dados = append(dados, map[string]any{
				"2": i + 1, "1": fmt.Sprintf("Chamado %d", i+1), "12": "Novo",
				"15": "2026-09-10 08:00:00", "21": "1) SOLICITACAO : TESTE", "3": "3",
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"totalcount": total, "data": dados})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	c.lote = 200 // acima do que este servidor aguenta

	chamados, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao)
	if err != nil {
		t.Fatalf("BuscarChamados: %v", err)
	}
	if len(chamados) != 250 {
		t.Errorf("esperava 250 chamados, veio %d", len(chamados))
	}
	if recusas == 0 {
		t.Error("o servidor deveria ter recusado o lote grande pelo menos uma vez")
	}
	if aceites == 0 {
		t.Error("nenhuma pagina foi aceita")
	}
}

// Sob carga o GLPI recusa requisicoes que sozinhas funcionariam. Insistir
// resolve; reduzir o lote, nao.
func TestBuscaInsisteAposErroTransitorio(t *testing.T) {
	var tentativas int32
	mux := http.NewServeMux()
	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok"})
	})
	mux.HandleFunc("/apirest.php/search/User", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		// Falha nas duas primeiras; na terceira responde.
		if atomic.AddInt32(&tentativas, 1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalcount": 1,
			"data": []map[string]any{
				{"2": 1, "1": "Chamado", "12": "Novo", "15": "2026-09-10 08:00:00",
					"21": "1) SOLICITACAO : TESTE", "3": "3"},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	chamados, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao)
	if err != nil {
		t.Fatalf("BuscarChamados: %v", err)
	}
	if len(chamados) != 1 {
		t.Errorf("esperava 1 chamado, veio %d", len(chamados))
	}
	if tentativas != 3 {
		t.Errorf("%d tentativas, esperado 3", tentativas)
	}
}

// Erro que nao e do servidor (400) nao merece retentativa.
func TestBuscaNaoInsisteEmErroDeCliente(t *testing.T) {
	var tentativas int32
	mux := http.NewServeMux()
	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok"})
	})
	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tentativas, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"erro":"criterio invalido"}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	if _, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao); err == nil {
		t.Fatal("esperava erro")
	}
	if tentativas != 1 {
		t.Errorf("%d tentativas, esperado 1", tentativas)
	}
}

// Um chamado com follow-up gigante derruba a pagina inteira, por menor que
// ela seja. A faixa precisa passar sem esses campos, deixando o encerramento
// para a consulta individual.
func TestBuscaDesligaFollowupsQuandoAFaixaNaoPassa(t *testing.T) {
	var comFollow, semFollow int32
	mux := http.NewServeMux()
	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok"})
	})
	mux.HandleFunc("/apirest.php/search/User", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/apirest.php/Ticket/1/ITILSolution", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/apirest.php/Ticket/1/ITILFollowup", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"content": "Normalizado.", "date_creation": "2026-09-10 10:00:00", "is_private": 0},
		})
	})
	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		pedeFollow := strings.Contains(r.URL.RawQuery, "="+campoFollowTexto+"&") ||
			strings.HasSuffix(r.URL.RawQuery, "="+campoFollowTexto)
		if pedeFollow {
			atomic.AddInt32(&comFollow, 1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		atomic.AddInt32(&semFollow, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalcount": 1,
			"data": []map[string]any{
				{"2": 1, "1": "Chamado grande", "12": "Solucionado",
					"15": "2026-09-10 08:00:00", "17": "2026-09-10 10:00:00",
					"21": "1) SOLICITACAO : TESTE", "3": "3"},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	c.lote = loteMinimo

	chamados, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao)
	if err != nil {
		t.Fatalf("BuscarChamados: %v", err)
	}
	if len(chamados) != 1 {
		t.Fatalf("esperava 1 chamado, veio %d", len(chamados))
	}
	if comFollow == 0 || semFollow == 0 {
		t.Errorf("tentativas: com follow-up=%d, sem=%d", comFollow, semFollow)
	}
	// O encerramento veio da consulta individual, nao ficou vazio.
	if chamados[0].MotivoEncerramento != "Normalizado." {
		t.Errorf("encerramento = %q", chamados[0].MotivoEncerramento)
	}
}
