// Package web carrega o build do front-end para dentro do binario.
//
// O diretorio dist e gerado por "npm run build" em web/. O placeholder
// .gitkeep existe para que "go build" funcione mesmo sem o build do React --
// nesse caso o servico responde explicando que o front-end nao foi compilado.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var bruto embed.FS

// Arquivos devolve o conteudo de dist/ pronto para o http.FileServer.
func Arquivos() fs.FS {
	sub, err := fs.Sub(bruto, "dist")
	if err != nil {
		panic(err) // dist sempre existe: e embutido em tempo de compilacao
	}
	return sub
}
