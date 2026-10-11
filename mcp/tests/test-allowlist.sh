#!/usr/bin/env bash
# test-allowlist.sh — prueba offline de la capa 1 (hook). Uso: test-allowlist.sh <prefijo> <script>
#   ej.: test-allowlist.sh mcp__gcs__ .claude/hooks/gcs-allowlist.sh
#        test-allowlist.sh mcp__plugin_gcsgrep-sdd_gcs__ plugin/gcsgrep-sdd/hooks/gcs-allowlist.sh
set -u
P="$1"; S="$2"; fail=0
t() { # nombre-tool, args-json, exit-esperado
  out="$(printf '{"tool_name":"%s%s","tool_input":%s}' "$P" "$1" "$2" | "$S" 2>&1)"; rc=$?
  r=OK; [ "$rc" -eq "$3" ] || { r=FALLA; fail=1; }
  printf '%-6s exit=%s (esperado %s)  %s%s\n' "$r" "$rc" "$3" "$P" "$1"
  [ "$rc" -ne 0 ] && printf '         stderr: %s\n' "$(echo "$out" | head -1)"
}
B=gcsgrep-mcp-e4c0ad
t list_objects            "{\"bucket_name\":\"$B\"}" 0
t read_object_metadata    "{\"bucket_name\":\"$B\",\"object_name\":\"logs/app1.log\"}" 0
t read_object_content     "{\"bucket_name\":\"$B\",\"object_name\":\"logs/app1.log\"}" 0
t write_object_safe        "{\"bucket_name\":\"$B\",\"object_name\":\"x\",\"content\":\"x\"}" 2
t upload_object_safe      "{\"bucket_name\":\"$B\"}" 2
t copy_object_safe        "{\"bucket_name\":\"$B\"}" 2
t create_bucket           "{\"bucket_name\":\"otro\"}" 2
t delete_object           "{\"bucket_name\":\"$B\",\"object_name\":\"x\"}" 2
t tool_nueva_de_v9        "{}" 2
t read_object_content     "{\"bucket_name\":\"un-bucket-real-de-prod\",\"object_name\":\"x\"}" 2
exit $fail
