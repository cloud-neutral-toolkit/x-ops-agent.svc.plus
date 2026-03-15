#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="${1:-}"
DEPLOY_MODE="${2:-}"
ACTION="${3:-deploy}"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/deploy/docker-compose.yml}"
CLOUD_RUN_REGION="${CLOUD_RUN_REGION:-europe-west1}"
PROCESS_PORT="${PROCESS_PORT:-18084}"
DOCKER_PORT="${DOCKER_PORT:-8080}"
CADDY_CONF_DIR="${CADDY_CONF_DIR:-/etc/caddy/conf.d}"
CADDY_CONF_PATH="${CADDY_CONF_DIR}/${PROJECT_NAME}.conf"
SYSTEMD_UNIT_NAME="$(printf '%s' "${PROJECT_NAME:-x-ops-agent.svc.plus}" | tr '[:upper:]' '[:lower:]' | tr '.' '-')"
SYSTEMD_UNIT_PATH="/etc/systemd/system/${SYSTEMD_UNIT_NAME}.service"
INSTALL_BIN_PATH="${INSTALL_BIN_PATH:-/usr/local/bin/${SYSTEMD_UNIT_NAME}}"
BUILD_GOFLAGS="${BUILD_GOFLAGS:--p=1}"
BUILD_GOMAXPROCS="${BUILD_GOMAXPROCS:-1}"

usage() {
  cat <<'EOF'
Usage:
  scripts/setup.sh <project-name> <process|docker> [deploy|uninstall]
  scripts/setup.sh <project-name> cloud-run [gcloud args...]

Examples:
  scripts/setup.sh x-ops-agent.svc.plus process
  scripts/setup.sh x-ops-agent.svc.plus process uninstall
  scripts/setup.sh x-ops-agent.svc.plus docker
  scripts/setup.sh x-ops-agent.svc.plus docker uninstall
  scripts/setup.sh x-ops-agent.svc.plus cloud-run --set-env-vars="OPENCLAW_GATEWAY_URL=wss://...,OPENCLAW_GATEWAY_TOKEN=...,AI_GATEWAY_URL=...,AI_GATEWAY_API_KEY=..."

Environment overrides:
  ENV_FILE=.env
  COMPOSE_FILE=deploy/docker-compose.yml
  CLOUD_RUN_REGION=europe-west1
  PROCESS_PORT=18084
  DOCKER_PORT=8080
  CADDY_CONF_DIR=/etc/caddy/conf.d
  BUILD_GOFLAGS=-p=1
  BUILD_GOMAXPROCS=1
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

ensure_project_name() {
  if [[ -z "$PROJECT_NAME" ]]; then
    echo "Project name is required." >&2
    usage
    exit 1
  fi
}

service_name() {
  printf '%s' "$PROJECT_NAME" | tr '[:upper:]' '[:lower:]' | tr '.' '-'
}

run_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
    return
  fi
  require_cmd sudo
  sudo "$@"
}

reload_caddy() {
  if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files caddy.service >/dev/null 2>&1; then
    run_root systemctl reload caddy || run_root systemctl restart caddy
    return
  fi
  if command -v caddy >/dev/null 2>&1; then
    run_root caddy reload --config /etc/caddy/Caddyfile
  fi
}

write_caddy_config() {
  local upstream="$1"
  run_root mkdir -p "$CADDY_CONF_DIR"
  run_root tee "$CADDY_CONF_PATH" >/dev/null <<EOF
${PROJECT_NAME} {
    encode gzip zstd
    reverse_proxy ${upstream}
}
EOF
  reload_caddy
}

remove_caddy_config() {
  if [[ -f "$CADDY_CONF_PATH" ]]; then
    run_root rm -f "$CADDY_CONF_PATH"
    reload_caddy
  fi
}

write_systemd_unit() {
  run_root tee "$SYSTEMD_UNIT_PATH" >/dev/null <<EOF
[Unit]
Description=${PROJECT_NAME} process deployment
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${ROOT_DIR}
ExecStart=${INSTALL_BIN_PATH} --mode ops --env-file ${ENV_FILE}
Restart=always
RestartSec=5
Environment=OPS_HTTP_ADDR=127.0.0.1:${PROCESS_PORT}
Environment=LISTEN_ADDR=127.0.0.1:${PROCESS_PORT}

[Install]
WantedBy=multi-user.target
EOF
  run_root systemctl daemon-reload
}

install_process_mode() {
  require_cmd go
  if [[ ! -f "$ENV_FILE" ]]; then
    echo "Env file not found: $ENV_FILE" >&2
    echo "Create it from .env.example before running process mode." >&2
    exit 1
  fi

  cd "$ROOT_DIR"
  env GOFLAGS="$BUILD_GOFLAGS" GOMAXPROCS="$BUILD_GOMAXPROCS" go build -o "$INSTALL_BIN_PATH" ./cmd/agent
  write_systemd_unit
  run_root systemctl enable --now "$(basename "$SYSTEMD_UNIT_PATH")"
  write_caddy_config "127.0.0.1:${PROCESS_PORT}"
  run_root systemctl status --no-pager "$(basename "$SYSTEMD_UNIT_PATH")"
}

uninstall_process_mode() {
  if command -v systemctl >/dev/null 2>&1; then
    run_root systemctl disable --now "$(basename "$SYSTEMD_UNIT_PATH")" 2>/dev/null || true
    run_root rm -f "$SYSTEMD_UNIT_PATH"
    run_root systemctl daemon-reload
  fi
  run_root rm -f "$INSTALL_BIN_PATH"
  remove_caddy_config
}

install_docker_mode() {
  require_cmd docker
  if [[ ! -f "$COMPOSE_FILE" ]]; then
    echo "Compose file not found: $COMPOSE_FILE" >&2
    exit 1
  fi

  cd "$ROOT_DIR"
  docker compose -f "$COMPOSE_FILE" up -d --build
  write_caddy_config "127.0.0.1:${DOCKER_PORT}"
  docker compose -f "$COMPOSE_FILE" ps
}

uninstall_docker_mode() {
  require_cmd docker
  if [[ -f "$COMPOSE_FILE" ]]; then
    cd "$ROOT_DIR"
    docker compose -f "$COMPOSE_FILE" down -v --remove-orphans
  fi
  remove_caddy_config
}

run_cloud_run_mode() {
  require_cmd gcloud
  cd "$ROOT_DIR"
  gcloud run deploy "$(service_name)" \
    --source . \
    --region "$CLOUD_RUN_REGION" \
    "${@:3}"
}

ensure_project_name

case "$DEPLOY_MODE" in
  process)
    case "$ACTION" in
      deploy)
        install_process_mode
        ;;
      uninstall)
        uninstall_process_mode
        ;;
      *)
        echo "Unsupported process action: $ACTION" >&2
        usage
        exit 1
        ;;
    esac
    ;;
  docker)
    case "$ACTION" in
      deploy)
        install_docker_mode
        ;;
      uninstall)
        uninstall_docker_mode
        ;;
      *)
        echo "Unsupported docker action: $ACTION" >&2
        usage
        exit 1
        ;;
    esac
    ;;
  cloud-run)
    run_cloud_run_mode "$@"
    ;;
  ""|-h|--help|help)
    usage
    ;;
  *)
    echo "Unsupported deploy mode: $DEPLOY_MODE" >&2
    usage
    exit 1
    ;;
esac
