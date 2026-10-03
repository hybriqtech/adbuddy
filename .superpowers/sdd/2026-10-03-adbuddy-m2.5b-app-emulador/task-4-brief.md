### Tarea 4: validación 6, Cortar acceso y Reactivar (variante según el spike)

**Implementador:** `adbuddy-implementer-opus` (criterio: integración entre el host y el instrumentado, adb que desaparece). **Revisor:** `adbuddy-reviewer-opus` (Cortar acceso es seguridad).

Lee los rulings de la Tarea 1 en el ledger y haz **solo** la variante que dicen. A, A', "A en local" y "A' en local" comparten ficheros (A' y "en local" son un bloque más sobre A); B tiene los suyos.

**Ficheros:**
- Variantes A y A': crear `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessOnEmulatorTest.kt` y `host/internal/emutest/cut_test.go`; modificar `host/README.md`.
- Variante B: crear `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessWifiTest.kt`.

**Interfaces:**
- Consume (Tarea 3): `newEmu`, `(e *emu) run/shell/state/waitState/installApp/installTests`. Consume (app): `CutAccess.cut(Context): Boolean`, `CutAccess.restore(Context): Boolean`, `CutAccess.isCut(ContentResolver): Boolean`, `AccessSwitch.ADB_WIFI_ENABLED`; (host) `phoneapp.Grant(ctx, a, v) error`.
- Produce para la Tarea 5: en A/A', el test `TestCutAccessAndReactivate` dentro de `./internal/emutest` (la CI ya corre el paquete entero); en "en local", el mismo test, que la CI salta porque no pone `ADBUDDY_EMULATOR_CUT`; en B, `CutAccessWifiTest` dentro de `connectedDebugAndroidTest`.

#### Variantes A y A'

- [ ] **Paso A1: la mitad de la app.** Crea `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessOnEmulatorTest.kt`:

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

- [ ] **Paso A2: el test del host.** Crea `host/internal/emutest/cut_test.go` con este contenido, poniendo en `tcpipOnEmulator` el valor del ruling de la Tarea 1 (`true` o `false`):

  ```go
  package emutest

  import (
  	"strconv"
  	"strings"
  	"testing"
  	"time"

  	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
  )

  // The app's instrumented half of the cut (android/app/src/androidTest/…/CutAccessOnEmulatorTest.kt).
  const (
  	cutTest   = "tech.hybriq.adbuddy.CutAccessOnEmulatorTest#cutThenReactivate"
  	cutRunner = "tech.hybriq.adbuddy.test/androidx.test.runner.AndroidJUnitRunner"
  	// How long the app waits between Cut access and Reactivar.
  	reactivateAfter = 15 * time.Second
  )

  // tcpipOnEmulator says whether `adb tcpip 5555` keeps the emulator under its
  // emulator-<port> serial (Task 1 of the M2.5b plan, spike S5).
  const tcpipOnEmulator = false

  // stuckHint is what to do when the emulator's adb does not come back.
  const stuckHint = "its adb is still cut: stop it with scripts/emulator.sh stop (same ADBUDDY_EMU_* variables) and start it again"

  // Validation 6 of M2: Cut access closes the emulator's adb and Reactivar
  // brings it back, both done by the app's own code (CutAccess), in
  // CutAccessOnEmulatorTest. The test starts it without waiting for it (`am
  // instrument` without -w): the cut takes away the adb that started it. It
  // watches the emulator from outside, then reads what the app logged.
  func TestCutAccessAndReactivate(t *testing.T) {
  	e := newEmu(t)
  	v := e.installApp(t)
  	e.installTests(t)
  	if err := phoneapp.Grant(e.ctx, e.adb, v); err != nil {
  		t.Fatalf("pm grant: %v", err)
  	}
  	t.Run("usb", func(t *testing.T) { e.cutCycle(t, false) })
  	t.Run("tcpip", func(t *testing.T) {
  		if !tcpipOnEmulator {
  			t.Skip("`adb tcpip` takes the emulator off its emulator-<port> serial (M2.5b plan, Task 1): validation 6 with tcpip mode stays manual")
  		}
  		e.cutCycle(t, true)
  	})
  }

  // adbSettings reads the two settings Cut access changes. A setting never
  // written ("null") counts as what AccessSwitch takes it for: adb on, wireless
  // debugging off.
  func (e *emu) adbSettings(t *testing.T) string {
  	t.Helper()
  	get := func(name, unset string) string {
  		v := e.shell(t, "settings get global "+name)
  		if v == "null" {
  			v = unset
  		}
  		return name + "=" + v
  	}
  	return get("adb_enabled", "1") + " " + get("adb_wifi_enabled", "0")
  }

  // cutCycle runs one Cut access and Reactivar, with or without tcpip mode,
  // and checks that the settings end as they began.
  func (e *emu) cutCycle(t *testing.T, tcpip bool) {
  	before := e.adbSettings(t)
  	if strings.Contains(before, "adb_enabled=0") {
  		t.Fatalf("%s before Cut access: a cut left over by an earlier run; put it back with `adb shell settings put global adb_enabled 1`", before)
  	}
  	if tcpip {
  		e.run(t, "tcpip", "5555")
  		if s, ok := e.waitState(func(s string) bool { return s == "device" }, 60*time.Second); !ok {
  			t.Fatalf("the emulator is %q after adb tcpip", s)
  		}
  		if p := e.shell(t, "getprop service.adb.tcp.port"); p != "5555" {
  			t.Fatalf("service.adb.tcp.port = %q after adb tcpip 5555", p)
  		}
  		defer func() {
  			e.run(t, "usb")
  			if s, ok := e.waitState(func(s string) bool { return s == "device" }, 60*time.Second); !ok {
  				t.Errorf("the emulator is %q after adb usb", s)
  			}
  		}()
  	}
  	e.shell(t, "logcat -c")
  	e.shell(t, "am instrument -r -e class "+cutTest+" -e adbuddyCut 1 -e reactivateAfterMs "+
  		strconv.FormatInt(reactivateAfter.Milliseconds(), 10)+" "+cutRunner)
  	// The cut: the emulator's adb goes away.
  	if s, ok := e.waitState(func(s string) bool { return s != "device" }, 30*time.Second); !ok {
  		t.Fatalf("the emulator is still %q 30 s after Cut access", s)
  	}
  	// Reactivar, reactivateAfter later: it comes back by itself.
  	if s, ok := e.waitState(func(s string) bool { return s == "device" }, reactivateAfter+90*time.Second); !ok {
  		t.Fatalf("the emulator is %q after Reactivar: %s", s, stuckHint)
  	}
  	// Without tcpip mode nothing listens on 5555 before or after: those
  	// lines only say something in tcpip mode.
  	want := []string{"cut=true isCut=true", "restored=true isCut=false"}
  	if tcpip {
  		want = append(want, "listening before the cut: true", "listening after the cut: false")
  	}
  	// The app's last line may reach logcat a moment after adb is back.
  	var logs string
  	for range 5 {
  		logs = e.shell(t, "logcat -d -s ADBuddyEmuCut:I")
  		if missing(logs, want) == "" {
  			break
  		}
  		time.Sleep(time.Second)
  	}
  	if m := missing(logs, want); m != "" {
  		t.Errorf("the app did not log %q:\n%s", m, logs)
  	}
  	if after := e.adbSettings(t); after != before {
  		t.Errorf("after Reactivar %s, before Cut access %s", after, before)
  	}
  }

  // missing returns the first of want that logs lacks, or "".
  func missing(logs string, want []string) string {
  	for _, w := range want {
  		if !strings.Contains(logs, w) {
  			return w
  		}
  	}
  	return ""
  }
  ```

  **Solo en la variante A'** (el emulador vuelve con un `reconnect` desde el PC): sustituye el bloque

  ```go
  	// Reactivar, reactivateAfter later: it comes back by itself.
  	if s, ok := e.waitState(func(s string) bool { return s == "device" }, reactivateAfter+90*time.Second); !ok {
  		t.Fatalf("the emulator is %q after Reactivar: %s", s, stuckHint)
  	}
  ```

  por

  ```go
  	// Reactivar, reactivateAfter later. The emulator's transport stays
  	// offline after adbd comes back until the PC reconnects it (Task 1 of the
  	// M2.5b plan): only this emulator, never the whole server.
  	back := func(s string) bool {
  		if s == "offline" {
  			_, _, _, _ = e.adb.Output(e.ctx, "reconnect")
  		}
  		return s == "device"
  	}
  	if s, ok := e.waitState(back, reactivateAfter+90*time.Second); !ok {
  		t.Fatalf("the emulator is %q after Reactivar: %s", s, stuckHint)
  	}
  ```

  **Solo en "A en local" o "A' en local"** (todo se cumplió menos el arranque con adb tras un corte no deshecho, S7): en `TestCutAccessAndReactivate`, sustituye

  ```go
  	e := newEmu(t)
  	v := e.installApp(t)
  	e.installTests(t)
  ```

  por

  ```go
  	e := newEmu(t)
  	// A run that fails half way leaves adb_enabled=0 in the AVD's data, and the
  	// emulator then boots without adb (M2.5b plan, Task 1, S7): the CI, which
  	// keeps its AVD between jobs, does not run it.
  	if os.Getenv("ADBUDDY_EMULATOR_CUT") != "1" {
  		t.Skip("set ADBUDDY_EMULATOR_CUT=1 to run it, on a development emulator only")
  	}
  	v := e.installApp(t)
  	e.installTests(t)
  ```

  y añade `"os"` a los imports de `cut_test.go`.

- [ ] **Paso A3: sin emulador.** Desde `host/`: `gofmt -l ./internal/emutest`, `go vet` con los tres `GOOS`, y `env -u ADBUDDY_EMULATOR_SERIAL go test -count=1 -timeout 120s -v ./internal/emutest` → `TestCutAccessAndReactivate` en `SKIP`. Y `cd android && gradle -q :app:assembleDebugAndroidTest` → `exit 0`.

- [ ] **Paso A4: el instrumentado se salta en la suite normal.** CI-LIBRE y SIN-EMULADOR, arranca el de desarrollo y `EMU; cd android && gradle :app:connectedDebugAndroidTest` → `exit 0`; en el XML, `CutAccessOnEmulatorTest.cutThenReactivate` como `skipped` y el emulador sigue en `device` al acabar (`EMU; "$ADB" devices`).

- [ ] **Paso A5: RED con una mutación.** En `CutAccessOnEmulatorTest.kt` cambia temporalmente `val restored = CutAccess.restore(context)` por `val restored = false` y compila (`cd android && gradle -q :app:assembleDebugAndroidTest`). Corre `EMU; cd host && go test -count=1 -timeout 10m -run 'TestCutAccessAndReactivate/usb' -v ./internal/emutest`. Esperado: `FAIL` con `the emulator is "…" after Reactivar` (adb no vuelve). Recupera el emulador con `EMU; scripts/emulator.sh stop` y la espera de SIN-EMULADOR (el spike comprobó que lo apaga con adb cortado), arráncalo de nuevo (en "A en local", con la "Recuperación del AVD de desarrollo" si no llega a `device`) y **ADB-ON** antes de seguir: la mutación dejó `adb_enabled=0` en los datos del AVD. Revierte la mutación y recompila.

- [ ] **Paso A6: GREEN.** CI-LIBRE; `EMU; cd host && go test -count=1 -timeout 15m -v ./internal/emutest` (en "A en local", con `ADBUDDY_EMULATOR_CUT=1` delante) → `PASS`: los cuatro de la Tarea 3 y `TestCutAccessAndReactivate/usb` verdes; `TestCutAccessAndReactivate/tcpip` verde si `tcpipOnEmulator = true` o `SKIP` con su motivo si `false`. Copia el `-v` al informe. Para el emulador.

- [ ] **Paso A7: README.** En `host/README.md`, en la sección `### The app on the emulator`, sustituye

  ```text
  the provider answers the shell user, the handshake on stdin reaches it, and `BridgeService` goes to the foreground after it (validations 1, 2 and 3 of the M2 plan).
  ```

  por

  ```text
  the provider answers the shell user, the handshake on stdin reaches it, `BridgeService` goes to the foreground after it, and Cut access closes the emulator's adb until Reactivar brings it back (validations 1, 2, 3 and 6 of the M2 plan). The cut runs inside the app (`CutAccessOnEmulatorTest`, started without waiting for it), so a failure can leave the emulator without adb: `scripts/emulator.sh stop` still stops it.
  ```

  En "A en local" / "A' en local", añade a ese texto, al final: ``It only runs with `ADBUDDY_EMULATOR_CUT=1`, and only on a development emulator: a run that fails half way leaves `adb_enabled=0` in the AVD, which then boots without adb (delete the AVD's `userdata-qemu.img*` to recover it). The CI does not run it.``

- [ ] **Paso A8: commit.**

  ```bash
  git add android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessOnEmulatorTest.kt host/internal/emutest/cut_test.go host/README.md
  git commit -m "test: Cortar acceso y Reactivar en el emulador, desde la app y vistos desde el host (validación 6 de M2)

  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
  ```

#### Variante B

- [ ] **Paso B1: la mitad de `adb_wifi_enabled`.** Crea `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessWifiTest.kt`:

  ```kotlin
  package tech.hybriq.adbuddy

  import android.os.ParcelFileDescriptor
  import android.provider.Settings
  import androidx.test.ext.junit.runners.AndroidJUnit4
  import androidx.test.platform.app.InstrumentationRegistry
  import org.junit.After
  import org.junit.Assert.assertEquals
  import org.junit.Assert.assertFalse
  import org.junit.Assume.assumeTrue
  import org.junit.Before
  import org.junit.Test
  import org.junit.runner.RunWith
  import tech.hybriq.adbuddy.control.AccessSwitch
  import tech.hybriq.adbuddy.control.CutAccess

  /**
   * Validation 6 of M2, the half the emulator can show (variant B of the M2.5b plan): with WRITE_SECURE_SETTINGS
   * granted as `adbuddy setup` grants it, the app itself (its uid, the calls CutAccess makes) writes
   * adb_wifi_enabled and reads it back. The adb_enabled half takes the emulator's adb away for good and stays a
   * manual validation, with the reason in the plan.
   */
  @RunWith(AndroidJUnit4::class)
  class CutAccessWifiTest {
      private val instrumentation = InstrumentationRegistry.getInstrumentation()
      private val context = instrumentation.targetContext
      private val resolver = context.contentResolver
      private val name = AccessSwitch.ADB_WIFI_ENABLED
      private var original = ""

      private fun shell(command: String): String {
          val pfd = instrumentation.uiAutomation.executeShellCommand(command)
          return ParcelFileDescriptor.AutoCloseInputStream(pfd).use { it.readBytes().decodeToString() }.trim()
      }

      @Before fun grantAsSetupDoes() {
          original = shell("settings get global $name")
          assumeTrue("wireless debugging is on in this emulator ($name=$original)", original == "null" || original == "0")
          shell("pm grant ${context.packageName} android.permission.WRITE_SECURE_SETTINGS")
      }

      @After fun putBack() {
          shell("pm revoke ${context.packageName} android.permission.WRITE_SECURE_SETTINGS")
          if (original == "null") shell("settings delete global $name") else shell("settings put global $name $original")
      }

      @Test fun theAppWritesAndReadsAdbWifiEnabled() {
          // What CutAccess's settings do: putInt to write, getString to read back.
          Settings.Global.putInt(resolver, name, 0)
          assertEquals("0", shell("settings get global $name"))
          assertEquals(0, Settings.Global.getString(resolver, name)?.toIntOrNull())
          assertFalse("adb counts as cut", CutAccess.isCut(resolver))
      }
  }
  ```

- [ ] **Paso B2: RED con una mutación.** Cambia temporalmente en `grantAsSetupDoes` la línea del `pm grant` por un comentario (`// shell("pm grant …")`) y corre (CI-LIBRE y SIN-EMULADOR, emulador de desarrollo arrancado, CI-LIBRE) `EMU; cd android && gradle :app:connectedDebugAndroidTest`. Esperado: `CutAccessWifiTest.theAppWritesAndReadsAdbWifiEnabled` falla con `SecurityException` (falta `WRITE_SECURE_SETTINGS`). Revierte.

- [ ] **Paso B3: GREEN.** `EMU; cd android && gradle :app:connectedDebugAndroidTest; echo "exit $?"` → `exit 0`, con `CutAccessWifiTest` 1, `AdbWifiObserverTest` 3, `TokenProviderTest` 7 y `LocalesTest` 5. (El permiso no hace falta comprobarlo después: AGP desinstala la app al terminar, y `putBack` lo revoca antes.) Para el emulador (con la espera de SIN-EMULADOR).

- [ ] **Paso B4: commit.**

  ```bash
  git add android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/CutAccessWifiTest.kt
  git commit -m "test(android): la mitad de adb_wifi_enabled de Cortar acceso (validación 6 de M2, variante B)

  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
  ```

---


---
# Notas del controlador (mandan sobre el texto de arriba)
- Variante: **"A en local"** (ledger, ruling del 2026-10-02; tcpipOnEmulator=true, documentando que la prueba de tcpip es débil). El test del host solo corre con `ADBUDDY_EMULATOR_CUT=1`; la CI no lo pone. Si el texto de arriba usa `connectedDebugAndroidTest` para algo, no: los instrumentados van por `am instrument` desde `emutest` (ya existe `TestInstrumentedSuite` en `host/internal/emutest/instrumented_test.go` y `parseInstrument`). **`TestInstrumentedSuite` falla hoy ante un test saltado (código -4)**: `CutAccessOnEmulatorTest.cutThenReactivate` se salta sin `-e adbuddyCut 1` (assumeTrue), así que añade a la suite una lista explícita y mínima de saltos permitidos (esa clase.método) y test unitario de `parseInstrument` para ello; cualquier otro salto sigue fallando. Ten cuidado de que el `-w` de `TestInstrumentedSuite` NO pase `adbuddyCut`.
- Fondo del emulador: `ADBUDDY_EMU_IMAGE='system-images;android-34;google_apis;x86_64' ADBUDDY_EMU_AVD=adbuddy-dev34 ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060 ADBUDDY_EMU_STABLE_SECS=120 ADB_MDNS=0 ADB_MDNS_AUTO_CONNECT=0`, JAVA_HOME=/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1, PATH con /c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin; `eval "$(scripts/emulator.sh start)"` (tarda ~2,5 min); antes: `tasklist | grep -i 'qemu\|emulator'` vacío y RAM libre >= 12 GB; después `scripts/emulator.sh stop` y `tasklist` otra vez. Solo el emulador `emulator-5600` en el servidor adb 5060; JAMÁS el servidor 5037 ni el móvil del usuario (verifica `adb -P 5060 devices` antes de cada paso). El corte deja el AVD sin adb si algo sale mal: el test tiene `finally` que restaura y, si el emulador queda sin adb, se documenta que hay que borrar los datos del AVD (`adbuddy-dev34`, nunca otro). Ejecuta el test una vez completo con emulador y registra la salida en el informe.
- Informe en tu respuesta final; no lances subagentes; commits con trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Rama feat/m2.5b-app-emulador, BASE = HEAD actual.
