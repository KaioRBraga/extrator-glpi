// Comando gerarcert cria um certificado TLS autoassinado para desenvolvimento.
//
// Em producao use o certificado da CA interna; este aqui existe so para subir
// o servico em HTTPS na maquina local sem depender do openssl.
//
//	go run ./cmd/gerarcert -saida ./certs -host localhost,127.0.0.1
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	saida := flag.String("saida", "certs", "diretorio onde gravar server.crt e server.key")
	hosts := flag.String("host", "localhost,127.0.0.1", "nomes e IPs cobertos pelo certificado")
	dias := flag.Int("dias", 825, "validade em dias")
	flag.Parse()

	if err := gerar(*saida, *hosts, *dias); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func gerar(saida, hosts string, dias int) error {
	if err := os.MkdirAll(saida, 0o755); err != nil {
		return err
	}

	chave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serie, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	modelo := x509.Certificate{
		SerialNumber:          serie,
		Subject:               pkix.Name{Organization: []string{"ExtratorGLPI"}, CommonName: "extratorglpi"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(0, 0, dias),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range strings.Split(hosts, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if ip := net.ParseIP(h); ip != nil {
			modelo.IPAddresses = append(modelo.IPAddresses, ip)
		} else {
			modelo.DNSNames = append(modelo.DNSNames, h)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, &modelo, &modelo, &chave.PublicKey, chave)
	if err != nil {
		return err
	}

	caminhoCrt := filepath.Join(saida, "server.crt")
	if err := escreverPEM(caminhoCrt, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}

	derChave, err := x509.MarshalECPrivateKey(chave)
	if err != nil {
		return err
	}
	caminhoKey := filepath.Join(saida, "server.key")
	if err := escreverPEM(caminhoKey, "EC PRIVATE KEY", derChave, 0o600); err != nil {
		return err
	}

	fmt.Printf("certificado gerado:\n  %s\n  %s\nvalido ate %s\n",
		caminhoCrt, caminhoKey, modelo.NotAfter.Format("02/01/2006"))
	return nil
}

func escreverPEM(caminho, tipo string, der []byte, modo os.FileMode) error {
	arquivo, err := os.OpenFile(caminho, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, modo)
	if err != nil {
		return err
	}
	defer arquivo.Close()
	return pem.Encode(arquivo, &pem.Block{Type: tipo, Bytes: der})
}
