// Package config le a configuracao do servico exclusivamente do ambiente
// (arquivo .env ou variaveis exportadas).
//
// Por exigencia do SDD, as credenciais do GLPI e os caminhos de TLS nao tem
// valor padrao: se qualquer uma faltar, Load devolve erro listando o que falta
// e o servico nao sobe.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config e a configuracao completa do servico.
type Config struct {
	GLPIURL       string
	GLPIAppToken  string
	GLPIUsuario   string
	GLPISenha     string
	GLPIVerifySSL bool

	TLSCertFile string
	TLSKeyFile  string
	ServerPort  string

	MotivoLabels  []string
	UsuarioLabels []string
	CacheTTL      time.Duration
	TempoConsulta time.Duration
	AquecerCache  bool
	MaxChamados   int
	Paralelismo   int
	LoteBusca     int
}

// obrigatorias sao lidas sem qualquer fallback.
var obrigatorias = []string{
	"GLPI_URL",
	"GLPI_APP_TOKEN",
	"GLPI_USUARIO",
	"GLPI_SENHA",
	"TLS_CERT_FILE",
	"TLS_KEY_FILE",
}

// Load carrega o .env (se existir) e valida o ambiente.
//
// A ausencia do arquivo .env nao e erro por si so -- em container as variaveis
// chegam pelo env_file/environment. O que importa e o conjunto final.
func Load(envPath string) (*Config, error) {
	if envPath != "" {
		if err := godotenv.Load(envPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("lendo %s: %w", envPath, err)
		}
	}

	var faltando []string
	for _, nome := range obrigatorias {
		if strings.TrimSpace(os.Getenv(nome)) == "" {
			faltando = append(faltando, nome)
		}
	}
	if len(faltando) > 0 {
		return nil, fmt.Errorf(
			"configuracao incompleta: defina %s no .env (o servico nao usa valores padrao para esses campos)",
			strings.Join(faltando, ", "),
		)
	}

	cfg := &Config{
		GLPIURL:       strings.TrimSpace(os.Getenv("GLPI_URL")),
		GLPIAppToken:  strings.TrimSpace(os.Getenv("GLPI_APP_TOKEN")),
		GLPIUsuario:   strings.TrimSpace(os.Getenv("GLPI_USUARIO")),
		GLPISenha:     os.Getenv("GLPI_SENHA"),
		GLPIVerifySSL: boolEnv("GLPI_VERIFY_SSL", true),
		TLSCertFile:   strings.TrimSpace(os.Getenv("TLS_CERT_FILE")),
		TLSKeyFile:    strings.TrimSpace(os.Getenv("TLS_KEY_FILE")),
		ServerPort:    strEnv("SERVER_PORT", "8443"),
		MotivoLabels:  listaEnv("MOTIVO_ABERTURA_LABELS", []string{"SOLICITACAO", "PROBLEMA", "ATENDIMENTO", "MOTIVO"}),
		UsuarioLabels: listaEnv("USUARIO_REDE_LABELS", []string{"USUARIO DE REDE", "LOGIN DE REDE", "USUARIO", "LOGIN", "USER"}),
		MaxChamados:   intEnv("MAX_CHAMADOS", 5000),
		Paralelismo:   intEnv("GLPI_PARALELISMO", 16),
		LoteBusca:     intEnv("GLPI_LOTE_BUSCA", 1000),
		AquecerCache:  boolEnv("AQUECER_CACHE", true),
	}

	ttl, err := time.ParseDuration(strEnv("CACHE_TTL", "5m"))
	if err != nil {
		return nil, fmt.Errorf("CACHE_TTL invalido (use algo como 30s, 5m, 1h): %w", err)
	}
	cfg.CacheTTL = ttl

	tempo, err := time.ParseDuration(strEnv("TEMPO_MAXIMO_CONSULTA", "10m"))
	if err != nil {
		return nil, fmt.Errorf("TEMPO_MAXIMO_CONSULTA invalido (use algo como 5m, 10m): %w", err)
	}
	cfg.TempoConsulta = tempo

	for _, f := range []struct{ nome, caminho string }{
		{"TLS_CERT_FILE", cfg.TLSCertFile},
		{"TLS_KEY_FILE", cfg.TLSKeyFile},
	} {
		if _, err := os.Stat(f.caminho); err != nil {
			return nil, fmt.Errorf("%s aponta para %q, que nao pode ser lido: %w", f.nome, f.caminho, err)
		}
	}

	return cfg, nil
}

func strEnv(nome, padrao string) string {
	if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
		return v
	}
	return padrao
}

func boolEnv(nome string, padrao bool) bool {
	v := strings.TrimSpace(os.Getenv(nome))
	if v == "" {
		return padrao
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return padrao
	}
	return b
}

func intEnv(nome string, padrao int) int {
	v := strings.TrimSpace(os.Getenv(nome))
	if v == "" {
		return padrao
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return padrao
	}
	return n
}

func listaEnv(nome string, padrao []string) []string {
	v := strings.TrimSpace(os.Getenv(nome))
	if v == "" {
		return padrao
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return padrao
	}
	return out
}
