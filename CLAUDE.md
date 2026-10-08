# CLAUDE.md

## Proyecto

TPs de Spec-Driven Development. Cada carpeta es un TP (ver `README.md`). El único
código es `greenfield/gcsgrep` (Go).

## Comandos

- Tests unitarios: `cd greenfield/gcsgrep && go test ./...`
- Tests contra GCS real: `go test -tags integration ./integration/...` (necesita
  credenciales; ver `greenfield/README.md`)

## Pipeline SDD

- Para especificar un cambio sobre código existente, usá el skill
  `write-spec-brownfield`. Para revisar una spec, el subagent `spec-reviewer`: quien
  escribe la spec no la aprueba.
- Una spec brownfield no se entrega sin `check-vc-coverage.sh --strict` en 0 y una revisión
  del `spec-reviewer` sin Issues. Las reglas que la cátedra penalizó (FR atómicos, un VC por
  invariante, decisiones con fundamento en código, cero TBD) están en el skill.
- Un test que estaba en verde y pasa a rojo es una regresión: se arregla el código, no
  el test. Un hook bloquea `git commit` con la suite en rojo.

## Commits

- Todo commit que implementa o cambia comportamiento cita los requisitos que toca
  (`FR-n`, `BR-n`, `NFR-n`, `VC-n`) y, si aplica, la iteración del plan.
  Ej.: `Add gzip decompression and size guardrails (FR-12, BR-4, BR-5)`.
- Commits de documentación de spec citan la sección o el hallazgo de revisión.
