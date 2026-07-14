#!/bin/sh
set -eu

mkdir -p "$HOME/.codex" "$HOME/.claude" "$HOME/.config/glab-cli"

if [ -f /run/secrets/benchmark.env ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      *=*)
        key=${line%%=*}
        value=${line#*=}
        ;;
      *)
        echo "invalid benchmark credential entry" >&2
        exit 1
        ;;
    esac
    case "$key" in
      GITLAB_TOKEN|CODEX_ACCESS_TOKEN|CODEX_API_KEY|CLAUDE_CODE_OAUTH_TOKEN|ANTHROPIC_API_KEY)
        export "$key=$value"
        ;;
      *)
        echo "unsupported benchmark credential key: $key" >&2
        exit 1
        ;;
    esac
  done < /run/secrets/benchmark.env
  unset line key value
fi

if [ -f /run/secrets/codex-auth.json ]; then
  cp /run/secrets/codex-auth.json "$HOME/.codex/auth.json"
  chmod 0600 "$HOME/.codex/auth.json"
fi

exec "$@"
