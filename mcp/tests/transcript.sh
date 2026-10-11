#!/usr/bin/env bash
# transcript.sh — convierte la salida `claude -p --output-format stream-json --verbose` en markdown legible.
# uso: transcript.sh <run.jsonl> "<titulo>" "<prompt>"
set -euo pipefail
echo "# $2"; echo; echo "**Prompt:** $3"; echo
jq -r '
  select(.type=="assistant" or .type=="user")
  | (if .parent_tool_use_id then "subagent" else "principal" end) as $w
  | .message.content
  | if type=="string" then empty else .[] end
  | if .type=="text" and ($w!="x") then "**[\($w)]** " + .text + "\n"
    elif .type=="tool_use" and .name!="SubagentHandback" then "**[\($w)] ▶ \(.name)** `" + (.input|tostring|.[0:600]) + "`\n"
    elif .type=="tool_result" then "<details><summary>resultado</summary>\n\n```\n" + ((.content|if type=="array" then map(.text? // "") | join("") else tostring end)|.[0:1800]) + "\n```\n</details>\n"
    else empty end' "$1"
