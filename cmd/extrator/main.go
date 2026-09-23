// Comando extrator: servico web HTTPS que extrai chamados do GLPI.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Banco de fusos embutido: a imagem distroless nao traz tzdata, e sem ele
	// TZ=America/Sao_Paulo seria silenciosamente ignorado.
	_ "time/tzdata"

	"extratorglpi/internal/api"
	"extratorglpi/internal/config"
	"extratorglpi/internal/glpi"
	"extratorglpi/web"
)

func main() {
	caminhoEnv := flag.String("env", ".env", "caminho do arquivo .env")
	checarSaude := flag.Bool("healthcheck", false, "consulta /api/health no proprio servico e sai (usado pelo Docker)")
	flag.Parse()

	if *checarSaude {
		os.Exit(healthcheck())
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: nivelLog()}))
	slog.SetDefault(log)

	cfg, err := config.Load(*caminhoEnv)
	if err != nil {
		// Sem configuracao valida o servico nao sobe -- exigencia do SDD.
		log.Error("configuracao invalida", "erro", err)
		os.Exit(1)
	}

	cliente := glpi.NovoCliente(glpi.Opcoes{
		URL:         cfg.GLPIURL,
		Usuario:     cfg.GLPIUsuario,
		Senha:       cfg.GLPISenha,
		AppToken:    cfg.GLPIAppToken,
		VerifySSL:   cfg.GLPIVerifySSL,
		MaxChamados: cfg.MaxChamados,
		Paralelismo: cfg.Paralelismo,
		LoteBusca:   cfg.LoteBusca,
		Log:         log,
	})

	servico := api.Novo(cliente, cfg, log)
	servidor := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           servico.Rotas(web.Arquivos()),
		ReadHeaderTimeout: 15 * time.Second,
		// Exportacoes de periodos longos demoram; o teto real por requisicao
		// fica no middleware de timeout.
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  2 * time.Minute,
		TLSConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
		ErrorLog:     slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	// Aviso antecipado de credencial errada, sem impedir a subida: o GLPI pode
	// estar apenas indisponivel no momento.
	ctxPing, cancelarPing := context.WithTimeout(context.Background(), 20*time.Second)
	if err := cliente.Ping(ctxPing); err != nil {
		log.Warn("nao foi possivel falar com o GLPI na subida", "erro", err)
	} else {
		log.Info("conexao com o GLPI confirmada", "url", cliente.BaseURL())
	}
	cancelarPing()

	// O aquecimento roda junto com o servidor e para no encerramento.
	ctxAquecimento, pararAquecimento := context.WithCancel(context.Background())
	go servico.Aquecer(ctxAquecimento)

	encerrar := make(chan os.Signal, 1)
	signal.Notify(encerrar, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("extrator no ar",
			"endereco", "https://localhost:"+cfg.ServerPort,
			"cache_ttl", cfg.CacheTTL.String(),
			"max_chamados", cfg.MaxChamados,
		)
		err := servidor.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("servidor parou", "erro", err)
			os.Exit(1)
		}
	}()

	<-encerrar
	log.Info("encerrando...")
	pararAquecimento()

	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	if err := servidor.Shutdown(ctx); err != nil {
		log.Warn("encerramento forcado", "erro", err)
	}
	cliente.EncerrarSessao(ctx)
}

// healthcheck permite que o HEALTHCHECK do container use o proprio binario --
// a imagem distroless nao tem curl nem wget.
func healthcheck() int {
	porta := os.Getenv("SERVER_PORT")
	if porta == "" {
		porta = "8443"
	}
	cliente := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			// O certificado e interno; aqui so interessa se o servico responde.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := cliente.Get("https://127.0.0.1:" + porta + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: HTTP", resp.StatusCode)
		return 1
	}
	return 0
}

func nivelLog() slog.Level {
	var nivel slog.Level
	if err := nivel.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
		return slog.LevelInfo
	}
	return nivel
}
