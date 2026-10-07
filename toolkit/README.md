# Toolkit SDD — Lección 3

Encodea el pipeline de los TPs de las Lecciones 1 y 2 para que el agente lo cargue
solo: un skill, un subagent, un hook y una rule. Cada pieza sale de un paso que en
L1–L2 hicimos a mano.

Las piezas viven donde Claude Code las carga, en la raíz del repo; esta carpeta tiene
la documentación y la evidencia.

| Pieza | Archivo | Sale de | Concepto L1–L2 |
|---|---|---|---|
| 📘 Skill | [`.claude/skills/write-spec-brownfield/`](../.claude/skills/write-spec-brownfield/SKILL.md) | [`brownfield/spec-brownfield.md`](../brownfield/spec-brownfield.md) | **Alcance acotado** y **cobertura de VCs** |
| 👥 Subagent | [`.claude/agents/spec-reviewer.md`](../.claude/agents/spec-reviewer.md) | [`brownfield/revision-spec.md`](../brownfield/revision-spec.md) | **Revisión independiente** e **higiene de contexto** |
| 🪝 Hook | [`.claude/hooks/tests-green-before-commit.sh`](../.claude/hooks/tests-green-before-commit.sh) + [`.claude/settings.json`](../.claude/settings.json) | La regresión de la Iteración 2 de `gcsgrep` | **Seguridad ante regresiones** |
| 📏 Rule | [`CLAUDE.md`](../CLAUDE.md) | La convención de commits de `git log` | **Trazabilidad** spec → código |

## 📘 `write-spec-brownfield`

**Qué hacíamos a mano.** Para escribir `spec-brownfield.md` hubo que recordar, en
orden:
- alcance dentro y fuera **por path**;
- invariantes con el **comando** que los comprueba;
- una línea de base de regresión medida antes del cambio;
- FRs atómicos con VCs de camino de falla;
- un plan donde cada VC cae en una iteración.

Varias de esas cosas se nos escaparon en la primera versión: la revisión encontró 14
huecos.

**Qué encodea:**
- La `description` está en forma de trigger, con el fraseo real ("quiero agregar X a
  …", "especificá este cambio") y excluye el greenfield.
- 10 pasos numerados y una plantilla copiable con el esqueleto de nuestra spec.
- Los anti-patrones son los huecos reales de `revision-spec.md`: invariante no
  observable, límite sin FR propio, VC con número sin fijar, solo camino feliz, y el
  harness de prueba que llega tarde.

**Lo determinístico va en un script.** [`check-vc-coverage.sh`](../.claude/skills/write-spec-brownfield/scripts/check-vc-coverage.sh)
hace los chequeos mecánicos de la revisión sin que el modelo cuente:
- cada FR/BR/NFR tiene VC;
- no hay VCs repetidos;
- cada VC está en el plan, entendiendo rangos `VC-14…VC-17`;
- existen "Fuera de alcance" e "Invariantes".

## 👥 `spec-reviewer`

**Qué hacíamos a mano.** La revisión la hicimos quienes escribimos la spec, en la misma
conversación. Ese revisor ya "sabe" qué quiso decir la spec, y completa los huecos en la
cabeza en vez de reportarlos.

**Qué encodea:**
- **Independencia:** arranca con contexto limpio, solo con la spec y lo que ella cita,
  y tiene instrucciones de tratar lo ambiguo como hueco.
- **Higiene de contexto:** relee 300 líneas de spec y el código que referencia en su
  propia ventana. Al contexto principal vuelven solo tres bloques con formato fijo, los
  mismos de `revision-spec.md`: chequeos mecánicos, una tabla de huecos con gravedad y
  un veredicto.
- **Solo lectura garantizada:** `tools: Read, Grep, Glob`. El brief *pide* no editar;
  el front-matter lo *garantiza*. Por eso tampoco tiene Bash: la salida de
  `check-vc-coverage.sh` se la pasa el skill en el paso 10, en vez de correrla el
  subagent.

## 🪝 `tests-green-before-commit`

**Qué hacíamos a mano.** Durante la Iteración 2 la regla era que la Iteración 1 siguiera
en verde. Eso dependía de acordarse de correr `go test` antes de cada commit.

**Qué garantiza:**
- **Evento:** `PreToolUse` sobre `Bash`. Si el comando es `git commit`, también
  `git -C dir commit`, corre `go test ./...` en `greenfield/gcsgrep`.
- **Si la suite está roja:** `exit 2`, se veta el commit, y stderr le dice al agente qué
  test falló y que **arregle el código, no el test**.
- **Por qué no usa credenciales:** los tests contra GCS real llevan
  `//go:build integration` y no entran en `./...`. El hook es rápido y no las necesita.
- **Si falta Go**, bloquea en vez de dejar pasar. Un guardrail que se apaga en silencio
  cuando no puede chequear no garantiza nada.

**Limitaciones conocidas:**
- **Matchea el texto del comando:** `echo "git commit"` dispara el chequeo. Es un falso
  positivo conservador.
- **Solo cubre commits que hace el agente.** Un `git commit` desde una terminal propia
  no pasa por Claude Code; para eso haría falta un `pre-commit` de git.
- **Corre la suite en cada commit**, también en los de solo documentación. Con la caché
  de `go test` son unos segundos.

## Instalación

Ya está instalado en este repo. Para llevarlo a otro, copien `.claude/` y fusionen
`.claude/settings.json`. Hace falta `jq` y Go; si Go no está en el `PATH`, definan
`GO_BIN`. Reinicien la sesión del agente después de cambiar `settings.json`.

## Evidencia

| Pieza | Archivo | Qué muestra |
|---|---|---|
| Hook | [`evidencia/hook-manual-verde.txt`](evidencia/hook-manual-verde.txt) | Un comando que no es commit pasa; un commit con la suite verde pasa (`exit 0`) |
| Hook | [`evidencia/hook-manual-bloqueo.txt`](evidencia/hook-manual-bloqueo.txt) | Con una regresión en `-i` (FR-4) fallan `TestMatchString_IgnoreCase` y `TestRun_IgnoreCase`: **commit vetado** (`exit 2`) |
| Skill | [`evidencia/skill-check-vc-coverage.txt`](evidencia/skill-check-vc-coverage.txt) | El script aprueba `spec-brownfield.md` y rechaza una copia con un NFR sin VC y un VC fuera del plan |
| Skill | `evidencia/sesion-skill.md` | Sesión donde el skill carga **sin nombrarlo** |
| Subagent | `evidencia/sesion-subagent.md` | Sesión donde `spec-reviewer` revisa `brownfield/spec-brownfield.md` y devuelve solo el veredicto |
| Hook | `evidencia/sesion-hook.md` | Sesión donde el agente intenta commitear con un test roto, el hook lo veta y el agente arregla el código |

### Cómo reproducir las sesiones

Cada prueba va en una sesión nueva de Claude Code, abierta en la raíz del repo, y se
exporta con `/export` a `toolkit/evidencia/`.

1. **Skill, sin nombrarlo:**
   > *"Quiero agregar soporte para que gcsgrep lea objetos de un bucket S3 además de
   > GCS. Especificá el cambio."*

   Tiene que cargar `write-spec-brownfield`. Si no carga, se corrige la
   `description`, no los pasos.
2. **Subagent:**
   > *"Revisá brownfield/spec-brownfield.md, ¿está lista para planificar?"*

   Tiene que lanzar `spec-reviewer` y mostrar el veredicto.
3. **Hook bloqueando:** en `greenfield/gcsgrep/internal/match/match.go`, cambien
   `p = "(?i)" + p` por `p = "" + p`. Después:
   > *"Commiteá este cambio."*

   El hook tiene que vetar el commit. La señal de que funcionó es que el agente lee el
   stderr y restaura el código en vez de tocar el test.
