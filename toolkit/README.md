# Toolkit SDD — Lección 3

Encodea el pipeline de los TPs de las Lecciones 1 y 2 para que el agente lo cargue
solo: un skill, un subagent, un hook y una rule. Cada pieza sale de un paso que en
L1–L2 hicimos a mano, y de lo que las correcciones de la cátedra penalizaron.

Las piezas viven donde Claude Code las carga, en la raíz del repo; esta carpeta tiene
la documentación y la evidencia.

| Pieza | Archivo | Sale de | Concepto SDD que encodea |
|---|---|---|---|
| 📘 Skill | [`.claude/skills/write-spec-brownfield/`](../.claude/skills/write-spec-brownfield/SKILL.md) | [`brownfield/spec-brownfield.md`](../brownfield/spec-brownfield.md) y las tres correcciones | **Alcance acotado**, **cobertura de VCs** (un VC por FR, BR, NFR e INV) y **decisiones con fundamento en código** |
| 👥 Subagent | [`.claude/agents/spec-reviewer.md`](../.claude/agents/spec-reviewer.md) | El criterio `correccion-de-specs` de la cátedra | **Revisión independiente** e **higiene de contexto** |
| 🪝 Hook | [`.claude/hooks/tests-green-before-commit.sh`](../.claude/hooks/tests-green-before-commit.sh) + [`.claude/settings.json`](../.claude/settings.json) | La regresión de la Iteración 2 de `gcsgrep` | **Seguridad ante regresiones** |
| 📏 Rule | [`CLAUDE.md`](../CLAUDE.md) | La convención de commits de `git log` | **Trazabilidad** spec → código |

## Cuándo la usa cada integrante

| Situación | Pieza | Quién la dispara |
|---|---|---|
| "Quiero agregar X a un proyecto que ya existe" | Skill `write-spec-brownfield` | El agente, por la `description`; no hace falta nombrarlo |
| La spec está escrita y hay que saber si se puede planificar | Subagent `spec-reviewer` | El skill en su paso 11, o quien pida "revisá esta spec" |
| El agente va a commitear | Hook `tests-green-before-commit` | Automático (`PreToolUse` sobre `Bash`) |
| Cualquier sesión | Rule `CLAUDE.md` | Se carga sola; fija commits con IDs y la regla de regresión |

## 📘 `write-spec-brownfield`

**Qué hacíamos a mano.** Para escribir `spec-brownfield.md` hubo que recordar, en orden:
alcance dentro y fuera por path, invariantes, línea de base, FRs atómicos con VCs de camino
de falla y un plan donde cada VC cae en una iteración. Aun así, la primera entrega perdió
puntos por lo que se nos escapó: FRs con varios resultados, invariantes sin VC numerado,
decisiones sin código que las funde y VCs sin salida literal.

**Qué encodea:**
- La `description` está en forma de trigger, con el fraseo real ("quiero agregar X a …",
  "especificá este cambio") y excluye el greenfield.
- 11 pasos numerados y una plantilla copiable con las secciones que la cátedra pide:
  glosario, invariantes con su `VC-INVn.k`, harness, matriz de cobertura, plan y
  **decisiones con elegido / fundamento / descartado**.
- Las reglas de las tres correcciones: atomicidad (un *Cuando*, un resultado), los cuatro
  caminos de falla, VCs con stdout/stderr literales y exit code, IDs `VC-<n>.<k>` alineados
  con los FR, NFR con umbral congelado y forma de medición, y los bordes que siempre se
  cubren.
- Una lista "Antes de entregar" de diez ítems.

**Lo determinístico va en un script.**
[`check-vc-coverage.sh`](../.claude/skills/write-spec-brownfield/scripts/check-vc-coverage.sh)
hace los chequeos mecánicos sin que el modelo cuente. Falla (`exit 1`) si:
- algún FR, BR, NFR o **INV** no tiene VC, o un VC apunta a un requisito que no existe;
- hay VCs repetidos, o `#VC < #FR + #BR + #NFR + #INV`;
- un requisito no cierra en **exactamente una** iteración del plan;
- un FR no tiene exactamente un "cuando", o un VC no tiene ningún literal entre backticks;
- faltan "Fuera de alcance", "Invariantes", "Glosario", "Decisiones" (con columna
  "Descartado"), "Plan de iteraciones" o el hash del commit base;
- hay wording de TBD ("a confirmar", "pendiente de", "sin medir"…) o historia del proceso.

Con `--strict` también falla por los avisos de atomicidad (" y " / " o " en el *Entonces*).

## 👥 `spec-reviewer`

**Qué hacíamos a mano.** La revisión la hicimos quienes escribimos la spec, en la misma
conversación. Ese revisor ya "sabe" qué quiso decir la spec, completa los huecos en la
cabeza en vez de reportarlos, y termina aprobándola.

**Qué encodea:**
- **Independencia:** arranca con contexto limpio, solo con la spec y lo que ella cita, trata
  lo ambiguo como hueco y no lee la autoevaluación previa antes de terminar.
- **El criterio de la cátedra:** seis dimensiones con severidad fija por chequeo (Issue,
  Warning, Suggestion), veredicto mecánico por dimensión (PASS/WARN/FAIL) y general
  (READY / NEEDS WORK / MAJOR ISSUES), y acciones `[MUST]`/`[SHOULD]`/`[COULD]`.
- **Sin evidencia no hay hallazgo:** cada hallazgo cita `archivo:línea`.
- **Solo lectura garantizada:** `tools: Read, Grep, Glob`. El brief *pide* no editar; el
  front-matter lo *garantiza*. La salida de `check-vc-coverage.sh` se la pasa el skill.

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
- **Si falta Go, bloquea** (`exit 2`) en vez de dejar pasar. Un guardrail que se apaga en
  silencio cuando no puede chequear no garantiza nada.
- **Si falta `jq`, no se apaga:** inspecciona el JSON crudo del evento con `grep`, que sigue
  conteniendo el texto del comando, y veta igual.

**Limitaciones conocidas:**
- **Matchea el texto del comando:** `echo "git commit"` dispara el chequeo. Es un falso
  positivo conservador.
- **Solo cubre commits que hace el agente.** Un `git commit` desde una terminal propia
  no pasa por Claude Code; para eso haría falta un `pre-commit` de git.
- **Corre la suite en cada commit**, también en los de solo documentación. Con la caché
  de `go test` son unos segundos.
- **Es un script de bash:** en Windows corre bajo Git Bash o WSL y necesita Go (`jq` es opcional).

## Instalación

Ya está instalado en este repo. Para llevarlo a otro, copien `.claude/` y fusionen
`.claude/settings.json`. Hace falta Go (`jq` es opcional); si Go no está en el `PATH`, definan
`GO_BIN`. Reinicien la sesión del agente después de cambiar `settings.json`.

## Evidencia

| Pieza | Archivo | Qué muestra |
|---|---|---|
| Hook | [`evidencia/hook-manual-verde.txt`](evidencia/hook-manual-verde.txt) | Un comando que no es commit pasa; un commit con la suite verde pasa (`exit 0`) |
| Hook | [`evidencia/hook-manual-bloqueo.txt`](evidencia/hook-manual-bloqueo.txt) | Con una regresión en `-i` (FR-4) fallan `TestMatchString_IgnoreCase` y `TestRun_IgnoreCase`: **commit vetado** (`exit 2`) |
| Hook | [`evidencia/hook-sin-jq.txt`](evidencia/hook-sin-jq.txt) | Sin `jq` el hook sigue **vetando** un commit con la suite roja (`exit 2`) y deja pasar uno en verde |
| Skill | [`evidencia/skill-check-vc-coverage.txt`](evidencia/skill-check-vc-coverage.txt) | El script rechaza la spec anterior (sin VC por invariante, sin decisiones con "Descartado"), aprueba la actual, y rechaza copias rotas |
| Skill | `evidencia/sesion-skill.md` | Sesión donde el skill carga **sin nombrarlo** |
| Subagent | `evidencia/sesion-subagent.md` | Sesión donde `spec-reviewer` revisa `brownfield/spec-brownfield.md` y devuelve solo el veredicto |
| Hook | `evidencia/sesion-hook.md` | Sesión donde el agente intenta commitear con un test roto, el hook lo veta y el agente arregla el código |

Las tres sesiones (`sesion-*.md`) se graban en una sesión real de Claude Code, no se
simulan: **todavía no están en el repo**. Hasta que se graben, la evidencia ejecutada es la
de los `.txt` de arriba.

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

   Tiene que lanzar `spec-reviewer` y mostrar el veredicto en el formato fijo.
3. **Hook bloqueando:** en `greenfield/gcsgrep/internal/match/match.go`, cambien
   `p = "(?i)" + p` por `p = "" + p`. Después:
   > *"Commiteá este cambio."*

   El hook tiene que vetar el commit. La señal de que funcionó es que el agente lee el
   stderr y restaura el código en vez de tocar el test.
