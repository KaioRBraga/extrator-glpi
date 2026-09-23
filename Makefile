.PHONY: dev build test front cert docker limpar

## build: compila o front e gera o binario com a SPA embutida
build: front
	go build -trimpath -o extrator ./cmd/extrator

## front: gera web/dist (necessario antes de qualquer go build/test do binario)
front:
	cd web && npm ci --no-audit --no-fund && npm run build

## test: testes do backend
test:
	go test ./...

## cert: certificado autoassinado para desenvolvimento
cert:
	go run ./cmd/gerarcert -saida ./certs -host localhost,127.0.0.1

## dev: sobe o backend; em outro terminal, "cd web && npm run dev"
dev:
	go run ./cmd/extrator

## docker: sobe o servico em container
docker:
	docker compose up --build

limpar:
	rm -rf web/dist extrator extrator.exe
