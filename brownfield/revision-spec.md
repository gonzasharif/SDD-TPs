# Revisión de la spec — `new-ssh-window`

> Paso **Revisar** del pipeline: el gate de "¿está lista para planificar?". Se releyó
> [`spec-brownfield.md`](./spec-brownfield.md) contra los criterios de la clase: **calidad de
> la spec**, **cobertura de VCs** y **disciplina de alcance** (límite solo-Linux e
> invariantes). Los huecos se corrigieron en la spec; este documento deja el registro.

## Cuarta iteración — tras la revisión independiente

Una primera pasada del `spec-reviewer` sobre la tercera iteración dio **MAJOR ISSUES** (dimensiones 2, 3, 4 y 5 en FAIL). La spec se **renumeró** otra vez: FR-1…FR-62, BR-1…BR-4, NFR-1…NFR-3 e INV-1…INV-7; 100 VCs.

| Hallazgo | Resolución |
|---|---|
| 2.1 · VC-7.1 exigía `ssh_client_run` en la Iteración 1, que tenía "fuentes vacías" | La Iteración 1 incluye `ssh_client_run()` como esqueleto que devuelve `1` |
| 3.1 · VCs sin comando o con placeholder (33.1, 42.1, 43.1, 53.1, 56.1) | Cada uno trae `new-ssh-window …` y la espera al pane; `$SSHD_PID` y `$FREE` en el harness |
| 3.1 · VC-BR1.1 fallaba por construcción: `tmux -vv` loguea los argumentos de `send-keys` (`cmd.c:249`) | La passphrase se inyecta con `load-buffer` + `paste-buffer` |
| 3.2 · VC-34.1 obligaba a decidir comando, espera y estado | Comando y espera explícitos en el VC |
| 4.1 · NFR-2 sin número | Una sinopsis idéntica al `.usage` y al menos una mención de `--enable-native-ssh`, medidas con `man -l` |
| 4.2 · NFR-3 no verificaba "sin perder ni duplicar" y `history-limit` es 2000 | `history-limit 250000`; el VC cuenta 200000 líneas y 0 duplicadas |
| 4.3 · NFR-1/3 sin aclarar `strace` | El server corre sin `strace` en los VCs de tiempo |
| 5.1 · Cuatro decisiones menores sin código | Citas: `spawn.c:541`, `client.c:126`, `proc.c:227`, `options-table.c:845-851`, `server.c:491-499` |
| 5.3 · Contradicción sobre funciones de `libssh` | Solo se cita `ssh_session_is_known_server` (lectura); la spec fija la versión, no las funciones |
| 2.2 · FR-6 y FR-7 con resultados alternativos | Se partieron: FR-6/FR-7 (`off`/`on`), FR-8/FR-9 (con y sin flag) |
| 2.5 · Comportamientos solo en el VC | Pasaron al Entonces (FR-29, FR-55, FR-59) o se quitaron del VC |
| 3.3 – 3.7 · Bordes de entrada | FR-23 amplía el destino mal formado (`@h`, `a@b@c`, espacios); FR-25 ruta de clave no absoluta; FR-41 clave por defecto ilegible; FR-49 archivo que no es clave; VC de `known_hosts` inexistente y sin `\n` final |
| 3.8 · `native-ssh-guard.sh` | Se define cómo elige qué VC correr; `regress/list-commands.base` versionado; VC-5.1 corre en una copia del árbol |
| 5.2 · Herramientas del harness | Versiones y paquetes de CI nombrados |
| 6.1 · Alias sin fundamento | Decisión con `cmd-new-window.c:39` |
| 6.2 · CI sin requisito | BR-4 |
| B.2 · Línea de base sin medir | Medida: 171 `PASS` y 1 `FAIL` previo (`prompt-words-history.sh`) |
| B.3 · INV-5 e INV-6 a medias | Cierran en la Iteración 4; sus VCs se parten (`.1` colgado, `.2` con sesión) |

**No se aplicó** el hallazgo 2.3 (símbolos de `tmux` en 20 FR): en una spec brownfield esas citas
son el fundamento que la cátedra pidió, no un mecanismo. Queda como decisión del equipo.

**Segunda pasada independiente** (MAJOR ISSUES, casi todo ejecutabilidad de VCs). Se aplicó:
`env -i` y `sshd` de prueba documentados en el harness; `TEST_REQUIRE_SSHD` se quitó (el script
falla si el comando está y falta `sshd`); VC-INV3.2 compara el conjunto de `PASS` con la línea de
base y no usa el exit de `make`; VC-3.1 con wrapper de `pkg-config`; VC-34.3 con `known_hosts`
sin `\n` final; VC-43.1 y VC-52.1 con quoting correcto; VC-NFR2.1 con `MANWIDTH=200`; glosario
unificado (cliente `tmux`, cliente SSH, servidor `tmux`, `sshd`, clave por defecto utilizable);
nuevos VC-23.6, VC-28.5, VC-47.2 y VC-49.2. Limitaciones: VC-5.1/VC-INV2.1 solo en máquina no Linux.

**Tercera pasada independiente** (sesión real, `toolkit/evidencia/sesion-subagent.md`): NEEDS WORK,
con un solo Issue (3.1: VCs de autenticación sin datos fijados). Se aplicó: formato de `known_hosts`
(`[127.0.0.1]:$PORT`, `ssh-keyscan`) y término "host conocido" en el glosario; `$PASS` como passphrase de
la clave protegida; VC-46.1 con su propia configuración de `sshd` (ya no contradice la base); medición de
tiempos en el harness (`t0`/`t1` con `date +%s%N`, consulta cada 100 ms) para VC-55.1, VC-59.1 y VC-NFR3.1;
VC-58.1 con una sola sesión y `pgrep -n`; VC-35.2 (`known_hosts` inexistente no se crea); referencia de
BR-4 corregida. Las citas `server.c:468` y `window.c:597` coinciden con el commit `5a820e63`; la diferencia
era con las notas. **Quedan** como warnings: mecanismos en FR (decisión del equipo), usuario por defecto,
`-t :N` ocupado, precedencia entre fallas de ejecución, `-i` a un directorio, connect TCP que no completa.

**Sigue sin hacerse:** el hash del commit de la entrega;
los VCs de red, que necesitan la implementación.

## Tercera iteración — contra el resumen de correcciones

Se releyó la spec contra `resumen-correcciones-sdd` (las tres correcciones recibidas). La
spec se **renumeró** otra vez: FR-1…FR-57 y VCs `VC-<n>.<k>` (`VC-BRn.k`, `VC-NFRn.k`,
`VC-INVn.k`), de modo que cada VC lleva el número del requisito que observa.

**Esta sección no es una aprobación.** Quien escribe la spec no la aprueba: el veredicto lo
da el `spec-reviewer`. El chequeo mecánico (`check-vc-coverage.sh --strict`) sale con `0`.

| Error del resumen | Qué se encontró | Resolución |
|---|---|---|
| FRs no atómicos | FR-20, 21, 27, 36, 37 con mensaje y exit code juntos; FR-14, FR-30 con dos resultados; FR-9, 15, 36 con alternativas | Se partieron (57 FR). El exit code de toda falla es la **BR-3**; los mensajes tienen su FR |
| Caminos de falla sin FR ni VC | Sin clave, clave rechazada, passphrase incorrecta, `-p` inválido, `-i` inexistente o ilegible, destino vacío, `known_hosts` ilegible, precedencia | FR-21 … FR-25, FR-34, FR-41, FR-43, FR-44 con su VC |
| Texto del error sin fijar | `<motivo>` de FR-36 sin definir; `<host>` ambiguo | FR-55 y FR-56 con texto literal; glosario define host, usuario y falla |
| Decisiones sin "descartado" | La tabla tenía decisión y código, no las alternativas | Columna "Descartado y por qué"; decisiones nuevas: flags, exit code `1`, timeout de 15 s y umbrales de NFR |
| TBD | "LGPL (a confirmar)", versión mínima de `libssh` sin fijar, línea de base "sin medir" | Mínimo `libssh >= 0.9.0` (FR-2, FR-4): en 0.9.0 `ssh_is_server_known` y `ssh_write_knownhost` ya están deprecadas en favor de `ssh_session_is_known_server`. Licencia sin "a confirmar". Línea de base medida (abajo) |
| NFR sin texto ni medición | NFR-3 solo existía en su VC; ningún VC nombraba cómo medir | NFR-1 y NFR-3 con umbral, condición y herramienta (`date +%s%N`) en el requisito y en el VC |
| IDs de VC desalineados | FR-22 usaba VC-23, FR-36 usaba VC-38 | `VC-<n>.<k>` por requisito |
| VCs atados al código | VC-8 y VC-41 hacían `grep` sobre fuentes | Se quitaron; VC-4 manual pasó a `VC-5.1` automático (`native-ssh-guard.sh`, build fuera del árbol) |
| Historia del proceso | Banner "Iteración 2 de la spec, tras la corrección de la cátedra" | Quitado; el registro vive acá |
| Autoevaluación como aprobación | La segunda iteración cerraba con "lista para planificar" | Se aclara abajo; el veredicto es del `spec-reviewer` |
| Glosario | No había | Sección "Glosario" |

**Citas verificadas** contra un checkout de `tmux` en `5a820e63`: `cmd.c:123`, `spawn.c:478`,
`:504`, `:541`, `:543`, `:544`, `:552`, `:567`, `:574`, `tmux.h:2531`, `:4195`, `:1306`,
`configure.ac:463`, `:508`, `:545`, `:670`, `:1006`, `Makefile.am:243`, `:253`,
`cmd-new-window.c:33`, `:41`, `:46`, `:83`, `:156`, `:172-174`, `arguments.c:240`, `cmd.c:530`,
`format.c:901`, `:958`, `:2288`, `compat.h:426`, `client.c:349`, `window.c:597`, `:612`,
`server.c:198`, `:258`, `:468`, `:491`, `:499`, `proc.c:227`, `osdep-linux.c:30`, `:92`,
`tmux.c:624`, `environ.c:253`, `:265`, `options-table.c:1207`. Los textos `unknown flag -%c` y
`usage: %s %s` salen de `arguments.c:240` y `cmd.c:530`.

**Línea de base medida** (build de `5a820e63` con `--enable-utf8proc`): `make` sale con `0`;
`tmux list-commands` imprime **92** líneas; `regress/` tiene **172** scripts `*.sh` (171 `PASS` y 1 `FAIL`
previo, medido en la cuarta iteración).

### Lo que sigue sin hacerse

- La corrida **completa** de `regress/` no se completó (cada pasada tarda varios minutos; los
  scripts que corrieron dieron `PASS`). La hace quien implemente antes de la Iteración 1.
- Los VCs de red (`sshd`, `strace`, listener mudo) no se ejecutaron: no hay implementación.
- Falta el **hash del commit de la entrega**: se agrega al entregar, después del merge.
- Falta la pasada del `spec-reviewer` sobre esta versión.

## Segunda iteración — tras la corrección de la cátedra

La primera entrega (tag `02-Brownfield`, commit `e8e6d3a`) obtuvo **5,5/10**: VE-1 y VE-2
cumplen; VE-3 parcial; VE-4 y VE-5 no se cumplen. Esta iteración ataca esos tres criterios.
La spec se **renumeró** entera (FR-1…FR-37, VC-1…VC-54); los IDs de la sección "Primera
revisión" de más abajo son los de la versión anterior.

### VE-3 · FR con más de un comportamiento

| FR viejo | Qué empaquetaba | FR nuevos |
|---|---|---|
| FR-0 | flag solo en Linux con `libssh` + resumen de `configure` | FR-1 (apagado por defecto), FR-2 (activa), FR-3 (falta `libssh`), FR-4 (no-Linux), FR-5 (resumen), FR-6 (fuentes solo con flag) |
| FR-1 | existe + alias + un posicional + `-d/-P/-F/-n/-t` + sin `-c/-e` | FR-7, FR-8, FR-9, FR-10, FR-11, FR-12, FR-13, FR-15 |
| FR-2 | `spawn_window` con `SPAWN_SSH` + hijo corre el cliente + contrato de `argv` + puerto | FR-16, FR-17, FR-18, FR-19 |
| FR-4 | host desconocido + host cambiado | FR-20, FR-21 |
| FR-5 | agent + `-i` + claves por defecto + orden + passphrase + sin password | FR-22 … FR-27 |
| FR-6 | pty remoto + shell y bytes + tty raw + restaurar | FR-28, FR-29, FR-30, FR-31 (restaurar el tty se quitó: no es observable) |
| FR-8 | código de salida + corte + `kill-pane` | FR-33, FR-34, FR-35 |
| FR-9 | error de conexión + timeout | FR-36, FR-37 |

FR-3 y FR-7 ya eran atómicos (hoy FR-14 y FR-32).

### VE-4 · Requisitos sin VC que los ejercite

| Hueco señalado | Resolución |
|---|---|
| INV-1, INV-3, INV-4, INV-5, INV-7 tenían el procedimiento en la tabla, no un `VC-n` | Sección "VCs de invariantes": VC-46, VC-48/49, VC-50/51, VC-52, VC-54 |
| INV-2: VC-17 viejo solo mostraba el error de `configure` | **VC-47**: el job de macOS compila sin tocarlo y `regress/native-ssh-guard.sh` comprueba ahí que el comando y `ssh_client_run` no están en el binario |
| INV-6: VC-10 viejo medía latencia, no hilos | **VC-53** cuenta `/proc/<pid>/task`; la latencia queda en VC-43 (NFR-1) |
| Ningún VC observaba un comando existente ni el modelo de panes | **VC-48** (`list-commands` idéntico salvo la línea nueva), **VC-49** (172 scripts), **VC-51** (un pane común sigue pasando por `execvp`) |
| FR-2 viejo: `pane_pid` y `pane_tty` también los cumple un pane shell | **VC-17**: `/proc/<pane_pid>/exe` es el binario `tmux`; **VC-18** lee el `argv` con `#{pane_start_command}` |
| Semántica de `-d/-P/-F/-n/-t` y ausencia de `-c/-e` sin observar | VC-10 … VC-13 y VC-15 |
| Prompt de passphrase sin observar | VC-27 |
| "Destino que descarta paquetes" sin forma de armarlo | Harness: listener mudo con `nc -l`; VC-40 y VC-43 |

Se agregó la sección "Harness de prueba" y la "Matriz de cobertura" (requisito → VC → caso de falla).

### VE-5 · Decisiones sin código que las funde

| Decisión | Antes | Ahora |
|---|---|---|
| 3 · `libssh` u OpenSSH | "es lo que el enunciado excluye" | los `exec` de `spawn.c:552/567/574` son la única vía a OpenSSH; `PKG_CHECK_MODULES` (`configure.ac:508`, `:463`, `:670`) es cómo `tmux` detecta librerías; `closefrom` (`spawn.c:541`) deja al hijo con fds 0-2 |
| 5 · claves o agent | "cubre el uso real" | `update-environment` (`options-table.c:1207-1214`) → `environ_for_session` (`environ.c:253`) → `environ_push` (`spawn.c:544`) |
| 4 · event loop | remitía al hallazgo 6 | cita `tmux.c:624`, `osdep-linux.c:92`, `server.c:198`, `:258`, `proc.c:227` en la fila |
| 1, 2, 6 | ya citaban código | se agregó la columna "FR" que las refleja |

### Citas corregidas en las notas

`format_cb_pane_current_command` no existía (es `format_cb_current_command`, `format.c:958`,
y se leyó el manejo del `NULL`); `cmd-display-menu.c:498` → `:499`; `spawn.c:458` → `:459`;
`struct cmd_entry` tiene `source` y `target`; `Makefile.am:252-255` → `:253-255`.

### Chequeos de esta iteración

| Chequeo | Resultado |
|---|---|
| Cada FR, BR, NFR e INV tiene al menos un VC numerado | Sí: 37 FR + 2 BR + 3 NFR + 7 INV, 54 VCs |
| VC-1…VC-54 definidos una sola vez | Sí (contado con `grep`) |
| Referencias `archivo:línea` nuevas | Releídas contra un clon de `tmux` en `5a820e63` |
| Casos mínimos: auth rechazada, sesión que no abre, build no-Linux, comando existente | VC-28, VC-40, VC-47, VC-48/VC-51 |
| Cada VC se puede correr en la iteración que lo cierra | Corregido (ver abajo) |

### Hallazgo: VCs que el plan cerraba antes de poder correrlos

Al revisar el plan contra los VCs aparecieron VCs asignados a iteraciones donde no se podían
ejercitar:

| VC | Iteración | Problema | Resolución |
|---|---|---|---|
| VC-19 | 2 | "llega al prompt remoto" necesita auth (3) y shell (4) | Observa la línea `Connection from … port $PORT` del log de `sshd` |
| VC-16, VC-17 | 2 | piden el hijo vivo, pero en la 2 el cliente solo conecta y sale: carrera con `ps` y `/proc` | Corren contra el listener mudo, que mantiene vivo al hijo hasta el timeout |
| VC-52, VC-53 | 2 | dependían de la sesión del VC-31, que cierra en la 4 | En la 2 se corren con la conexión colgada; se repiten con la sesión en la 4 |
| VC-23, VC-24, VC-25, VC-27 | 3 | "llega al prompt remoto" necesita la shell de la 4 | Observan `Accepted publickey for $USER` en el log de `sshd` |

El harness define ahora tres observables ("llega a `sshd`", "se autentica", "llega al
prompt remoto") y dice qué iteración necesita cada uno.

### Lo que sigue abierto

- **API y versión mínima de `libssh`: no verificadas** (no están en el repo de `tmux`).
- **VC-4 es manual** (el flag en macOS): ningún job pasa el flag fuera de Linux.
- **Línea de base sin medir:** no se compiló `tmux`; 92 y 172 salen de lectura estática.
- **`strace` en el harness** supone que el runner permite trazar procesos hijos.

*Autoevaluación de la segunda iteración: no vale como aprobación (ver la tercera).*

---

# Primera revisión (previa a la entrega)

> Los IDs de FR y VC de esta sección son los de la **versión anterior** de la spec.

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

## Veredicto de la primera revisión

**Lista para planificar.** El alcance dentro y fuera está nombrado por path, el límite
solo-Linux tiene tres capas con VCs propios, los siete invariantes son comprobables y el
plan avanza en iteraciones chicas que dejan la línea de base intacta.
