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
./gcsgrep [-i] [-l | -c] [-j N] [--max N] [--max-object-size SIZE] \
          [--max-total-size SIZE] [--max-line-size SIZE] "PATRÓN" gs://bucket/prefijo
```

Los flags van antes del patrón. `-l` lista solo los objetos con match, `-c`
cuenta las líneas que matchean por objeto. Los objetos gzip se descomprimen al
vuelo. Guardrails por defecto: 1000 objetos (`--max`), 250 MiB por objeto y
2 GiB por corrida (los tamaños aceptan `KiB`/`MiB`/`GiB`; `0` deshabilita).

Necesita Application Default Credentials (`gcloud auth application-default
login`) — no acepta ninguna otra forma de autenticación (BR-2). No escribe
nunca en GCS (BR-1).

### Estado

- **Iteración 1:** implementada y verificada contra GCS real (ver
  `gcsgrep-cobertura-vc.md`), con dos VCs (VC-9/VC-16) marcados con una
  salvedad explícita: se probaron con un cliente GCS fake, no contra un objeto
  real con ACL restringida.
- **Iteración 2** (`-l`/`-c`, gzip, guardrails de tamaño, reintentos, color y
  progreso): implementada; todos sus VCs pasan en tests unitarios y con el
  binario real contra un emulador local de GCS, y contra GCS real todos salvo
  VC-23 (reintentos ante 503, que GCS no permite provocar a pedido; cubierto
  por tests y emulador). Ver nota 3 de la tabla de cobertura.
- **Iteración 3** (concurrencia, rama `iteration-3`): implementada
  (`--concurrency N` / `-j N`, worker pool, `Budget` y `Writer` thread-safe) y
  verificada con tests unitarios bajo `-race` y contra GCS real (VC-14/20, VC-19 y
  la regresión con `-j 8`). VC-13 medido contra GCS real: entre 3,6x y 5,7x más
  rápido con `-j 8` sobre 30 objetos, en tres corridas (varía con la red).
  **Pendiente:** el benchmark de VC-21 (el umbral de ≥ 15 objetos/seg no se
  alcanza desde el entorno de prueba, limitado por la red), VC-3 con TTY real y
  VC-22. Los tests de integración contra GCS real están en `gcsgrep/integration/`
  (`go test -tags integration ./integration/`). Ver la sección "Iteración 3" de
  `gcsgrep-cobertura-vc.md`.
