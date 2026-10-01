# Greenfield — `gcsgrep` (Lección 1)

TP de Spec-Driven Development sobre un proyecto desde cero: `gcsgrep`, un CLI en Go que
busca texto dentro de objetos de Google Cloud Storage **sin bajarlos primero a disco**.
La consigna completa está en [`enunciado.md`](enunciado.md).

```
gcsgrep "timeout" gs://logs/
```

## Qué hay en esta carpeta

### Los documentos, en orden de pipeline

| # | Archivo | Paso SDD |
|---|---|---|
| 0 | [`enunciado.md`](enunciado.md) | La consigna de la tarea |
| 1 | [`gcsgrep-requirements.md`](gcsgrep-requirements.md) | Base context refinado: requerimientos del dominio y glosario (lenguaje ubicuo) |
| 2 | [`gcsgrep-design.md`](gcsgrep-design.md) | Diseño: modelo de dominio, decisiones, arquitectura |
| 3 | [`gcsgrep-spec.md`](gcsgrep-spec.md) | Spec: FRs/BRs/NFRs en Dado/Cuando/Entonces, un VC por cada uno |
| 4 | [`gcsgrep-plan.md`](gcsgrep-plan.md) | Plan de iteraciones (el alcance diferido vive acá, no en la spec) |
| 5 | [`gcsgrep-cobertura-vc.md`](gcsgrep-cobertura-vc.md) | Evidencia de verificación, acumulada por iteración |

### El código: `gcsgrep/`

Un módulo de Go (`gcsgrep/go.mod`) con una carpeta por responsabilidad. `app` conecta
`cli`, `match`, `gcsclient` y `scanner`; `scanner` usa `reader` para cada objeto y `output`
para escribir los resultados.

| Ruta | Responsabilidad |
|---|---|
| `cmd/gcsgrep/main.go` | Punto de entrada |
| `internal/app` | Conecta los módulos; vive fuera de `main` para poder testear todo el camino |
| `internal/cli` | Parsea los argumentos a una estructura |
| `internal/match` | Aplica el patrón a una línea de texto; sin I/O |
| `internal/gcsclient` | Capa anticorrupción sobre el SDK de GCS; es el único código que lo importa. `gcsclienttest/` trae un cliente falso para los tests |
| `internal/scanner` | Lista los objetos de una corrida y aplica el guardrail de cantidad de objetos |
| `internal/reader` | Procesa un objeto por streaming, sin leerlo entero en memoria; detecta binarios |
| `internal/output` | Escribe resultados (stdout) y avisos (stderr) |
| `internal/invariants` | Sin código de producción: tests que verifican propiedades estructurales de todo el código |
| `integration/` | Tests de los VCs contra GCS real (build tag `integration`) |
| `testdata/setup-testdata.sh` | Crea en GCS los datos de prueba y las cuentas de servicio |

## Correrlo

Desde esta carpeta:

```bash
cd gcsgrep
go build -o gcsgrep ./cmd/gcsgrep
go vet ./...     # chequeo estático
go test ./...    # tests unitarios (incluye los chequeos de VC-1.2 y VC-15.2)
```

```bash
./gcsgrep [-i] [-n] [--max N] PATRÓN gs://bucket/prefijo
```

El patrón es una expresión regular RE2. Necesita Application Default Credentials
(`gcloud auth application-default login`): no acepta ninguna otra forma de autenticación.
No escribe nunca en GCS (BR-1).

Los VCs contra GCS real son tests de Go con build tag `integration` (ver
`gcsgrep/integration/doc.go`). Los datos que usan se crean con
`gcsgrep/testdata/setup-testdata.sh`. Los valores reales de tu proyecto van en
`gcsgrep/testenv.local.md`, que no se versiona.

> El binario `gcsgrep/gcsgrep` y `gcsgrep/testenv.local.md` están en el `.gitignore` de la raíz.

## Estado

- **Iteración 1: implementada y verificada.** La spec se corrigió después de la revisión de
  la cátedra (FRs atómicos, casos de falla nuevos, VCs con observables literales, NFRs
  congelados) y la iteración se re-verificó contra ella: pasan sus 41 VCs, 30 de ellos
  contra GCS real y 3 como mediciones de rendimiento y memoria. Detalle en
  `gcsgrep-cobertura-vc.md`, sección "Iteración 1 — re-verificación contra la spec
  corregida".
- **Iteraciones 2 y 3: planificadas, no implementadas.** Costo en objetos
  grandes o comprimidos, UX y confiabilidad de red, y concurrencia con validación de
  rendimiento. Están en `gcsgrep-plan.md`.

Tag de esta carpeta: `01-Greenfield`.
