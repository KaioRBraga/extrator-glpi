#!/usr/bin/env bash
#
# ExtratorGLPI - instalacao no 10.100.0.97
#
# Roda a partir do diretorio descompactado do pacote:
#   sudo bash instalar.sh
#
# O que faz, nesta ordem:
#   1. confere o Docker e o compose, e oferece instalar se faltarem
#   2. para uma instalacao anterior, se houver (atualizacao)
#   3. confere se a porta 8030 esta livre (nao encosta na 8443, de outra app)
#   4. instala em /opt/extratorglpi
#   5. carrega a imagem do arquivo (sem baixar nada da internet)
#   6. sobe o container e espera ficar saudavel
#
# Nao mexe em nginx, systemd, firewall nem nas outras aplicacoes do host.

set -euo pipefail

DESTINO=/opt/extratorglpi
PORTA=8030
IMAGEM_TAR=extratorglpi-imagem.tar.gz
IMAGEM=extratorglpi:latest
# UID do usuario nonroot da imagem distroless.
UID_CONTAINER=65532

vermelho() { printf '\033[31m%s\033[0m\n' "$*"; }
verde()    { printf '\033[32m%s\033[0m\n' "$*"; }
amarelo()  { printf '\033[33m%s\033[0m\n' "$*"; }
passo()    { printf '\n\033[1m==> %s\033[0m\n' "$*"; }

abortar() { vermelho "ERRO: $*"; exit 1; }

# instalar_pacote tenta os nomes na ordem e para no primeiro que existir.
#
# O pacote do compose muda de nome conforme a origem: no Ubuntu e
# docker-compose-v2, no repositorio oficial do Docker e docker-compose-plugin,
# e em instalacoes antigas e o binario docker-compose. Errar o nome faz o apt
# falhar inteiro, sem instalar nem o que existia.
instalar_pacote() {
  local gerenciador="$1"; shift
  local pacote
  for pacote in "$@"; do
    case "$gerenciador" in
      apt) apt-get install -y "$pacote" >/dev/null 2>&1 && { verde "instalado: $pacote"; return 0; } ;;
      dnf) dnf install -y "$pacote"     >/dev/null 2>&1 && { verde "instalado: $pacote"; return 0; } ;;
    esac
    amarelo "pacote indisponivel nesta distro: $pacote"
  done
  return 1
}

gerenciador_de_pacotes() {
  if command -v apt-get >/dev/null 2>&1; then echo apt
  elif command -v dnf >/dev/null 2>&1; then echo dnf
  else echo desconhecido
  fi
}

instalar_docker() {
  local g; g="$(gerenciador_de_pacotes)"
  [[ "$g" != desconhecido ]] || abortar "gerenciador de pacotes nao reconhecido; instale o Docker manualmente"
  [[ "$g" == apt ]] && apt-get update

  instalar_pacote "$g" docker.io docker-ce docker     || abortar "nao consegui instalar o Docker; instale manualmente e rode de novo"
  # Sem systemd (container, por exemplo) o servico e problema de quem instalou.
  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable --now docker || amarelo "nao consegui ligar o servico docker; veja: systemctl status docker"
  else
    amarelo "systemctl ausente; suba o daemon do Docker por conta propria"
  fi
  instalar_compose
}

instalar_compose() {
  local g; g="$(gerenciador_de_pacotes)"
  instalar_pacote "$g" docker-compose-v2 docker-compose-plugin docker-compose     || abortar "nao consegui instalar o docker compose; instale manualmente e rode de novo"
}

perguntar() {
  local pergunta="$1" resposta
  read -r -p "$pergunta [s/N] " resposta
  [[ "$resposta" =~ ^[SsYy]$ ]]
}

# ── 0. Contexto ───────────────────────────────────────────────────────────────
[[ $EUID -eq 0 ]] || abortar "rode com sudo: sudo bash instalar.sh"

ORIGEM="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ORIGEM"

for arquivo in docker-compose.yml .env "$IMAGEM_TAR" certs/server.crt certs/server.key; do
  [[ -e "$arquivo" ]] || abortar "arquivo faltando no pacote: $arquivo"
done

# ── 1. Docker ─────────────────────────────────────────────────────────────────
passo "Conferindo o Docker"
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  verde "Docker presente: $(docker --version)"
else
  amarelo "Docker nao encontrado (ou o daemon nao esta rodando)."
  amarelo "Este host roda DiscMotiva, SFU, Redis e nginx. Instalar o Docker mexe"
  amarelo "na rede do host (cria a bridge docker0 e regras de iptables)."
  if perguntar "Instalar o Docker agora?"; then
    instalar_docker
    verde "Docker instalado: $(docker --version)"
  else
    abortar "instalacao cancelada - o Docker e necessario"
  fi
fi

passo "Conferindo o docker compose"
if docker compose version >/dev/null 2>&1; then
  COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE=(docker-compose)
else
  amarelo "o plugin do compose nao esta instalado"
  if perguntar "Instalar o plugin agora?"; then
    instalar_compose
    if docker compose version >/dev/null 2>&1; then
      COMPOSE=(docker compose)
    elif command -v docker-compose >/dev/null 2>&1; then
      COMPOSE=(docker-compose)
    else
      abortar "o plugin do compose continua ausente; instale manualmente"
    fi
  else
    abortar "instalacao cancelada - o docker compose e necessario"
  fi
fi
verde "compose: $("${COMPOSE[@]}" version | head -1)"

# ── 2. Instalacao anterior ────────────────────────────────────────────────────
# Precisa vir antes da conferencia de porta: numa atualizacao, quem esta
# ocupando a 8030 e o proprio extrator, e isso nao e conflito.
passo "Procurando instalacao anterior"
if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx extratorglpi; then
  amarelo "container extratorglpi encontrado - isto e uma atualizacao"
  if [[ -f "$DESTINO/docker-compose.yml" ]]; then
    (cd "$DESTINO" && "${COMPOSE[@]}" down) || docker rm -f extratorglpi >/dev/null
  else
    docker rm -f extratorglpi >/dev/null
  fi
  verde "instalacao anterior parada"
else
  verde "nenhuma instalacao anterior"
fi

# ── 3. Porta ──────────────────────────────────────────────────────────────────
passo "Conferindo a porta $PORTA"
if ss -ltn 2>/dev/null | grep -qE "[:.]$PORTA\b"; then
  ss -ltnp 2>/dev/null | grep -E "[:.]$PORTA\b" || true
  abortar "a porta $PORTA ja esta em uso. Escolha outra em docker-compose.yml e .env"
fi
verde "porta $PORTA livre"

amarelo "Portas ja usadas neste host (para conferencia):"
ss -ltn 2>/dev/null | awk 'NR>1 {print $4}' | sed 's/.*://' | sort -un | tr '\n' ' ' || true
echo


# ── 4. Arquivos ───────────────────────────────────────────────────────────────
passo "Instalando em $DESTINO"
if [[ -d "$DESTINO" ]]; then
  amarelo "$DESTINO ja existe - sera atualizado (o .env atual e preservado)."
fi
mkdir -p "$DESTINO/certs"

install -m 644 docker-compose.yml "$DESTINO/docker-compose.yml"
install -m 644 certs/server.crt   "$DESTINO/certs/server.crt"
install -m 600 certs/server.key   "$DESTINO/certs/server.key"

# A imagem roda como nonroot (UID 65532 do distroless). Sem este dono, o
# container nao consegue ler a chave e reinicia em laco com
# "open /certs/server.key: permission denied".
chown "$UID_CONTAINER":"$UID_CONTAINER" "$DESTINO/certs/server.crt" "$DESTINO/certs/server.key"
chmod 755 "$DESTINO/certs"

if [[ -f "$DESTINO/.env" ]]; then
  # Configuracao ajustada no servidor nao pode ser sobrescrita em silencio.
  install -m 600 .env "$DESTINO/.env.novo"
  amarelo "ja havia um .env: o novo ficou em $DESTINO/.env.novo (compare e troque se quiser)"
else
  install -m 600 .env "$DESTINO/.env"
fi
verde "arquivos no lugar"

# ── 5. Imagem ─────────────────────────────────────────────────────────────────
passo "Carregando a imagem (sem baixar nada da internet)"
gunzip -c "$IMAGEM_TAR" | docker load
docker image inspect "$IMAGEM" >/dev/null || abortar "imagem $IMAGEM nao ficou disponivel"
verde "imagem carregada"

# ── 6. Subir ──────────────────────────────────────────────────────────────────
passo "Subindo o container"
cd "$DESTINO"
"${COMPOSE[@]}" up -d

passo "Esperando ficar saudavel"
for _ in $(seq 1 30); do
  estado="$(docker inspect -f '{{.State.Health.Status}}' extratorglpi 2>/dev/null || echo desconhecido)"
  [[ "$estado" == "healthy" ]] && break
  sleep 2
done

echo
if [[ "$estado" == "healthy" ]]; then
  verde "=========================================================="
  verde " ExtratorGLPI no ar: https://10.100.0.97:$PORTA"
  verde "=========================================================="
  echo
  curl -sk "https://127.0.0.1:$PORTA/api/health" || true
  echo
else
  vermelho "o container nao ficou saudavel (estado: $estado)"
  echo
  amarelo "Ultimas linhas do log:"
  docker logs --tail 30 extratorglpi || true
  echo
  amarelo "Causas mais comuns:"
  amarelo "  - o GLPI recusa o IP deste servidor: libere 10.100.0.97 no cliente"
  amarelo "    da API (GLPI > Configurar > Geral > API > Intervalo de IPv4)"
  amarelo "  - credencial errada no $DESTINO/.env"
  exit 1
fi

echo "Comandos uteis:"
echo "  cd $DESTINO"
echo "  ${COMPOSE[*]} logs -f          # acompanhar"
echo "  ${COMPOSE[*]} restart          # reiniciar"
echo "  ${COMPOSE[*]} down             # parar e remover"
