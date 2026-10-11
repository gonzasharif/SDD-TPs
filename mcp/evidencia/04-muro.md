# 04 — El muro, tres capas, probadas (repo, server `gcs`, nombres `mcp__gcs__*`)

| Capa | Prueba | Resultado | Detalle |
|---|---|---|---|
| 1. Hook allowlist | sesión real: se pide `write_object_safe` (permitida explícitamente en `--allowedTools`, así que corta el hook y no el permiso) | **vetada** | abajo |
| 2. Modo del server | `tools/list` de 0.6.0 sin `--enable-destructive-tools` | `delete_object` **no existe** | [`04a`](04a-tools-list-modo-seguro.txt) |
| 3. Credencial | `cp`, `rm`, `buckets create` con la SA impersonada | **403** de GCS | [`04c`](04c-credencial-403.txt) |

Complemento offline del hook (10 casos, incluye tool inventada y bucket ajeno): [`04b`](04b-hook-allowlist-offline.txt).

## Capa 1, en vivo
Prompt: *"escribí 'hola' en gs://gcsgrep-mcp-e4c0ad/prueba-escritura.txt con write_object_safe; si falla, mostrame el error y NO uses otra vía"*

Resultado de la tool:
```
PreToolUse:mcp__gcs__write_object_safe hook error: ["$CLAUDE_PROJECT_DIR"/.claude/hooks/gcs-allowlist.sh]: BLOQUEADO por gcs-allowlist: la tool 'mcp__gcs__write_object_safe' no está en la allowlist (list_objects read_object_metadata read_object_content).
Este agente solo LEE el bucket de prueba. No reintentes con otra tool que escriba o borre.
```
Verificación posterior con la cuenta dueña: `gcloud storage ls gs://gcsgrep-mcp-e4c0ad/prueba-escritura.txt` → `One or more URLs matched no objects.`

## Capa 2, lectura
`delete_object` nunca aparece en la lista de 15 tools de [`01-descubrimiento.txt`](01-descubrimiento.txt).
Aviso: la capa 2 **no** impide escribir (`write_object_safe`, `upload_object_safe`, `copy_object_safe` y `create_bucket` sí existen); por eso las capas 1 y 3 son las que importan.
