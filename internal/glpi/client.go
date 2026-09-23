// Package glpi conversa com a API REST nativa do GLPI (apirest.php).
//
// O fluxo segue o que ja esta em producao no RoboMotiva: initSession com
// usuario/senha + App-Token, requisicoes GET com Session-Token no header e
// killSession no encerramento. Este servico e somente leitura -- nenhuma
// chamada altera dados no GLPI.
package glpi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Erro carrega o codigo HTTP devolvido pelo GLPI, para a API distinguir
// "sem resultado" de "credencial invalida".
type Erro struct {
	Status   int
	Operacao string
	Detalhe  string
}

func (e *Erro) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("%s [HTTP %d]: %s", e.Operacao, e.Status, e.Detalhe)
	}
	return fmt.Sprintf("%s: %s", e.Operacao, e.Detalhe)
}

// Client e um cliente da API REST do GLPI seguro para uso concorrente.
type Client struct {
	baseURL  string // .../apirest.php
	usuario  string
	senha    string
	appToken string
	http     *http.Client
	log      *slog.Logger

	// maxChamados limita o tamanho de uma varredura, protegendo o GLPI de
	// consultas de periodo aberto.
	maxChamados int

	// paralelismo e quantas buscas de encerramento correm ao mesmo tempo.
	// Mais rapido para o usuario, mais carga para o GLPI.
	paralelismo int

	// lote e quantos chamados sao pedidos por pagina.
	lote int

	// esperas define os intervalos entre retentativas apos erro 5xx.
	// Os testes encurtam isso.
	esperas []time.Duration

	// vagasBusca limita quantas buscas correm ao mesmo tempo no total.
	// Fatias e paginas se combinam: sem um teto unico, a concorrencia
	// multiplicaria e o GLPI levaria uma enxurrada.
	vagasBusca chan struct{}

	mu     sync.Mutex
	sessao string

	// Cadastro de usuarios (id -> nome), usado para traduzir requerente e
	// tecnico, que a busca devolve como ID.
	muUsuarios  sync.Mutex
	usuarios    map[string]string
	usuariosAte time.Time
}

// Opcoes de construcao do cliente.
type Opcoes struct {
	URL         string
	Usuario     string
	Senha       string
	AppToken    string
	VerifySSL   bool
	Timeout     time.Duration
	MaxChamados int
	Paralelismo int
	LoteBusca   int
	Log         *slog.Logger
}

// NovoCliente monta o cliente a partir da configuracao do .env.
func NovoCliente(o Opcoes) *Client {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	log := o.Log
	if log == nil {
		log = slog.Default()
	}

	transporte := http.DefaultTransport.(*http.Transport).Clone()
	transporte.TLSClientConfig = &tls.Config{InsecureSkipVerify: !o.VerifySSL}

	paralelismo := o.Paralelismo
	if paralelismo <= 0 {
		paralelismo = 16
	}

	lote := o.LoteBusca
	if lote <= 0 {
		lote = loteMaximo
	}
	if lote > loteMaximo {
		lote = loteMaximo
	}
	if lote < loteMinimo {
		lote = loteMinimo
	}

	simultaneas := paralelismo / 2
	teto := 8
	if lote >= 500 {
		teto = 6
	}
	if simultaneas < 1 {
		simultaneas = 1
	}
	if simultaneas > teto {
		simultaneas = teto
	}

	return &Client{
		baseURL:     NormalizarURL(o.URL),
		usuario:     o.Usuario,
		senha:       o.Senha,
		appToken:    o.AppToken,
		log:         log,
		maxChamados: o.MaxChamados,
		paralelismo: paralelismo,
		lote:        lote,
		vagasBusca:  make(chan struct{}, simultaneas),
		esperas:     esperasRetentativa,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transporte,
		},
	}
}

// NormalizarURL aceita a raiz do GLPI, um caminho de front ou a propria
// apirest.php e devolve sempre "<raiz>/apirest.php".
func NormalizarURL(bruto string) string {
	bruto = strings.TrimSpace(bruto)
	if u, err := url.Parse(bruto); err == nil && u.Host != "" {
		caminho := strings.TrimSuffix(u.Path, "/")
		for _, corte := range []string{"/apirest.php", "/front", "/index.php"} {
			if i := strings.Index(caminho, corte); i >= 0 {
				caminho = caminho[:i]
				break
			}
		}
		return fmt.Sprintf("%s://%s%s/apirest.php", u.Scheme, u.Host, caminho)
	}
	return strings.TrimSuffix(bruto, "/") + "/apirest.php"
}

// BaseURL expoe o endpoint resolvido (usado no /api/health e nos logs).
func (c *Client) BaseURL() string { return c.baseURL }

// ── Sessao ───────────────────────────────────────────────────────────────────

func (c *Client) tokenSessao(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessao != "" {
		return c.sessao, nil
	}
	if err := c.iniciarSessaoLocked(ctx); err != nil {
		return "", err
	}
	return c.sessao, nil
}

func (c *Client) iniciarSessaoLocked(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/initSession", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.usuario, c.senha)
	req.Header.Set("Content-Type", "application/json")
	if c.appToken != "" {
		req.Header.Set("App-Token", c.appToken)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &Erro{Operacao: "autenticando no GLPI", Detalhe: err.Error()}
	}
	defer resp.Body.Close()
	corpo, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// GLPI com API desabilitada responde a pagina HTML de login.
	if !pareceJSON(corpo) {
		return &Erro{
			Status:   resp.StatusCode,
			Operacao: "autenticando no GLPI",
			Detalhe: fmt.Sprintf(
				"resposta nao e JSON. Confira GLPI_URL (%s) e se a API REST esta habilitada em Configurar > Geral > API",
				c.baseURL),
		}
	}
	if resp.StatusCode != http.StatusOK {
		return &Erro{Status: resp.StatusCode, Operacao: "autenticando no GLPI", Detalhe: mensagemGLPI(corpo)}
	}

	var dados struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.Unmarshal(corpo, &dados); err != nil || dados.SessionToken == "" {
		return &Erro{Status: resp.StatusCode, Operacao: "autenticando no GLPI", Detalhe: "resposta sem session_token: " + trecho(corpo)}
	}

	c.sessao = dados.SessionToken
	c.log.Info("sessao GLPI iniciada", "url", c.baseURL)
	return nil
}

func (c *Client) descartarSessao(anterior string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessao == anterior {
		c.sessao = ""
	}
}

// EncerrarSessao chama killSession. Erros aqui nao interessam ao usuario.
func (c *Client) EncerrarSessao(ctx context.Context) {
	c.mu.Lock()
	token := c.sessao
	c.sessao = ""
	c.mu.Unlock()
	if token == "" {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/killSession", nil)
	if err != nil {
		return
	}
	req.Header.Set("Session-Token", token)
	if c.appToken != "" {
		req.Header.Set("App-Token", c.appToken)
	}
	resp, err := c.http.Do(req)
	if err == nil {
		resp.Body.Close()
	}
	c.log.Info("sessao GLPI encerrada")
}

// Ping confirma que a sessao esta valida (usado pelo /api/health).
func (c *Client) Ping(ctx context.Context) error {
	_, _, err := c.get(ctx, "/getMyProfiles", nil)
	return err
}

// ── Requisicao ───────────────────────────────────────────────────────────────

// get executa um GET autenticado. Em 401 renova a sessao e tenta uma vez mais.
// Devolve corpo, status e erro; status 204 vem com corpo nil e erro nil.
func (c *Client) get(ctx context.Context, caminho string, params url.Values) ([]byte, int, error) {
	for tentativa := 0; tentativa < 2; tentativa++ {
		token, err := c.tokenSessao(ctx)
		if err != nil {
			return nil, 0, err
		}

		alvo := c.baseURL + caminho
		if len(params) > 0 {
			alvo += "?" + params.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, alvo, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Session-Token", token)
		if c.appToken != "" {
			req.Header.Set("App-Token", c.appToken)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, 0, &Erro{Operacao: "consultando " + caminho, Detalhe: err.Error()}
		}
		corpo, lerErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if lerErr != nil {
			return nil, resp.StatusCode, &Erro{Status: resp.StatusCode, Operacao: "lendo resposta de " + caminho, Detalhe: lerErr.Error()}
		}

		switch {
		case resp.StatusCode == http.StatusUnauthorized && tentativa == 0:
			c.log.Warn("sessao GLPI expirada, reautenticando", "caminho", caminho)
			c.descartarSessao(token)
			continue
		case resp.StatusCode == http.StatusNoContent:
			return nil, resp.StatusCode, nil
		case resp.StatusCode == http.StatusOK, resp.StatusCode == http.StatusPartialContent:
			return corpo, resp.StatusCode, nil
		default:
			return nil, resp.StatusCode, &Erro{Status: resp.StatusCode, Operacao: "consultando " + caminho, Detalhe: mensagemGLPI(corpo)}
		}
	}
	return nil, http.StatusUnauthorized, &Erro{
		Status:   http.StatusUnauthorized,
		Operacao: "consultando " + caminho,
		Detalhe:  "sessao recusada apos reautenticar; confira GLPI_USUARIO, GLPI_SENHA e GLPI_APP_TOKEN",
	}
}

// pareceJSON aceita objeto e array: o GLPI devolve os erros como array
// (["ERROR_...", "explicacao"]).
func pareceJSON(corpo []byte) bool {
	t := strings.TrimSpace(string(corpo))
	return strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")
}

// mensagemGLPI extrai a explicacao que o GLPI manda no corpo do erro.
//
// Tratar esse array como "resposta invalida" escondia justamente o texto que
// diz o que esta errado.
func mensagemGLPI(corpo []byte) string {
	var partes []string
	if err := json.Unmarshal(corpo, &partes); err == nil && len(partes) > 0 {
		msg := strings.Join(partes, ": ")
		if dica := dicaDeErro(partes[0]); dica != "" {
			msg += " -- " + dica
		}
		return msg
	}
	return trecho(corpo)
}

// dicaDeErro traduz os codigos do GLPI que mais confundem na pratica.
func dicaDeErro(codigo string) string {
	switch strings.ToUpper(strings.TrimSpace(codigo)) {
	case "ERROR_WRONG_APP_TOKEN_PARAMETER":
		// O GLPI casa o app_token com o cliente da API cuja faixa de IPv4
		// cobre quem chamou. De um IP fora da faixa, o token "correto" vira
		// "invalido".
		return "confira GLPI_APP_TOKEN e, principalmente, se o IP deste servidor esta " +
			"no Intervalo de IPv4 do cliente da API (GLPI > Configurar > Geral > API)"
	case "ERROR_GLPI_LOGIN":
		return "confira GLPI_USUARIO e GLPI_SENHA"
	case "ERROR_LOGIN_PARAMETERS_MISSING", "ERROR_APP_TOKEN_PARAMETERS_MISSING":
		return "faltou credencial ou App-Token na requisicao"
	case "ERROR_SESSION_TOKEN_INVALID":
		return "sessao expirada"
	}
	return ""
}

func trecho(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}
