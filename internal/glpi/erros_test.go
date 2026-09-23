package glpi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// O GLPI devolve erro como array JSON. Tratar isso como "resposta invalida"
// escondia a unica mensagem que dizia o que estava errado -- foi o que
// aconteceu no deploy do servidor.
func TestErroDoGLPIChegaAoUsuario(t *testing.T) {
	casos := []struct {
		nome   string
		status int
		corpo  string
		contem []string
	}{
		{
			nome:   "app-token recusado aponta a faixa de IPv4",
			status: http.StatusBadRequest,
			corpo:  `["ERROR_WRONG_APP_TOKEN_PARAMETER","o parametro app_token parece ser invalido"]`,
			contem: []string{"ERROR_WRONG_APP_TOKEN_PARAMETER", "app_token parece ser invalido", "Intervalo de IPv4"},
		},
		{
			nome:   "credencial errada aponta usuario e senha",
			status: http.StatusUnauthorized,
			corpo:  `["ERROR_GLPI_LOGIN","Nome de usuario ou senha invalidos"]`,
			contem: []string{"ERROR_GLPI_LOGIN", "GLPI_USUARIO"},
		},
		{
			nome:   "erro sem codigo conhecido passa o texto do GLPI",
			status: http.StatusBadRequest,
			corpo:  `["ERROR_QUALQUER","explicacao do glpi"]`,
			contem: []string{"ERROR_QUALQUER", "explicacao do glpi"},
		},
		{
			nome:   "pagina HTML continua indicando URL ou API desabilitada",
			status: http.StatusOK,
			corpo:  "<!DOCTYPE html><html><body>login</body></html>",
			contem: []string{"resposta nao e JSON", "API REST esta habilitada"},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.corpo))
			}))
			defer srv.Close()

			cli := NovoCliente(Opcoes{
				URL: srv.URL, Usuario: "robo", Senha: "segredo", AppToken: "app",
				Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})

			err := cli.Ping(context.Background())
			if err == nil {
				t.Fatal("esperava erro")
			}
			for _, trecho := range c.contem {
				if !strings.Contains(err.Error(), trecho) {
					t.Errorf("mensagem nao contem %q:\n  %v", trecho, err)
				}
			}
		})
	}
}

// Erro no meio da busca tambem precisa carregar o texto do GLPI.
func TestErroDeBuscaMostraMensagemDoGLPI(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/apirest.php/initSession", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"session_token": "tok"})
	})
	mux.HandleFunc("/apirest.php/search/Ticket", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`["ERROR_FIELD_NOT_FOUND","campo 999 nao existe"]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := clienteDeTeste(t, srv)
	_, err := c.BuscarChamados(context.Background(), Periodo{}, rotulosPadrao)
	if err == nil {
		t.Fatal("esperava erro")
	}
	if !strings.Contains(err.Error(), "campo 999 nao existe") {
		t.Errorf("mensagem = %v", err)
	}
}
