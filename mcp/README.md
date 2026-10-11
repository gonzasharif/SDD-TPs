# Lección 4 — `gcsgrep` contra un bucket real, por MCP

> **Estado: parcial.** Hecho y verificado offline: config, hook, skill, subagent, plugin
> (`claude plugin validate` pasa), fixtures y prueba del hook. **Pendiente** (necesita
> crear la service account y el bucket en GCP, ver "Pasos manuales"): evidencias
> `01`, `02`, `03`, `04` (403 de GCS), `05`, `06` (sesión con `--plugin-dir`) y `07`.

## Qué hace y qué paso SDD alimenta

Alimenta **Verificar**. El skill `verify-against-bucket` lanza el subagent `bucket-reader`
(sin Bash ni red; solo `list_objects`, `read_object_metadata`, `read_object_content`),
corre `gcsgrep` sobre el bucket de prueba y contrasta su salida con lo que el server leyó
objeto por objeto. Frena si falta el server, registra procedencia (objeto, *generation*,
fecha, cuenta) y **falla si una *generation* cambió** entre la lectura y el veredicto.

| Pieza | Archivo |
|---|---|
| Config MCP (versión fijada `0.6.0`) | [`.mcp.json`](../.mcp.json) |
| Skill | [`.claude/skills/verify-against-bucket/`](../.claude/skills/verify-against-bucket/SKILL.md) |
| Subagent lector | [`.claude/agents/bucket-reader.md`](../.claude/agents/bucket-reader.md) |
| Hook allowlist | [`.claude/hooks/gcs-allowlist.sh`](../.claude/hooks/gcs-allowlist.sh) + [`settings.json`](../.claude/settings.json) |
| Plugin | [`plugin/gcsgrep-sdd/`](../plugin/gcsgrep-sdd/.claude-plugin/plugin.json) |
| Fixtures sintéticos + hostil | [`fixtures/bucket/`](fixtures/bucket), subida con [`fixtures/setup-bucket.sh`](fixtures/setup-bucket.sh) |

## Instalar el plugin

```bash
claude plugin validate plugin/gcsgrep-sdd
claude --plugin-dir plugin/gcsgrep-sdd     # pide project_id y bucket_name (userConfig)
```

Credencial siempre por ADC, nunca en el paquete:
`gcloud auth application-default login --impersonate-service-account=gcsgrep-mcp-reader@<proyecto>.iam.gserviceaccount.com`.

## Pasos manuales (los hace una persona con permisos de IAM)

```bash
P=gcsgrep-sdd-e4c0ad; B=gcsgrep-mcp-e4c0ad
gcloud storage buckets create gs://$B --project=$P --location=us-central1 --uniform-bucket-level-access
gcloud iam service-accounts create gcsgrep-mcp-reader --project=$P
SA=gcsgrep-mcp-reader@$P.iam.gserviceaccount.com
gcloud storage buckets add-iam-policy-binding gs://$B --member=serviceAccount:$SA --role=roles/storage.objectViewer
gcloud iam service-accounts add-iam-policy-binding $SA --member=user:<tu-mail> --role=roles/iam.serviceAccountTokenCreator
mcp/fixtures/setup-bucket.sh $B
gcloud auth application-default login --impersonate-service-account=$SA
```

## Nota de seguridad

1. **Server:** `@google-cloud/storage-mcp@0.6.0` (fijada en `.mcp.json` y en el plugin),
   contra `gs://gcsgrep-mcp-e4c0ad` (proyecto de prueba `gcsgrep-sdd-e4c0ad`).
2. **Modo:** sin `--enable-destructive-tools` ("safe mode"). **No es solo lectura**:
   [`04a-tools-list-modo-seguro.txt`](evidencia/04a-tools-list-modo-seguro.txt) muestra 15
   tools, entre ellas `create_bucket`, `write_object_safe`, `upload_object_safe`,
   `copy_object_safe` (en 0.6.0 se llaman `*_safe`, no `*_new`). `delete_object` no existe.
3. **Credencial:** service account `gcsgrep-mcp-reader` con `roles/storage.objectViewer`
   **sobre el bucket**, no el proyecto. Vive en ADC impersonado; ninguna key en el repo.
4. **Datos:** solo fixtures sintéticos (`mcp/fixtures/bucket/`). El hostil apunta a `recolector.invalid`.
5. **Peor caso** (escritura/borrado), en orden de corte: (1) hook allowlist veta toda tool
   fuera de las 3 de lectura —probado offline: [`04b`](evidencia/04b-hook-allowlist-offline.txt);
   (2) modo del server: `delete_object` no existe —[`04a`](evidencia/04a-tools-list-modo-seguro.txt);
   (3) credencial: GCS responde `403` a `cp`/`rm` con la SA —**pendiente** (`04-muro`).
   La capa 3 es la que garantiza; las otras dos son defensa en profundidad.
6. **Trifecta:** la sesión principal tiene Bash+red (y ADC en la máquina) y lee contenido
   no confiable. La pata cortada es la **exposición del contenido no confiable a una sesión
   con Bash**: lo lee `bucket-reader`, que no tiene Bash ni red y devuelve solo una tabla.
   Residual: la sesión principal igual corre `gcsgrep` con la ADC impersonada (solo lectura).
7. **Costo de contexto:** **pendiente** (`07-contexto`, `/context`).
