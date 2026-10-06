#!/usr/bin/env bash
#
# Monta o pacote de deploy do ExtratorGLPI: extratorglpi-deploy.tar.gz na raiz
# do projeto, pronto para o scp no 10.100.0.97.
#
#   bash deploy/gerar-pacote.sh              # constroi a imagem e empacota
#   bash deploy/gerar-pacote.sh --sem-build  # usa a imagem que ja esta no Docker
#
# Roda da raiz do projeto (ou de qualquer lugar: o script se localiza).

set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$RAIZ"

IMAGEM=extratorglpi:latest
PACOTE=extratorglpi-deploy.tar.gz
PREFIXO=extratorglpi-deploy

if [[ "${1:-}" != "--sem-build" ]]; then
  echo "==> Construindo a imagem (front-end + binario Go)"
  docker build -t "$IMAGEM" .
fi

docker image inspect "$IMAGEM" >/dev/null || {
  echo "ERRO: imagem $IMAGEM nao existe no Docker local" >&2
  exit 1
}

echo "==> Salvando a imagem em deploy/extratorglpi-imagem.tar.gz"
docker save "$IMAGEM" | gzip -9 > deploy/extratorglpi-imagem.tar.gz

# O .env e os certificados do servidor nao estao no git (ver .gitignore): quem
# empacota precisa ter os arquivos locais de deploy/.
for arquivo in .env docker-compose.yml instalar.sh diagnostico.sh RUNBOOK.md certs/server.crt certs/server.key; do
  [[ -e "deploy/$arquivo" ]] || { echo "ERRO: falta deploy/$arquivo" >&2; exit 1; }
done

echo "==> Empacotando $PACOTE"
rm -f "$PACOTE"
# --transform poe tudo sob extratorglpi-deploy/, que e o diretorio que o
# RUNBOOK manda entrar depois do tar xzf.
tar czf "$PACOTE" --transform "s,^deploy,$PREFIXO," \
  deploy/.env deploy/docker-compose.yml deploy/instalar.sh \
  deploy/diagnostico.sh deploy/RUNBOOK.md \
  deploy/certs/server.crt deploy/certs/server.key \
  deploy/extratorglpi-imagem.tar.gz

echo
echo "Pacote pronto: $PACOTE ($(du -h "$PACOTE" | cut -f1))"
tar tzf "$PACOTE"
echo
echo "Proximos passos:"
echo "  scp $PACOTE aut@10.100.0.97:/tmp/"
echo "  ssh aut@10.100.0.97"
echo "  cd /tmp && tar xzf $PACOTE && cd $PREFIXO && sudo bash instalar.sh"
