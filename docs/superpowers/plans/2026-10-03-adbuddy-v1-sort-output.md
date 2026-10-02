# Plan: ningún 🟢 escribe ni mata por opciones agrupadas, abreviadas o posicionales (P1, hueco de la revisión A4)

> Plan v2: incorpora la revisión opus del plan (`.superpowers/sdd/2026-10-03-sort-output/plan-review.md`: C1 `pgrep -L`, C2 `ss -D/-K`, I1 `uniq --`, I2 asimetría de getopt, I3 emulador con `eval`, I4 `ip`, I5 barrido cerrado, M1-M8).
>
> Hallazgo (revisión opus de `fix/v1-confirm-hardening`, preexistente en main): `sort -uo FICHERO /dev/null`, `sort -mo …`, `sort -ro/…` salen 🟢 y vacían el fichero. `checkSort` (`host/internal/classify/checks_read.go:204`) solo mira `-o` suelto, `-o<valor>` que EMPIEZA por `-o` y `--output*`; una `o` dentro de un grupo (`-uo`) no se ve. La revisión del plan encontró más 🟢 que actúan sobre el móvil. Un 🟢 que modifica el móvil no se aparca (CLAUDE.md). Base: `origin/main`, rama `fix/v1-sort-output`. Lo marcado **[verificar]** es de memoria de toybox/iproute2 y lo confirma el emulador antes de fiarse.

## Principio del análisis (vale para todas las utilidades)

Un ayudante común en `checks.go`/`checks_read.go` (extrae el getopt agrupado que ya hace `checkLogcat`, `checks.go:104-139`, y lo usan sort, uniq, pgrep, ss y logcat; sus tests cubren `--`, valor en la palabra siguiente y letra desconocida): recorre las palabras; una palabra `-xyz` se lee letra a letra; una letra **con valor** se come el resto del grupo como valor, o la palabra siguiente si el grupo acaba ahí; `--` termina las opciones (el resto son ficheros); `-` solo y `+POS` son ficheros; se sigue leyendo opciones tras un posicional (toybox no usa `^`).

**Asimetría de seguridad (I2):** el error peligroso es tomar por «con valor» una letra que es un simple flag (el analizador se saltaría el resto del grupo o la palabra siguiente, y con ellos un `-o`). Una letra desconocida se trata como 🔴 o como flag, nunca como valor. El conjunto «con valor» es como mucho el que confirme `CMD --help` en el emulador (si no confirma `S`/`T` en sort, se quitan del conjunto con valor). **Si el valor va en la palabra siguiente y empieza por `-`, → 🔴** (`sort -t -- -o f x` escribe `f`: `--` es el valor de `-t`; primero se consume el valor y después se mira `--`). Un grupo que termina en una opción con valor sin palabra siguiente → 🔴 (nunca índice fuera de rango). Opciones largas por **lista blanca exacta**; larga desconocida o abreviada → 🔴 sin condiciones. Ante la duda, 🔴: un 🔴 de más cuesta una confirmación; un 🟢 que escribe cuesta un fichero o un proceso del móvil sin preguntar.

## Tarea 1: `sort` y `uniq` (+ ayudante común)

- **sort:** la `o` como opción (sola, en grupo, `-o<valor>`, `-o` en la palabra siguiente, también con otras banderas antes: `-uo`, `-mo`, `-nro`, `-ro/…`) → 🔴 «sort -o writes a file». Con valor: `o k t S T` (verificar en el emulador; `g` es un flag). `-to` = delimitador `o`, `-k2,2o` = clave con modificador: NO son salida (🟢 si el emulador confirma). `--output`, `--output=…`, `--out`, `--o` y cualquier larga fuera de la lista blanca → 🔴. Las formas habituales siguen 🟢: `sort`, `-h`, `-rh`, `-u`, `-n -r`, `-t, -k2`.
- **uniq (I1):** a partir de `--` todas las palabras cuentan como posicionales (`uniq -- -x /sdcard/y` → 🔴: antes 🟢 y escribía); `-f`/`-s`/`-w` consumen su valor, pegado o en la palabra siguiente (`uniq -f 1 in` pasa a 🟢; hoy da 🔴 de más); ≥2 posicionales → 🔴 como hoy.
- **Tests RED→GREEN** (`host/internal/classify/…_test.go`, fichero nuevo `sort_test.go` si conviene; **en las dos formas**: argv con `Classify([]string)` y `sh(…)`, como `hardening_test.go:43-55`, y con un caso envuelto: `nice sort -uo f x`, `timeout 5 sort -uo f x`).
  - 🔴 sort: `-uo f /dev/null`, `-mo f x`, `-ro/sdcard/x x`, `-o f x`, `-ofile x`, `--output=f x`, `--out f x`, `-u -o f x`, `-nro f x`, `x -uo f`, `x -o f x`, `-t -- -o f x`, `-k -- -o f x`, `- -uo f`, `+1 -uo f`, `-uo` (último), `-t -o f x` (valor que empieza por guion).
  - 🟢 sort: `sort`, `-h`, `-rh`, `-u`, `-n -r`, `-t, -k2 x`, `-to x`, `-k2,2o x`, y `sort -- -o` (🟢 solo si el emulador lo confirma como fichero).
  - 🔴 uniq: `-- -x /sdcard/y`, `-c in out`; 🟢: `-f1 in`, `-f 1 in`, `-c in`.
  - Los que ya eran 🔴 (`-o f`, `-ofile`, `--output=f`) valen de regresión. `habitual.jsonl`, `m2-levels.jsonl`, `change_test.go:64` y `hardening_test.go:230` siguen pasando (correr el corpus habitual en cada commit).
  - Razones nuevas con su traducción en `reasons_es.go` (`reasons_test.go` lo exige).
- Commits: (1) tests rojos, (2) ayudante + sort + uniq.

## Tarea 2: `pgrep`, `ss`, `ip` y el resto del barrido (tabla cerrada)

- **C1 `pgrep` (`shell-read.yaml` ~:67, hoy sin `check`):** `check: pgrep` con el ayudante; `-L SIGNAL` (toybox: «Send SIGNAL instead of printing name»: `pgrep -L 9 adbd` mataría adbd) sola, en grupo (`-lL9`, `-fL KILL`) o `--signal` → 🔴; con valor: `d L u U t s P g G` (verificar con `pgrep --help`). 🟢: `pgrep -f com.x`, `pgrep -l x`. Test RED: `pgrep -L 9 x`, `-lL9 x`, `-fL KILL x`.
- **C2 `ss` (`shell-read.yaml` ~:78, sin `check`):** `check: ss`; `D` (`-D FILE`/`--diag=FILE` escribe y trunca un fichero), `K` (`--kill`), `F` (lee filtros de un fichero: inocuo, pero 🔴 ante la duda) y cualquier larga fuera de la lista blanca → 🔴; 🟢: `ss -tanp`. Verificar que `ss` existe en la imagen. Test RED: `ss -D /sdcard/x`, `ss -tanD /sdcard/x`, `ss --diag=/sdcard/x`, `ss -K`.
- **I4 `ip`** (`checkIP`, `checks_read.go` ~:411): lista blanca de lectura: objeto = cualquier prefijo de `addr|link|route|rule|neigh|…` (iproute2 usa `matches()`, acepta prefijos), verbo = vacío/`show`/`list`/`get` (o prefijo de `show`/`list`), opciones `-4 -6 -s -d -o -j -p -br -c`; lo demás 🔴, `-b/-batch/-force` incluidos. Tests RED: `ip a a 1.2.3.4/24 dev wlan0`, `ip l s wlan0 down`, `ip r d default`, `ip -b -`, `ip -batch /sdcard/x`; GREEN: `ip addr show wlan0` (`habitual.jsonl:59`) y las formas de lectura del corpus.
- **Tabla cerrada del resto** (el implementador la rellena con `CMD --help` y una prueba en el emulador y la deja en el informe; cada fila «No» queda documentada, cada «Sí» recibe `check`+test o se retira de 🟢): xxd (toybox: un fichero como máximo, `-r` escribe a stdout **[verificar]** `xxd /proc/version /data/local/tmp/p1-x`), file (toybox `-bhLs`, sin `-C`), ls, cat, stat, du, df, readlink, realpath, md5sum/sha1sum/sha256sum, cmp, diff, head, tail, wc, cut, tr, grep/egrep/fgrep, strings, od, hexdump, base64, seq, expr, ps, top, pidof, free, vmstat, uptime, uname, id, printenv, getprop, netstat, ping, echo/cd/true/false/`:`/test/`[`/sleep/exit/break/continue/which/type/whence/basename/dirname. Esperado: ninguna escribe ni mata salvo las de arriba; si el emulador dice otra cosa, se trata.
- **`sed`, `awk`, `find`, `tee`, `logcat`:** la revisión no halló agujero de opciones agrupadas (las formas raras salen 🔴 de más); no se tocan salvo que el emulador diga lo contrario.
- Commits: (1) tests rojos, (2) pgrep + ss, (3) ip, (4) tabla/docs/pendientes. Si el oráculo cambia por un caso nuevo del generador/`differential.jsonl`, se regraba (CLAUDE.md).

## Oráculo / emulador (I3: con `eval`, `-s` y parada con las mismas variables)

Nunca el móvil del usuario ni el servidor adb 5037. Arranque como en `host/README.md` («The classifier's oracle»), **con eval**, puertos y AVD propios para no pisar el emulador de otra sesión:

```
eval "$(ADBUDDY_EMU_PORT=5584 ADBUDDY_EMU_ADB_PORT=5043 ADBUDDY_EMU_AVD=adbuddy-api37-p1 scripts/emulator.sh start)"
```

Antes de la primera orden manual comprobar `echo $ANDROID_ADB_SERVER_PORT` = `5043` (si está vacío o es 5037, parar). Cada orden manual con el adb de `ADBUDDY_MKSH_ADB` y `-s "$ADBUDDY_MKSH_SERIAL"` (`emulator-5584`); solo comandos que escriben en `/data/local/tmp/p1-*` del emulador (o matan un proceso propio de prueba, nunca `adbd`/`system_server`: la prueba de `pgrep -L` se hace con un `sleep 300 &` propio). Parar con las mismas variables: `ADBUDDY_EMU_PORT=5584 ADBUDDY_EMU_ADB_PORT=5043 scripts/emulator.sh stop` (sin ellas mataría el servidor 5039 de otra sesión). Con él: (a) confirmar que cada hueco es real antes del arreglo (`sort -uo /data/local/tmp/p1-out /dev/null` vacía el fichero; `pgrep -L 9 sleep` mata el `sleep` propio; `ss -D /data/local/tmp/p1-ss`) y que tras el arreglo el clasificador da 🔴; (b) las formas 🟢 de arriba. Si el emulador no arranca en 15 min, el implementador lo dice, se hace la parte de host con la semántica documentada de toybox y el oráculo queda en «Validaciones pendientes».

## No cambia

El evaluador de shell (`shell.go`, `word.go`), `testdata/habitual.jsonl` (el contrato de lo que no puede subir: `sort -h`, `sort -rh`, `du … | sort -rh | head -10`, `ip addr show wlan0`) ni ningún 🟢 habitual.

## Orden y límites

Dos tareas en serie sobre el mismo árbol (Sonnet `adbuddy-implementer`; si BLOCKED o ronda 4-5, `adbuddy-implementer-opus`); revisión `adbuddy-reviewer-opus` medium por tarea (clasificador = seguridad). Push de la rama sin `--force` y PR; el merge lo hace el supervisor. `git grep` de seriales e IPs antes del push (usar TEST-NET en ejemplos); trailer de commit exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Riesgo si me equivoco

Un 🔴 de más en una forma rara (`sort -to`, `ip a`) cuesta una confirmación al usuario. Un `sort -o`, `pgrep -L` o `ss -D` que se escape cuesta un fichero vaciado o un proceso muerto en el móvil sin pedir permiso: por eso las largas van por lista blanca, el valor que empieza por `-` es 🔴 y la duda se resuelve a 🔴.

## Resultado de la tarea 2 (barrido cerrado, comprobado en el emulador toybox 0.8.13-android)

Huecos reales reproducidos antes del arreglo: `pgrep -L 9 -x sleep` mató el `sleep` propio; `ss -D /data/local/tmp/p1-ss -t` creó el fichero. Arreglados con `check: pgrep` y `check: ss` (ayudante getopt), y `checkIP` con lista blanca. Resto de utilidades 🟢: ninguna escribe ni mata (`--help` y prueba con un segundo argumento: ls, cat, stat, du, df, readlink, realpath, md5sum/sha1sum/sha256sum, cmp, diff, head, tail, wc, cut, tr, grep, strings, od, base64, seq, expr, file, xxd [máx. 1 fichero], ps, top, pidof, free, vmstat, uptime, uname, id, printenv, getprop, netstat, ping, builtins). Detalle por utilidad en `.superpowers/sdd/2026-10-03-sort-output/task-2-report.md`. Sin validaciones manuales pendientes de este hueco. Nota: `ping -f` (flood) sigue 🟢: no escribe ni mata, y necesita root.
