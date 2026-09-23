package api

import (
	"context"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// tempoMaximoPadrao vale quando a configuracao nao diz outra coisa.
const tempoMaximoPadrao = 10 * time.Minute

// Rotas monta o handler completo: API + SPA embutida.
func (s *Servico) Rotas(estaticos fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleSaude)
	mux.HandleFunc("GET /api/chamados", s.handleChamados)
	mux.HandleFunc("GET /api/export.csv", s.handleExportar)
	mux.Handle("GET /", handlerSPA(estaticos))

	limite := s.cfg.TempoConsulta
	if limite <= 0 {
		limite = tempoMaximoPadrao
	}
	return s.registrar(s.recuperar(comTimeout(limite, cabecalhosSeguranca(mux))))
}

// handlerSPA serve os arquivos do build do React e cai no index.html para
// qualquer rota desconhecida (navegacao no cliente).
func handlerSPA(estaticos fs.FS) http.Handler {
	arquivos := http.FileServer(http.FS(estaticos))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(estaticos, "index.html"); err != nil {
			http.Error(w, "Front-end nao compilado: rode 'npm ci && npm run build' em web/ antes de subir o servico.", http.StatusServiceUnavailable)
			return
		}

		limpo := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if limpo == "" || limpo == "." {
			servirIndex(w, r, estaticos)
			return
		}
		if _, err := fs.Stat(estaticos, limpo); err != nil {
			servirIndex(w, r, estaticos)
			return
		}

		// Os arquivos do Vite levam hash no nome; o index.html nunca e cacheado.
		if strings.HasPrefix(limpo, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		arquivos.ServeHTTP(w, r)
	})
}

func servirIndex(w http.ResponseWriter, r *http.Request, estaticos fs.FS) {
	conteudo, err := fs.ReadFile(estaticos, "index.html")
	if err != nil {
		http.Error(w, "index.html indisponivel", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(string(conteudo)))
}

// ── Middlewares ──────────────────────────────────────────────────────────────

type respostaObservada struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *respostaObservada) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *respostaObservada) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (s *Servico) registrar(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		obs := &respostaObservada{ResponseWriter: w, status: http.StatusOK}
		proximo.ServeHTTP(obs, r)

		nivel := s.log.Info
		if obs.status >= 500 {
			nivel = s.log.Error
		}
		nivel("requisicao",
			"metodo", r.Method,
			"rota", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", obs.status,
			"bytes", obs.bytes,
			"duracao", time.Since(inicio).Round(time.Millisecond),
		)
	})
}

func (s *Servico) recuperar(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("panico atendendo requisicao", "rota", r.URL.Path, "panico", p)
				responderJSON(w, http.StatusInternalServerError, map[string]string{
					"erro": "erro interno no extrator",
				})
			}
		}()
		proximo.ServeHTTP(w, r)
	})
}

// comTimeout poe um teto por requisicao: sem ele, um periodo enorme deixaria
// a consulta pendurada indefinidamente.
func comTimeout(limite time.Duration, proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancelar := context.WithTimeout(r.Context(), limite)
		defer cancelar()
		proximo.ServeHTTP(w, r.WithContext(ctx))
	})
}

func cabecalhosSeguranca(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		proximo.ServeHTTP(w, r)
	})
}
