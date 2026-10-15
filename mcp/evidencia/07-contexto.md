# 07 — /context con el server gcs (@0.6.0) conectado, vs. sin server
# 'MCP tools (deferred)': las definiciones se cargan bajo demanda (ToolSearch), no en cada turno.

--- CON server
**Tokens:** 15.6k / 1m (2%)
| MCP tools (deferred) | 3.2k | 0.3% |
| Custom agents | 3.3k | 0.3% |
| Skills | 9.4k | 0.9% |
--- SIN server
**Tokens:** 15.6k / 1m (2%)
| Custom agents | 3.3k | 0.3% |
| Skills | 9.4k | 0.9% |

--- Detalle por tool (CON server)
| mcp__gcs__check_iam_permissions | gcs | 165 |
| mcp__gcs__copy_object_safe | gcs | 279 |
| mcp__gcs__create_bucket | gcs | 348 |
| mcp__gcs__execute_insights_query | gcs | 236 |
| mcp__gcs__get_bucket_location | gcs | 123 |
| mcp__gcs__get_bucket_metadata | gcs | 123 |
| mcp__gcs__get_metadata_table_schema | gcs | 273 |
| mcp__gcs__list_buckets | gcs | 111 |
| mcp__gcs__list_insights_configs | gcs | 142 |
| mcp__gcs__list_objects | gcs | 489 |
| mcp__gcs__read_object_content | gcs | 154 |
| mcp__gcs__read_object_metadata | gcs | 153 |
| mcp__gcs__upload_object_safe | gcs | 249 |
| mcp__gcs__view_iam_policy | gcs | 119 |
| mcp__gcs__write_object_safe | gcs | 239 |
