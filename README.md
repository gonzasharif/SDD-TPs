# SDD-TPs

Trabajos prácticos de **Spec-Driven Development (73.31)**. Cada carpeta es un TP y
recorre el mismo pipeline de SDD sobre un caso distinto:

| Carpeta | Lección | Caso | Tipo | Entregable |
|---|---|---|---|---|
| [`greenfield/`](greenfield/) | 1 | `gcsgrep`: grep sobre objetos de Google Cloud Storage | Proyecto desde cero | Spec, plan y **código** de la Iteración 1, verificado |
| [`brownfield/`](brownfield/) | 2 | Cliente SSH nativo en `tmux`, solo Linux | Cambio sobre un codebase existente | Notas de exploración y spec. **Sin implementación** |

Cada carpeta tiene su propio `README.md` con el detalle de lo que contiene y cómo
recorrerlo.

## El pipeline

Los dos TPs usan los mismos pasos. El brownfield agrega uno adelante, porque antes de
especificar hay que entender un sistema que ya existe.

| Paso | Greenfield (`gcsgrep`) | Brownfield (`tmux`) |
|---|---|---|
| 0 · Descubrir | — | `notas-exploracion.md`: mapa del código, en modo solo lectura |
| 1 · Especificar | `gcsgrep-requirements.md`, `gcsgrep-design.md`, `gcsgrep-spec.md` | `spec-brownfield.md`: alcance dentro y fuera, invariantes, FRs y VCs |
| 2 · Revisar | Correcciones a la spec tras la revisión de la cátedra | `revision-spec.md`: huecos encontrados y veredicto |
| 3 · Planificar | `gcsgrep-plan.md` | Plan de iteraciones dentro de la spec |
| 4 · Implementar | Código de la Iteración 1 en `gcsgrep/` | No se implementa, por consigna |
| 5 · Verificar | `gcsgrep-cobertura-vc.md` | Los VCs quedan definidos para quien implemente |

## Estructura

```
SDD-TPs/
├── README.md                   ← este archivo
├── greenfield/                 ← TP de la Lección 1
│   ├── README.md
│   ├── enunciado.md
│   ├── gcsgrep-requirements.md · gcsgrep-design.md · gcsgrep-spec.md
│   ├── gcsgrep-plan.md · gcsgrep-cobertura-vc.md
│   └── gcsgrep/                ← código en Go
└── brownfield/                 ← TP de la Lección 2
    ├── README.md
    ├── notas-exploracion.md
    ├── spec-brownfield.md
    └── revision-spec.md
```

## Tags

| Tag | Carpeta | Qué marca |
|---|---|---|
| `01-Greenfield` | `greenfield/` | El TP de la Lección 1 |
| `02-Brownfield` | `brownfield/` | El TP de la Lección 2 |
| `Entrega-1` | `greenfield/` | Merge del PR #1 (`iteration-2`) |

## Qué hace falta para recorrerlo

- **Greenfield:** Go (ver `greenfield/gcsgrep/go.mod`) para compilar y correr los tests
  unitarios. Los VCs contra GCS real necesitan además un proyecto de Google Cloud y
  credenciales; está explicado en [`greenfield/README.md`](greenfield/README.md).
- **Brownfield:** solo `git`, para clonar `tmux` y comprobar las referencias de las notas.
  No hay nada que compilar. Ver [`brownfield/README.md`](brownfield/README.md).
