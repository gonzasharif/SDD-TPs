---
name: write-spec-brownfield
description: Usar cuando la persona quiere especificar un cambio sobre un codebase que ya existe — "quiero agregar X a <proyecto>", "especificá este cambio", "armá la spec para modificar…", "qué hay que tocar para soportar Y". Produce una spec con alcance dentro/fuera por path, invariantes comprobables, línea de base de regresión, FRs con VCs observables y plan de iteraciones. No usar para un proyecto desde cero.
---

# write-spec-brownfield

Convierte un pedido de cambio sobre código existente en una spec desde la que otro
equipo puede construir **sin romper lo que ya anda**.

**La regla que este skill hace imposible violar:** no hay spec brownfield sin
alcance *fuera* nombrado por path ni sin invariantes que se puedan comprobar con un
comando. Lo que no está acotado, se toca; lo que no se mide, se rompe en silencio.

## Cuándo NO usarlo

- Proyecto desde cero: no hay nada que preservar, no hay invariantes ni línea de base.
- Bugfix de una línea con test que lo cubre: no amerita spec.

## Pasos

1. **Descubrí antes de especificar.** Si no hay notas de exploración, lanzá un
   subagent `Explore` en modo solo lectura: punto de enganche, módulos vecinos,
   capa de portabilidad, cómo se testea hoy. Cada hallazgo con `archivo:línea` y el
   commit explorado. Nada de esto se escribe en la spec todavía.
2. **Escribí el propósito** en un párrafo: qué cambia y qué límite lo encierra.
3. **Alcance dentro, por path.** Una fila por archivo, con qué cambia y dónde.
4. **Alcance fuera, por path.** Lo que *no* se toca y por qué. Incluí features
   vecinas que alguien podría suponer incluidas.
5. **Invariantes.** Qué sigue siendo verdad después del cambio. Cada uno con el
   **comando** que lo comprueba y su salida esperada.
6. **Línea de base de regresión.** Qué se corre *antes* de tocar código y qué número
   da. Al cerrar cada iteración, la misma corrida tiene que dar lo mismo.
7. **FR/BR/NFR atómicos**, en Dado/cuando/entonces, cada uno con al menos un VC.
   Incluí VCs de **camino de falla**, no solo el feliz. Todo número va fijado.
8. **Plan de iteraciones.** Cada VC e invariante cae en exactamente una iteración;
   la primera es la más angosta que se verifica sola.
9. **Corré el chequeo mecánico:** `scripts/check-vc-coverage.sh <spec.md>` desde
   la raíz del skill. Corregí hasta que salga 0.
10. **Pedí revisión independiente** al subagent `spec-reviewer`, pasándole el path
    de la spec y la salida del paso 9. No la apruebes vos.

## Plantilla

```md
# Spec — <cambio>

## Propósito
<qué cambia, en un párrafo, y qué límite lo encierra>

## Alcance
### Dentro
| Archivo | Cambio |
|---|---|
| `<path>` (**nuevo**) | <qué> |
| `<path>` | <qué, y dónde: `archivo:línea`> |

### Fuera de alcance
- **`<path o glob>`** — <por qué no se toca>
- **<feature vecina>** — fuera de alcance.

## Invariantes
| # | Invariante | Cómo se comprueba |
|---|---|---|
| **INV-1** | <lo que sigue siendo verdad> | `<comando>` devuelve `<salida>` |

## Línea de base de regresión
<comando> → <resultado medido antes del cambio>

## Requerimientos
### FR-1 · <título>
**Dado** …, **cuando** …, **entonces** ….

**VC-1:** <acción concreta> → <salida observable con valores fijos>.
**VC-2:** <camino de falla> → <error observable>, <qué NO pasa>.

## Plan de iteraciones
| Iteración | Alcance | Cierra |
|---|---|---|
| **1** | <lo más angosto verificable> | FR-1, VC-1, VC-2, INV-1 |

## Preguntas abiertas
## Limitaciones conocidas
```

## Anti-patrones

Salen de los huecos reales que encontró la revisión de `brownfield/revision-spec.md`:

- **Invariante no observable** ("nunca llama a `execvp`"). Si no hay comando que lo
  compruebe, no es un invariante: es una esperanza.
- **Un límite que vive solo como invariante.** Si es el eje del cambio, necesita FR
  y VCs propios, o ninguna iteración lo cierra.
- **VC con número sin fijar** ("responde rápido", "en un timeout razonable").
- **Solo camino feliz.** Falta host inalcanzable, uso incorrecto, permiso denegado.
- **Fuera de alcance implícito.** Si no está escrito por path, se va a tocar.
- **Infraestructura de prueba al final.** Si un VC de la iteración 2 necesita un
  harness, el harness llega en la 2.
- **"Actualizar" un test viejo para que pase.** Un test viejo en rojo es regresión.
