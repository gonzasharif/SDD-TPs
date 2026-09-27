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
| 1 | [`gcsgrep-requirements.md`](./gcsgrep-requirements.md) | Base context refinado: requerimientos del dominio y glosario (lenguaje ubicuo) |
| 2 | [`gcsgrep-design.md`](./gcsgrep-design.md) | Diseño: modelo de dominio, decisiones, arquitectura |
| 3 | [`gcsgrep-spec.md`](./gcsgrep-spec.md) | Spec: FRs/BRs/NFRs en Dado/Cuando/Entonces, un VC por cada uno |
| 4 | [`gcsgrep-plan.md`](./gcsgrep-plan.md) | Plan de iteraciones (alcance diferido vive acá, no en la spec) |
| 5 | [`gcsgrep-cobertura-vc.md`](./gcsgrep-cobertura-vc.md) | Evidencia de verificación, acumulada por iteración |

### Correrlo

```bash
cd gcsgrep
go build -o gcsgrep ./cmd/gcsgrep
go vet ./...     # chequeo estático
go test ./...    # tests unitarios (incluye los chequeos de VC-1.2 y VC-15.2)
```

```bash
./gcsgrep [-i] [-n] [--max N] PATRÓN gs://bucket/prefijo
```

El patrón es una expresión regular RE2. Necesita Application Default
Credentials (`gcloud auth application-default login`) — no acepta ninguna
otra forma de autenticación. No escribe nunca en GCS (BR-1).

Los VCs contra GCS real son tests de Go con build tag `integration` (ver
`gcsgrep/integration/doc.go`); los datos que usan se crean con
`gcsgrep/testdata/setup-testdata.sh`.

### Estado

La spec se corrigió después de la revisión de la cátedra (FRs atómicos,
casos de falla nuevos, VCs con observables literales, NFRs congelados) y la
Iteración 1 se re-verificó contra ella: pasan sus 41 VCs, 30 de ellos contra
GCS real y 3 como mediciones de rendimiento y memoria.
Detalle en `gcsgrep-cobertura-vc.md`, sección "Iteración 1 — re-verificación
contra la spec corregida".
