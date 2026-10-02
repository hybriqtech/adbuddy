## Restricciones globales

- **Rama `feat/m2.5b-app-emulador`**, local, sobre `main` 2f3d650. Si `main` avanza antes de la Tarea 1, se rebasa sobre `origin/main` (la rama no se ha subido). Sin push salvo como copia (tras `git grep` de seriales e IPs) y nunca `--force`; a `main` integra el coordinador.
- **Ledger:** `.superpowers/sdd/2026-10-03-adbuddy-m2.5b-emulador/progress.md` (se añade con `git add -f`). Cada decisión, como `Ruling: qué — por qué — qué cuesta si me equivoco`.
- **El móvil del usuario no se toca.** Ningún paso ejecuta nada en un móvil real. Todo lo que habla con un dispositivo lo hace con el número de serie del emulador (`emulator-5600` en desarrollo) y en el servidor adb del emulador (`ANDROID_ADB_SERVER_PORT`, nunca 5037). Gradle se niega a lanzar un test conectado si `ANDROID_SERIAL` no es exactamente un `emulator-<puerto>` (Tarea 2) y `emutest` se niega con otro número de serie o con el servidor 5037.
- **Un emulador a la vez en el PC** (cada uno ocupa ~5 GB de RAM y el runner de la CI corre en este PC). Dos comprobaciones con nombre:
  - **CI-LIBRE:** ningún job del workflow `emulator` (oráculo) en marcha o en cola. Tiene que imprimir `0`:

    ```bash
    GH_CONFIG_DIR="$HOME/.config/gh-hybriqtech" gh run list -R hybriqtech/adbuddy --workflow emulator.yml --limit 20 \
      --json status --jq '[.[] | select(.status != "completed")] | length'
    ```

  - **SIN-EMULADOR:** ningún proceso de emulador en el PC. Tiene que imprimir nada (PowerShell):

    ```powershell
    Get-Process qemu-system-x86_64-headless,emulator -ErrorAction SilentlyContinue | Select-Object Id, ProcessName
    ```

  Antes de arrancar el emulador de desarrollo: CI-LIBRE y SIN-EMULADOR. Antes de cada paso largo con el emulador encendido (Gradle `connected…`, `go test ./internal/emutest`, R3 del spike): CI-LIBRE otra vez. Si no se cumplen, espera (con `Monitor` y un bucle `until`, nunca `sleep` en primer plano) y vuelve a mirar; si un job de la CI empieza con el de desarrollo encendido, páralo y espera a que acabe. Tras cada `stop`, SIN-EMULADOR hasta ~30 s:

  ```powershell
  $end = (Get-Date).AddSeconds(30)
  while ((Get-Process qemu-system-x86_64-headless,emulator -ErrorAction SilentlyContinue) -and (Get-Date) -lt $end) { Start-Sleep -Seconds 1 }
  Get-CimInstance Win32_Process | Where-Object { $_.Name -match '^(qemu-system|emulator)' } | Select-Object ProcessId, Name, CommandLine
  ```

  Si queda algo, actúa **solo** sobre los procesos cuya línea de órdenes contiene `-port 5600` (`Stop-Process -Id <ProcessId>`); cualquier otro (p. ej. `-port 5590`, de la CI) no se toca: detente y avisa.
- **El emulador solo se arranca con `scripts/emulator.sh`:** nunca `emulator.exe` a mano, ni con otro `-gpu` u otras opciones que las del script.
- **Receta del emulador de desarrollo** (desde la raíz del repo; el estado de la shell no persiste entre órdenes, así que cada orden carga el fichero de variables):
  - Arrancar (hasta 10 min; `timeout` amplio o en segundo plano):

    ```bash
    export ADBUDDY_EMU_AVD=adbuddy-dev ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060
    scripts/emulator.sh start > /tmp/adbuddy-m25b-emu.env && cat /tmp/adbuddy-m25b-emu.env
    ```

    La primera vez crea el AVD `adbuddy-dev` (imagen de Android 37, ya instalada). Debe imprimir `ADBUDDY_EMULATOR_SERIAL=emulator-5600` y `ANDROID_ADB_SERVER_PORT=5060`.
  - Preludio de cada orden posterior (en adelante, **`EMU;`**):

    ```bash
    export ADBUDDY_EMU_AVD=adbuddy-dev ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060; . /tmp/adbuddy-m25b-emu.env; export ANDROID_SERIAL="$ADBUDDY_EMULATOR_SERIAL"; ADB="$ADBUDDY_EMULATOR_ADB"; S="$ADBUDDY_EMULATOR_SERIAL"
    ```

  - Parar al terminar cada tarea: `EMU; scripts/emulator.sh stop`.
  - **Recuperación del AVD de desarrollo** (solo si el arranque no llega a `device` porque quedó `adb_enabled=0` en sus datos): con el emulador parado, borra los datos de usuario **de `adbuddy-dev` y de ningún otro AVD**, y arranca de nuevo (el emulador los recrea desde la imagen):

    ```bash
    ls "$HOME/.android/avd/adbuddy-dev.avd/"
    rm -f "$HOME/.android/avd/adbuddy-dev.avd/userdata-qemu.img" "$HOME/.android/avd/adbuddy-dev.avd/userdata-qemu.img.qcow2"
    ```

    Si en ese directorio no están esos ficheros, no borres otros: detente y explícalo.
  - **ADB-ON tras una recuperación.** Cada vez que el emulador vuelve a `device` después de un corte que no deshizo la app (recuperación, S6/S7 del spike, la mutación A5), antes de seguir:

    ```bash
    EMU; "$ADB" -s "$S" shell 'settings put global adb_enabled 1; settings get global adb_enabled'
    ```

    Tiene que imprimir `1`. Un `adb_enabled=0` que se queda en los datos del AVD falsea las medidas siguientes (y en la CI dejaría su AVD sin adb en el siguiente arranque).
  - Nunca toques el AVD `adbuddy-ci-api37`, el puerto 5590 ni el servidor 5049 (son de la CI), ni `adbuddy-api37`/5580/5039 (valores por defecto) ni `adbuddy-api34`.
- **Gradle** (desde `android/`): `export JAVA_HOME='C:\Users\Jose\AppData\Local\Android\jdk-17.0.20.1+1'` y `/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon` (en los pasos, `gradle`). `android/local.properties` no se versiona; si falta: `printf 'sdk.dir=C\\:/Users/Jose/AppData/Local/Android/Sdk\n' > android/local.properties`.
- **Go:** `go` y `gofmt` del PATH (`/c/Program Files/Go/bin`). Siempre `-count=1` y `-timeout`. Antes de cada commit de `host/`: `gofmt -l .` vacío y `go vet ./...` con `GOOS=windows`, `linux` y `darwin`. Sin `-race` en Windows.
- **Windows y Git Bash:** sin `sleep` en primer plano en el PC (las esperas cortas van dentro del emulador, `adb shell 'sleep N; …'`, o en un script en segundo plano). Una orden que espera dentro del emulador necesita un `timeout` de la herramienta mayor que la espera (p. ej. 300000 ms para `sleep 120`), y Gradle con un dispositivo, unos 900000 ms. Ficheros con LF.
- **No cambies la app ni el protocolo** (`android/app/src/main`, `protocol/`), salvo las mutaciones temporales que un paso pide y revierte en el mismo paso. Si un test revela un fallo de la app, detente y repórtalo: el arreglo es otra tarea.
- **No instales paquetes del SDK** ni imágenes de sistema ni nada que pida administrador. Si falta algo, `BLOQUEADO`.
- **Commits:** identidad local del repo (hybriqtech), nunca `git config --global`. Cada mensaje termina con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, exactamente así. `git add` con rutas explícitas, nunca `-A`.
- **Privacidad:** ni seriales ni IPs reales en el repo; `emulator-5600`, `emulator-5590` y `emulator-5580` son los únicos de este plan.
- **Revisores:** no arrancan el emulador; leen la evidencia del informe (órdenes y salidas). Límite de 30 min por revisión y `go test -timeout` en lo que ejecuten. Sin subagentes dentro de una tarea.


### Tarea 1: spike — ¿sobrevive el emulador a Cortar acceso y vuelve al Reactivar?

**Implementador:** `adbuddy-implementer-opus` (exploración con criterio). **Revisión:** sin revisor de código (no se commitea código); el controlador lee el ledger, aplica la regla de decisión y escribe los rulings.

**Ficheros:**
- Crear: `.superpowers/sdd/2026-10-03-adbuddy-m2.5b-emulador/progress.md`
- Temporal (se borra en el paso 9): `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessOnEmulatorTest.kt`, `/tmp/adbuddy-m25b-timeline.sh`

**Interfaces:**
- Produce para la Tarea 4: la variante (A, A', A en local, A' en local o B) y el valor de `tcpipOnEmulator` (`true`/`false`), como rulings en el ledger.
- Produce para todo el plan: si la imagen de Android 37 sirve para la app (S1).

- [ ] **Paso 0: requisitos.**

  ```bash
  git fetch origin
  git merge-base --is-ancestor 2f3d650 HEAD && echo "base con el PR de puffin"
  git show origin/main:scripts/emulator.sh | grep -n 'ADBUDDY_EMU_'
  git status --short   # vacío
  git log --oneline HEAD..origin/main   # si imprime algo: git rebase origin/main
  ```

  Esperado: la rama contiene 2f3d650 y `emulator.sh` de `main` lee `ADBUDDY_EMU_AVD`, `ADBUDDY_EMU_PORT` y `ADBUDDY_EMU_ADB_PORT`. Si los nombres son otros, usa los de `main` en todo el plan y anótalo. Después, CI-LIBRE y SIN-EMULADOR (Restricciones globales).

- [ ] **Paso 1: crea el ledger** con esta cabecera y ve añadiendo cada medida con su orden y su salida:

  ```markdown
  # M2.5b · La app en el emulador — progress

  Plan: docs/superpowers/plans/2026-10-03-adbuddy-m2.5b-app-emulador.md

  ## Spike (Tarea 1)

  Emulador: <versión de emulator/source.properties>, AVD adbuddy-dev, emulator-5600, servidor adb 5060.

  Nota: 5600 (y el 5590 de la CI) están fuera del rango que escanea el servidor adb (5555-5585): tras perder adbd, el emulador vuelve por los reintentos de adb (~60 × 1 s). reactivateAfter (15 s) debe quedar muy por debajo de 60 s, y estas medidas no valen para el puerto por defecto 5580.
  ```

- [ ] **Paso 2 (S1): ¿el sistema aguanta?** Arranca el emulador de desarrollo (receta). Luego:

  ```bash
  EMU; "$ADB" -s "$S" shell 'a=$(pidof system_server); f=$(pidof surfaceflinger); sleep 120; echo "system_server $a -> $(pidof system_server)"; echo "surfaceflinger $f -> $(pidof surfaceflinger)"; logcat -d -b crash | grep -c -i surfaceflinger'
  ```

  Sirve si los dos pid no cambian y el recuento es `0`. **Si no sirve:** anótalo (con `logcat -d -b crash | tail -40`), para el emulador y detente con `BLOQUEADO: la imagen android-37.0 google_apis reinicia el sistema (surfaceflinger); la app no puede correr en ella`. Cambiar de imagen lo decide el usuario (spec, ruling de M2.5b) y necesitaría que `scripts/emulator.sh` leyera también la imagen (es de puffin). No sigas.

- [ ] **Paso 3 (S2): línea base.** Anota la salida de:

  ```bash
  EMU; "$ADB" -s "$S" shell 'for k in adb_enabled adb_wifi_enabled; do echo "$k=$(settings get global $k)"; done; getprop init.svc.adbd; getprop service.adb.tcp.port; getprop persist.adb.tcp.port; netstat -ltn 2>/dev/null | grep -E ":5555 " || echo "nada en 5555"'
  ```

- [ ] **Paso 4: el registrador de estados.** Crea `/tmp/adbuddy-m25b-timeline.sh` con la herramienta Write:

  ```bash
  #!/usr/bin/env bash
  # The emulator's state in its own adb server, once a second, for $1 seconds.
  export ADBUDDY_EMU_AVD=adbuddy-dev ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060
  . /tmp/adbuddy-m25b-emu.env
  for _ in $(seq 1 "$1"); do
    s=$("$ADBUDDY_EMULATOR_ADB" devices | tr -d '\r' | awk -v s="$ADBUDDY_EMULATOR_SERIAL" '$1 == s { print $2 }')
    echo "$(date +%T) ${s:-gone}"
    sleep 1
  done
  ```

  Cada vez que el plan dice "registra N s", lánzalo **en segundo plano** (`bash /tmp/adbuddy-m25b-timeline.sh N > /tmp/adbuddy-m25b-tl.txt`, con `run_in_background`) justo antes de la orden, y al terminar copia al ledger las líneas donde cambia el estado.

- [ ] **Paso 5 (S3, R1): corte desde el shell con restauración programada dentro del emulador.** Registra 120 s y lanza:

  ```bash
  EMU; "$ADB" -s "$S" shell 'setsid sh -c "sleep 20; settings put global adb_enabled 1" </dev/null >/dev/null 2>&1 & sleep 1; settings put global adb_enabled 0'
  ```

  Anota: ¿sale `emulator-5600` de `device` y en cuántos segundos? ¿Vuelve a `device` solo? Si queda `offline` al final, prueba `EMU; "$ADB" -s "$S" reconnect` (registra 60 s) y anota si vuelve. Si sigue sin volver, haz aquí la segunda mitad del paso 8 (S6 y S7: `stop` con adb cortado y arranque de nuevo) y anótala; el emulador queda arrancado para el paso 6. Si volvió (sola, con `reconnect` o tras S6/S7), ADB-ON antes de seguir.

- [ ] **Paso 6 (S3, R2): lo mismo como root, fuera del grupo de procesos de adbd** (al parar `adbd`, init mata su grupo, y con él lo que lanzó su shell). Solo si R1 no volvió solo. Registra 120 s y lanza:

  ```bash
  EMU; "$ADB" -s "$S" shell 'su 0 setsid sh -c "echo \$\$ >/sys/fs/cgroup/cgroup.procs; sleep 20; settings put global adb_enabled 1" </dev/null >/dev/null 2>&1 & sleep 1; settings put global adb_enabled 0'
  ```

  Anota lo mismo que en R1. Es informativo (dice si existe una red de seguridad desde el shell); la decisión depende de R3.

- [ ] **Paso 7 (S4, R3 y S5): Reactivar desde la app, como hará el test.** Crea el fichero temporal `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessOnEmulatorTest.kt` con este contenido exacto (es el de la Tarea 4, variante A):

  ```kotlin
  package tech.hybriq.adbuddy

  import android.os.Build
  import android.util.Log
  import androidx.test.ext.junit.runners.AndroidJUnit4
  import androidx.test.platform.app.InstrumentationRegistry
  import java.io.IOException
  import java.net.InetSocketAddress
  import java.net.Socket
  import org.junit.Assert.assertTrue
  import org.junit.Assume.assumeTrue
  import org.junit.Test
  import org.junit.runner.RunWith
  import tech.hybriq.adbuddy.control.CutAccess

  /**
   * Validation 6 of M2, the app's half: Cut access and, a while later, Reactivar, with the app's own code
   * (CutAccess, as its buttons do). Cutting closes adb, and with it whoever started this test, so it only runs
   * when host/internal/emutest starts it without waiting (`am instrument` without -w, with -e adbuddyCut 1) and
   * watches the emulator from outside; it reports in logcat (tag ADBuddyEmuCut). In connectedDebugAndroidTest it
   * is skipped.
   */
  @RunWith(AndroidJUnit4::class)
  class CutAccessOnEmulatorTest {
      @Test fun cutThenReactivate() {
          val args = InstrumentationRegistry.getArguments()
          assumeTrue("only host/internal/emutest runs it", args.getString("adbuddyCut") == "1")
          // Never on a phone, whoever starts it: the emulator's hardware is ranchu (goldfish before it).
          assertTrue("not an emulator (Build.HARDWARE=${Build.HARDWARE}): Cut access is only tested on one",
              Build.HARDWARE == "ranchu" || Build.HARDWARE == "goldfish")
          val after = args.getString("reactivateAfterMs")?.toLongOrNull() ?: 15_000L
          val context = InstrumentationRegistry.getInstrumentation().targetContext
          val resolver = context.contentResolver
          log("listening before the cut: ${listening()}")
          val cut = CutAccess.cut(context)
          log("cut=$cut isCut=${CutAccess.isCut(resolver)}")
          // adbd stops a moment after the setting changes.
          var open = listening()
          for (i in 0 until 10) {
              if (!open) break
              Thread.sleep(500)
              open = listening()
          }
          log("listening after the cut: $open")
          Thread.sleep(after)
          val restored = CutAccess.restore(context)
          log("restored=$restored isCut=${CutAccess.isCut(resolver)}")
          assertTrue("Cut access failed", cut)
          assertTrue("Reactivar failed", restored)
      }

      /** Whether something (adbd in tcpip mode) accepts connections on 127.0.0.1:5555. */
      private fun listening(): Boolean = try {
          Socket().use { it.connect(InetSocketAddress("127.0.0.1", 5555), 1000) }
          true
      } catch (e: IOException) {
          false
      }

      private fun log(line: String) {
          Log.i(TAG, line)
      }

      private companion object {
          const val TAG = "ADBuddyEmuCut"
      }
  }
  ```

  Compila e instala (el emulador sigue encendido; nada de Gradle toca un dispositivo aquí):

  ```bash
  cd android && gradle -q :app:assembleDebug :app:assembleDebugAndroidTest && cd ..
  EMU; "$ADB" -s "$S" install -r --no-incremental android/app/build/outputs/apk/debug/app-debug.apk
  EMU; "$ADB" -s "$S" install -r --no-incremental android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
  EMU; "$ADB" -s "$S" shell 'pm grant tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS; logcat -c'
  ```

  **R3:** CI-LIBRE; registra 150 s y lanza (sin `-w`: la orden vuelve enseguida y el test sigue dentro del emulador):

  ```bash
  EMU; "$ADB" -s "$S" shell 'am instrument -r -e class tech.hybriq.adbuddy.CutAccessOnEmulatorTest#cutThenReactivate -e adbuddyCut 1 -e reactivateAfterMs 15000 tech.hybriq.adbuddy.test/androidx.test.runner.AndroidJUnitRunner'
  ```

  Cuando acabe el registro: si `emulator-5600` está en `device`, anota `EMU; "$ADB" -s "$S" shell 'logcat -d -s ADBuddyEmuCut:I; settings get global adb_enabled; settings get global adb_wifi_enabled'`. Si quedó `offline`, prueba `EMU; "$ADB" -s "$S" reconnect` (registra 60 s) y anota si eso lo trae. Anota: segundos hasta salir de `device`, segundos hasta volver, si hizo falta `reconnect`, y las líneas de `ADBuddyEmuCut`. Si `adb_enabled` no es `1` al volver, ADB-ON (y anótalo: Reactivar no lo dejó como estaba).

  **S5 (modo tcpip):** con el emulador en `device`, registra 90 s y lanza `EMU; "$ADB" -s "$S" tcpip 5555`. Anota si `emulator-5600` vuelve a `device` (no vale que aparezca otro número de serie, como `127.0.0.1:5601`) y `EMU; "$ADB" -s "$S" shell getprop service.adb.tcp.port`. Si volvió, repite R3 en modo tcpip (mismas órdenes) y anota lo mismo, y después `EMU; "$ADB" -s "$S" usb` (registra 60 s, anota si vuelve).

- [ ] **Paso 8 (S6): limpieza con adb cortado.** Comprueba que el `stop` del script apaga un emulador sin adb (lo que hará el paso "Stop the emulator" de la CI si un test se queda a medias). Registra 30 s y corta sin restauración:

  ```bash
  EMU; "$ADB" -s "$S" shell 'settings put global adb_enabled 0'
  ```

  Cuando el registro muestre que salió de `device`, ejecuta `EMU; scripts/emulator.sh stop` y la espera de SIN-EMULADOR tras un `stop` (Restricciones globales). Anota si quedó vacío sin tocar nada. Si no: apágalo **solo a él** como dicen las Restricciones globales (`-port 5600`), y anótalo: entonces (iv) no se cumple.

  **S7: ¿arranca con adb tras un corte que nadie deshizo?** `adb_enabled=0` queda guardado en los datos del AVD, y `-no-snapshot` no los borra. CI-LIBRE y SIN-EMULADOR; registra 720 s en segundo plano y arranca otra vez con la receta (hasta 10 min). Anota si llega a `device` y, si llega, **si sigue en `device` al menos 120 s después** del arranque (el sistema puede aplicar `adb_enabled=0` y parar adbd ya arrancado). Si se mantiene: ADB-ON. Si no llega o se cae: para el emulador (con la espera de SIN-EMULADOR), aplica la "Recuperación del AVD de desarrollo", arranca de nuevo y ADB-ON, anotándolo todo. Esto decide si un test que falle a medias puede dejar inservible el AVD de la CI, que el runner conserva entre jobs. Al acabar, para el emulador (con la espera de SIN-EMULADOR).

- [ ] **Paso 9: deja el árbol como estaba.**

  ```bash
  rm android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessOnEmulatorTest.kt /tmp/adbuddy-m25b-timeline.sh /tmp/adbuddy-m25b-tl.txt
  git status --short   # solo el ledger, sin seguimiento
  ```

  El emulador queda apagado (paso 8).

- [ ] **Paso 10: aplica la regla de decisión** y escríbela en el ledger como rulings:

  - **Variante A** si: (i) con R3 `emulator-5600` sale de `device` en menos de 30 s; (ii) vuelve a `device` por sí solo en menos de 105 s desde el corte (15 s de espera de la app más 90 s); (iii) `ADBuddyEmuCut` dice `cut=true isCut=true` y `restored=true isCut=false`, y `adb_enabled` vuelve a `1`; (iv) S6 apaga el emulador con `scripts/emulator.sh stop` (sin `Stop-Process`); y (v) en S7 el emulador vuelve a arrancar con adb sin recuperación y lo mantiene 120 s (si no, un fallo a medias del test dejaría sin adb, en el siguiente job, el AVD que la CI conserva).
  - **Variante A'** si se cumple todo menos (ii), y (ii) se cumple con un `adb -s emulator-5600 reconnect` desde el PC.
  - **Variante A en local** (combinable con A') si se cumple todo menos (v): el test existe, pero solo corre con `ADBUDDY_EMULATOR_CUT=1`; la CI no lo pone. La 6 queda automatizada en local y el motivo de que la CI no la corra, en el ledger.
  - **Variante B** en cualquier otro caso. El motivo (cuál de (i)-(v) falló, con la medida) va al ledger y, en la Tarea 6, a la lista manual.
  - **`tcpipOnEmulator = true`** solo si en S5 `emulator-5600` volvió a `device` tras `adb tcpip 5555`, R3 en modo tcpip cumplió (i)-(iii) con, además, `listening before the cut: true` y `listening after the cut: false`, y `adb usb` lo devolvió a `device`; si no, `false`, con el motivo. (Sin modo tcpip nada escucha en 5555 ni antes ni después: esas líneas solo dicen algo en modo tcpip.)

  Formato: `Ruling: variante <A|A'|A en local|A' en local|B> de la Tarea 4 — <medidas> — si me equivoco, <coste>.` y `Ruling: tcpipOnEmulator=<true|false> — <medidas> — …`. Copia también al ledger la nota de los puertos fuera del rango de adb (paso 1) con lo que se midió en (ii).

- [ ] **Paso 11: commit del ledger.**

  ```bash
  git add -f .superpowers/sdd/2026-10-03-adbuddy-m2.5b-emulador/progress.md
  git commit -m "docs(m2.5b): spike del emulador (Cortar acceso y Reactivar)

  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
  ```

---

