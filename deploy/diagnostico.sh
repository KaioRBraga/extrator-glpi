#!/usr/bin/env bash
# Compara o acesso ao GLPI a partir do host e de dentro do container.
# Nao imprime senha nem token: so status HTTP e o inicio do corpo.
set -uo pipefail

ENV=/opt/extratorglpi/.env
[[ -r "$ENV" ]] || { echo "nao consigo ler $ENV (use sudo)"; exit 1; }

URL=$(grep -E '^GLPI_URL=' "$ENV" | cut -d= -f2-)
TOKEN=$(grep -E '^GLPI_APP_TOKEN=' "$ENV" | cut -d= -f2-)
USUARIO=$(grep -E '^GLPI_USUARIO=' "$ENV" | cut -d= -f2-)
SENHA=$(grep -E '^GLPI_SENHA=' "$ENV" | cut -d= -f2-)
HOST=$(echo "$URL" | sed -E 's#^https?://##; s#/.*##; s#:.*##')

echo "=============================================="
echo "GLPI_URL : $URL"
echo "host     : $HOST"
echo "=============================================="

echo
echo "--- 1. DNS no host ---"
getent hosts "$HOST" || echo "(nao resolveu)"

echo
echo "--- 2. DNS dentro do container ---"
docker run --rm --network container:extratorglpi busybox nslookup "$HOST" 2>/dev/null | tail -5 \
  || docker exec extratorglpi sh -c "true" 2>/dev/null \
  || echo "(sem shell no container - imagem distroless; veja o teste 4)"

echo
echo "--- 3. initSession a partir do HOST ---"
curl -s -o /tmp/glpi_host.txt -w "HTTP %{http_code} | tipo=%{content_type} | tamanho=%{size_download}\n" \
  -u "$USUARIO:$SENHA" -H "App-Token: $TOKEN" -H "Content-Type: application/json" \
  "$URL/apirest.php/initSession"
echo "inicio do corpo:"
head -c 200 /tmp/glpi_host.txt | tr -d '\r'; echo

echo
echo "--- 4. initSession a partir da REDE DO CONTAINER ---"
docker run --rm --network container:extratorglpi curlimages/curl:latest \
  -s -o /dev/null -w "HTTP %{http_code} | tipo=%{content_type}\n" \
  -u "$USUARIO:$SENHA" -H "App-Token: $TOKEN" -H "Content-Type: application/json" \
  "$URL/apirest.php/initSession" 2>/dev/null \
  || echo "(nao consegui rodar; a imagem curlimages/curl precisa de internet)"

echo
echo "--- 5. IP de saida visto pelo GLPI (host x container) ---"
echo -n "host      : "; ip route get "$(getent hosts "$HOST" | awk '{print $1}' | head -1)" 2>/dev/null | sed -n 's/.*src \([0-9.]*\).*/\1/p' | head -1
echo -n "container : "; docker exec extratorglpi true 2>/dev/null && echo "(sem shell)" || echo "(distroless, sem shell - use o teste 4)"

rm -f /tmp/glpi_host.txt
