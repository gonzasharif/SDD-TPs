# SDD-TPs

Trabajos prácticos del curso de Spec-Driven Development. Por ahora, uno solo:
`gcsgrep` (Lección 1) — la consigna completa está en
[`enunciado.md`](./enunciado.md).

## `gcsgrep`

Un CLI en Go que busca texto dentro de objetos de Google Cloud Storage sin
bajarlos primero a disco.

### Los documentos, en orden de pipeline

| # | Archivo | Paso SDD |
|---|---|---|
| 1 | [`gcsgrep-requirements.md`](./gcsgrep-requirements.md) | Base context refinado (decisiones tomadas, arquitectura) |
| 2 | [`gcsgrep-spec.md`](./gcsgrep-spec.md) | Spec: FRs/BRs/NFRs en Dado/Cuando/Entonces, un VC por cada uno |
| 3 | [`gcsgrep-plan.md`](./gcsgrep-plan.md) | Plan de iteraciones (alcance diferido vive acá, no en la spec) |
| 4 | [`gcsgrep-cobertura-vc.md`](./gcsgrep-cobertura-vc.md) | Evidencia de verificación, acumulada por iteración |

### Correrlo

```bash
cd gcsgrep
go build -o gcsgrep ./cmd/gcsgrep
go test ./...   # tests unitarios
go vet ./...     # chequeo estático
```

```bash
./gcsgrep [-i] [--max N] "PATRÓN" gs://bucket/prefijo
```

Necesita Application Default Credentials (`gcloud auth application-default
login`) — no acepta ninguna otra forma de autenticación (BR-2). No escribe
nunca en GCS (BR-1).

### Estado

Iteración 1 implementada y verificada (ver `gcsgrep-cobertura-vc.md`), con
dos VCs (VC-9/VC-16) marcados con una salvedad explícita: se probaron con
un cliente GCS fake, no contra un objeto real con ACL restringida.
