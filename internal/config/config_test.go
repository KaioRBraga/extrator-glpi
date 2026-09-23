package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// prepararTLS cria cert e chave falsos: Load so checa se os arquivos existem.
func prepararTLS(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	crt := filepath.Join(dir, "server.crt")
	key := filepath.Join(dir, "server.key")
	for _, c := range []string{crt, key} {
		if err := os.WriteFile(c, []byte("fake"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return crt, key
}

func ambienteCompleto(t *testing.T) {
	t.Helper()
	crt, key := prepararTLS(t)
	t.Setenv("GLPI_URL", "https://glpi.empresa.com.br")
	t.Setenv("GLPI_APP_TOKEN", "token")
	t.Setenv("GLPI_USUARIO", "robo")
	t.Setenv("GLPI_SENHA", "segredo")
	t.Setenv("TLS_CERT_FILE", crt)
	t.Setenv("TLS_KEY_FILE", key)
}

func TestLoadComAmbienteCompleto(t *testing.T) {
	ambienteCompleto(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != "8443" {
		t.Errorf("porta padrao = %q", cfg.ServerPort)
	}
	if cfg.CacheTTL != 5*time.Minute {
		t.Errorf("TTL padrao = %v", cfg.CacheTTL)
	}
	if !cfg.GLPIVerifySSL {
		t.Error("GLPI_VERIFY_SSL deveria ser true por padrao")
	}
	if len(cfg.MotivoLabels) == 0 || cfg.MotivoLabels[0] != "SOLICITACAO" {
		t.Errorf("labels padrao = %v", cfg.MotivoLabels)
	}
	if cfg.MaxChamados != 5000 {
		t.Errorf("MAX_CHAMADOS padrao = %d", cfg.MaxChamados)
	}
}

// O SDD exige que as informacoes venham so do .env, sem fallback: faltando
// qualquer uma, o servico nao sobe.
func TestLoadRecusaVariavelFaltando(t *testing.T) {
	for _, faltando := range obrigatorias {
		t.Run(faltando, func(t *testing.T) {
			ambienteCompleto(t)
			t.Setenv(faltando, "")

			_, err := Load("")
			if err == nil {
				t.Fatalf("Load aceitou ambiente sem %s", faltando)
			}
			if !strings.Contains(err.Error(), faltando) {
				t.Errorf("erro nao diz qual variavel falta: %v", err)
			}
		})
	}
}

func TestLoadRecusaCertificadoInexistente(t *testing.T) {
	ambienteCompleto(t)
	t.Setenv("TLS_CERT_FILE", filepath.Join(t.TempDir(), "nao-existe.crt"))

	_, err := Load("")
	if err == nil || !strings.Contains(err.Error(), "TLS_CERT_FILE") {
		t.Fatalf("erro = %v", err)
	}
}

func TestLoadLeArquivoEnv(t *testing.T) {
	crt, key := prepararTLS(t)
	dir := t.TempDir()
	caminho := filepath.Join(dir, ".env")
	conteudo := strings.Join([]string{
		"GLPI_URL=https://glpi.interno",
		"GLPI_APP_TOKEN=abc",
		"GLPI_USUARIO=robo",
		"GLPI_SENHA=segredo",
		"TLS_CERT_FILE=" + filepath.ToSlash(crt),
		"TLS_KEY_FILE=" + filepath.ToSlash(key),
		"SERVER_PORT=9443",
		"CACHE_TTL=30s",
		"GLPI_VERIFY_SSL=false",
		"MOTIVO_ABERTURA_LABELS=PROBLEMA, SOLICITACAO",
		"MAX_CHAMADOS=100",
	}, "\n")
	if err := os.WriteFile(caminho, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(caminho)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GLPIURL != "https://glpi.interno" || cfg.ServerPort != "9443" {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.CacheTTL != 30*time.Second {
		t.Errorf("TTL = %v", cfg.CacheTTL)
	}
	if cfg.GLPIVerifySSL {
		t.Error("GLPI_VERIFY_SSL=false deveria desligar a verificacao")
	}
	if len(cfg.MotivoLabels) != 2 || cfg.MotivoLabels[0] != "PROBLEMA" {
		t.Errorf("labels = %v", cfg.MotivoLabels)
	}
	if cfg.MaxChamados != 100 {
		t.Errorf("MAX_CHAMADOS = %d", cfg.MaxChamados)
	}
}

func TestLoadRecusaTTLInvalido(t *testing.T) {
	ambienteCompleto(t)
	t.Setenv("CACHE_TTL", "cinco minutos")

	if _, err := Load(""); err == nil {
		t.Fatal("esperava erro de CACHE_TTL")
	}
}

// Arquivo .env ausente nao e erro: em container as variaveis chegam pelo
// ambiente.
func TestLoadSemArquivoEnv(t *testing.T) {
	ambienteCompleto(t)

	if _, err := Load(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
