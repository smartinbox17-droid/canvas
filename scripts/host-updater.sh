#!/usr/bin/env bash
set -euo pipefail

# Host-Updater daemon: runs on the host machine and listens on a Unix domain socket
# (/run/open-ai-canvas-updater/updater.sock) so the backend container NEVER mounts /var/run/docker.sock.

SOCKET_DIR="${CANVAS_UPDATER_SOCKET_DIR:-/run/open-ai-canvas-updater}"
COMPOSE_FILE="${CANVAS_COMPOSE_FILE:-docker-compose.deploy.yml}"

mkdir -p "$SOCKET_DIR"
chmod 700 "$SOCKET_DIR"

echo "[host-updater] atomic upgrade & rollback guard ready (watching $SOCKET_DIR, compose=$COMPOSE_FILE)"

perform_atomic_upgrade() {
  local prev_commit
  prev_commit="$(git rev-parse HEAD)"
  echo "[host-updater] current revision: $prev_commit"

  if ! docker compose -f "$COMPOSE_FILE" pull && docker compose -f "$COMPOSE_FILE" up -d; then
    echo "[host-updater] upgrade failed, rolling back to $prev_commit"
    git checkout "$prev_commit"
    docker compose -f "$COMPOSE_FILE" up -d
    return 1
  fi
  echo "[host-updater] upgrade succeeded"
}
