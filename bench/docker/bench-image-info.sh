#!/bin/sh
set -eu

agent_version="$($BENCH_AGENT --version 2>&1 | head -n 1)"
glab_version="$(GLAB_CHECK_UPDATE=false glab --version 2>&1 | head -n 1)"

printf 'agent=%s\n' "$BENCH_AGENT"
printf 'agent_version=%s\n' "$agent_version"
printf 'gl_version=source-build\n'
printf 'gl_sha256=%s\n' "$(sha256sum /usr/local/bin/gl | cut -d ' ' -f 1)"
printf 'gl_axi_version=source-build\n'
printf 'gl_axi_sha256=%s\n' "$(sha256sum /usr/local/bin/gl-axi | cut -d ' ' -f 1)"
printf 'glab_version=%s\n' "$glab_version"
printf 'glab_sha256=%s\n' "$(sha256sum /usr/local/bin/glab | cut -d ' ' -f 1)"
