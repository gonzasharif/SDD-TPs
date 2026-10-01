# Revisión de la spec — `new-ssh-window`

> Paso **Revisar** del pipeline: el gate de "¿está lista para planificar?". Se releyó
> [`spec-brownfield.md`](./spec-brownfield.md) contra los criterios de la clase: **calidad de
> la spec**, **cobertura de VCs** y **disciplina de alcance** (límite solo-Linux e
> invariantes). Los huecos se corrigieron en la spec; este documento deja el registro.

## Chequeos mecánicos

| Chequeo | Resultado |
|---|---|
| Cada FR/BR/NFR tiene al menos un VC | Sí (NFR-1 reutiliza VC-10 e INV-6, y lo dice) |
| Los VC-1…VC-21 están definidos, sin faltantes ni repetidos | Sí |
| Todos los VCs aparecen en el plan de iteraciones | Sí |
| Referencias `archivo:línea` contra `tmux` `5a820e63` | Verificadas (ver notas de exploración) |
| Hay VCs de **camino de falla**, no solo el feliz | Sí: host desconocido o cambiado, auth no soportada, host inalcanzable, corte de conexión, uso incorrecto del comando |

## Huecos encontrados y cómo se resolvieron

| # | Gravedad | Hueco | Resolución |
|---|---|---|---|
| 1 | **Alta** | El hijo no ponía su tty en modo raw: la disciplina de línea local duplicaría el eco y atraparía `Ctrl-C`; ningún VC lo detectaba | FR-6 exige `cfmakeraw` (`compat.h:425`); VC-7 pide el eco **una sola vez**; VC-19 prueba `Ctrl-C` |
| 2 | **Alta** | El límite solo-Linux estaba solo como invariantes: sin FR ni VCs propios, la Iteración 1 no tenía qué cerrar | Nuevo **FR-0** con VC-14…VC-17 (default off, falta `libssh`, éxito, no-Linux) |
| 3 | **Alta** | El `sshd` de prueba llegaba en la Iteración 4, pero los VCs de red son de las 2 y 3 | El esqueleto del script con `sshd` pasa a la Iteración 2; `TEST_REQUIRE_SSHD=1` evita que el CI "pase" salteando |
| 4 | Media | Faltaba el contrato entre el comando y el cliente (`argv` sin formato) | FR-2: `argv[0]` destino, luego pares `-p` / `-i`; devuelve el código de salida |
| 5 | Media | INV-5 ("nunca llama a `execvp`") no era observable | Ahora: `strace -f -e trace=execve` sin eventos y `pgrep -x ssh` vacío |
| 6 | Media | VC-10 hablaba de 20 s sin fijar el timeout | FR-9: timeout fijo de 15 s |
| 7 | Media | Sin camino de falla para el uso incorrecto del comando | VC-18: sin destino falla y no crea ventana |
| 8 | Media | Un `kill-pane` podía dejar un hijo huérfano | FR-8 y VC-20: `SIGHUP` por cierre del master, el hijo se desconecta |
| 9 | Baja | Semántica de `-d`, `-P`, `-F`, `-t` sin declarar | FR-1: igual que `new-window` (`cmd-new-window.c:46`, `:172`); `-c` y `-e` excluidos |
| 10 | Baja | `known_hosts` sin alcance ni forma de probar | FR-4: solo el del usuario; el harness fija `HOME`; también host key cambiada |
| 11 | Baja | La línea de base solo cubría el build sin flag | Se agrega la corrida con flag: 172 scripts + el nuevo = 173 |
| 12 | Baja | Sin NFR sobre el camino de datos | NFR-3 / VC-21: `seq 1 200000` en menos de 30 s |
| 13 | Baja | `tmux.1` se instala siempre aunque el comando no exista | NFR-2: la doc aclara que depende del flag |
| 14 | Baja | Licencia de `libssh` sin mencionar | Limitaciones conocidas, marcada **a confirmar** |

## Lo que queda abierto (no bloquea planificar)

- **API y versión mínima de `libssh`: no verificadas.** No están en el repo; las fija quien
  implemente y debe confirmarlas en la Iteración 2.
- **VC-17 es manual:** el CI de Linux no puede ejecutar un `configure` en macOS o BSD.
- **Línea de base numérica sin medir:** no se compiló `tmux` (exploración solo lectura). Los
  números 92 y 172 salen de lectura estática; el verde/rojo lo registra quien implemente.
- **`respawn-pane` sobre un pane SSH** queda como limitación conocida, con spec propia.

## Veredicto

**Lista para planificar.** El alcance dentro y fuera está nombrado por path, el límite
solo-Linux tiene tres capas con VCs propios, los siete invariantes son comprobables y el
plan avanza en iteraciones chicas que dejan la línea de base intacta.
