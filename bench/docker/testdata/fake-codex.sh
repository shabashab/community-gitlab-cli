#!/bin/sh
set -eu
prompt="$(cat)"

case "$prompt" in
  *require-auth*)
    if [ ! -s "$HOME/.codex/auth.json" ]; then
      printf 'account auth was not staged\n' >&2
      exit 12
    fi
    ;;
esac

case "$prompt" in
  *timeout*)
    sleep 30
    ;;
  *crash*)
    printf 'fake crash\n' >&2
    exit 9
    ;;
  *oom*)
    value=xxxxxxxxxxxxxxxx
    while :; do
      value="${value}${value}"
    done
    ;;
esac

if [ -e /host-home ] || [ -e /var/run/docker.sock ] || [ -e /results ]; then
  printf 'forbidden host path visible\n' >&2
  exit 10
fi

if [ -e "$HOME/trial-marker" ] || [ -e /workspace/trial-marker ]; then
  printf 'state leaked from another trial\n' >&2
  exit 11
fi

printf 'home-marker\n' > "$HOME/trial-marker"
printf 'workspace-marker\n' > /workspace/trial-marker
printf 'fake stderr\n' >&2
printf '%s\n' '{"type":"item.completed","item":{"type":"command_execution","command":"gl-axi mr list"}}'
printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"benchmark fake complete"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":3,"reasoning_output_tokens":1}}'
