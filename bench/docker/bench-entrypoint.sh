#!/bin/sh
set -eu

mkdir -p "$HOME/.codex" "$HOME/.claude" "$HOME/.config/glab-cli"

if [ -f /run/secrets/codex-auth.json ]; then
  cp /run/secrets/codex-auth.json "$HOME/.codex/auth.json"
  chmod 0600 "$HOME/.codex/auth.json"
fi

exec "$@"
