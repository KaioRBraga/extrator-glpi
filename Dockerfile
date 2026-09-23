# syntax=docker/dockerfile:1

# ── 1. Build do front-end (React + Tailwind) ─────────────────────────────────
FROM node:24-alpine AS front
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ── 2. Build do binario Go, com o front embutido ─────────────────────────────
FROM golang:1.24-alpine AS back
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/embed.go ./web/
COPY --from=front /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /extrator ./cmd/extrator

# ── 3. Imagem final: so o binario, sem shell ─────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=back /extrator /extrator
EXPOSE 8443
USER nonroot:nonroot
# As variaveis vem do env_file do compose; nao ha .env dentro da imagem.
ENTRYPOINT ["/extrator", "-env", ""]
