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
- **Móvil real fuera de alcance (hallazgo del spike, 2026-10-02):** el servidor adb propio del emulador autoconectó por mDNS el móvil real del usuario. Regla de TODOS los preludios, pasos y guardas de Gradle: `ANDROID_SERIAL=emulator-<n>` obligatorio (y `-s emulator-<n>` en cada orden de adb), `ADB_MDNS=0` y `ADB_MDNS_AUTO_CONNECT=0` en el servidor propio, y comprobar antes de cada paso que `adb devices` del servidor propio no lista ningún dispositivo que no sea `emulator-<n>`; si aparece uno, parar y escalar. `scripts/emulator.sh` lo hará él mismo (PR de puffin); el plan no lo da por hecho en los pasos.
- **GPU del emulador (hallazgo del spike):** con la imagen android-37.0 y `swiftshader_indirect`, `surfaceflinger` aborta en bucle (S1). Tras el PR de puffin se reintenta con `ADBUDDY_EMU_GPU=swangle_indirect`; si sigue inestable, escalar al controlador con el log (cambiar de imagen o de GPU en la CI no es de este plan).
- **No cambies la app ni el protocolo** (`android/app/src/main`, `protocol/`), salvo las mutaciones temporales que un paso pide y revierte en el mismo paso. Si un test revela un fallo de la app, detente y repórtalo: el arreglo es otra tarea.
- **No instales paquetes del SDK** ni imágenes de sistema ni nada que pida administrador. Si falta algo, `BLOQUEADO`.
- **Commits:** identidad local del repo (hybriqtech), nunca `git config --global`. Cada mensaje termina con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, exactamente así. `git add` con rutas explícitas, nunca `-A`.
- **Privacidad:** ni seriales ni IPs reales en el repo; `emulator-5600`, `emulator-5590` y `emulator-5580` son los únicos de este plan.
- **Revisores:** no arrancan el emulador; leen la evidencia del informe (órdenes y salidas). Límite de 30 min por revisión y `go test -timeout` en lo que ejecuten. Sin subagentes dentro de una tarea.

### Tarea 2: instrumentados en el emulador — guarda, validación 1 (lado de la app) y validación 10

**Implementador:** `adbuddy-implementer` (Sonnet). **Revisor:** `adbuddy-reviewer-opus` (TokenProvider y la guarda de seguridad).

**Ficheros:**
- Modificar: `android/app/build.gradle.kts` (guarda antes de `dependencies {`)
- Crear: `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/AdbWifiObserverTest.kt`
- Modificar: `android/README.md` (párrafo de los instrumentados)
- Sin cambios: `TokenProviderTest.kt` y `LocalesTest.kt` (ya existen; aquí corren por primera vez)

**Interfaces:**
- Consume: `AccessSwitch.ADB_WIFI_ENABLED` (`"adb_wifi_enabled"`, `tech.hybriq.adbuddy.control`), `UiAutomation.adoptShellPermissionIdentity(String...)` / `dropShellPermissionIdentity()` (API 29+), `Settings.Global.getUriFor/putString/getString`.
- Produce: la guarda de Gradle (todo task cuyo nombre empieza por `connected` exige que `ANDROID_SERIAL` sea exactamente `emulator-<número>`), en la que se apoyan las tareas 4 y 5.

- [ ] **Paso 1: la guarda.** En `android/app/build.gradle.kts`, sustituye

  ```kotlin
  dependencies {
      implementation(libs.androidx.core.ktx)
  ```

  por

  ```kotlin
  // The instrumented tests run only on an emulator, never on the user's phone (M2.5 spec §2.4): ANDROID_SERIAL
  // must name exactly one emulator (AGP splits it on commas, so "emulator-5554,<phone>" is refused too). Checked
  // when a connected task starts, before it installs anything.
  tasks.configureEach {
      if (name.startsWith("connected")) {
          doFirst {
              val serial = System.getenv("ANDROID_SERIAL").orEmpty()
              check(Regex("emulator-[0-9]+").matches(serial)) {
                  "connected tests run only on one emulator: set ANDROID_SERIAL=emulator-<port> " +
                      "(scripts/emulator.sh start prints it as ADBUDDY_EMULATOR_SERIAL), not \"$serial\""
              }
          }
      }
  }

  dependencies {
      implementation(libs.androidx.core.ktx)
  ```

- [ ] **Paso 2: la guarda se niega (RED de la guarda).** CI-LIBRE y SIN-EMULADOR, y arranca el emulador de desarrollo (receta). Con el servidor del emulador cargado y números de serie que no son exactamente un emulador:

  ```bash
  EMU; cd android && ANDROID_SERIAL=not-an-emulator gradle :app:connectedDebugAndroidTest; echo "exit $?"
  EMU; cd android && ANDROID_SERIAL=emulator-5600,x gradle :app:connectedDebugAndroidTest; echo "exit $?"
  EMU; cd android && ANDROID_SERIAL= gradle :app:connectedDebugAndroidTest; echo "exit $?"
  ```

  Esperado en los tres: `exit 1` con `connected tests run only on one emulator: set ANDROID_SERIAL=emulator-<port> … not "<lo que se pasó>"` (`not-an-emulator`, `emulator-5600,x` y vacío), y en la salida ninguna línea de instalación (`Installing APK`). El segundo es el importante: AGP parte `ANDROID_SERIAL` por comas, y una guarda por prefijo dejaría pasar `emulator-5600,<serie del móvil>`.

- [ ] **Paso 3: los instrumentados que ya existen.**

  ```bash
  EMU; cd android && gradle :app:connectedDebugAndroidTest; echo "exit $?"
  ```

  Esperado: `exit 0`; en `android/app/build/outputs/androidTest-results/connected/debug/` (XML) `TokenProviderTest` 7 tests y `LocalesTest` 5, sin fallos. Anota en el informe la línea de resumen de Gradle (`Finished N tests on emulator-5600`). Si alguno falla, detente y repórtalo con el XML del fallo (no cambies la app).

- [ ] **Paso 4: mutación de la validación 1.** Prueba que `TokenProviderTest` puede fallar: en `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/TokenProvider.kt` cambia temporalmente

  ```kotlin
      private fun isShell() = Caller.isShell(Binder.getCallingUid())
  ```

  por

  ```kotlin
      private fun isShell() = true
  ```

  y corre `EMU; cd android && gradle :app:connectedDebugAndroidTest -Pandroid.testInstrumentationRunnerArguments.class=tech.hybriq.adbuddy.TokenProviderTest`. Esperado: fallan `callsFromAnAppAreRefused`, `handshakeFromAnAppIsRefused`, `writesFromAnAppAreRefused` y `getTypeAndReadingFromAnAppAreRefused` (`queryFromAnAppGivesNoData` sigue verde: el proveedor no da cursor a nadie). Revierte con `git checkout -- android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/TokenProvider.kt` y comprueba `git diff --stat` sin ese fichero.

- [ ] **Paso 5: el test de la validación 10.** Crea `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/AdbWifiObserverTest.kt`:

  ```kotlin
  package tech.hybriq.adbuddy

  import android.Manifest
  import android.database.ContentObserver
  import android.os.Handler
  import android.os.Looper
  import android.os.ParcelFileDescriptor
  import android.provider.Settings
  import androidx.test.ext.junit.runners.AndroidJUnit4
  import androidx.test.platform.app.InstrumentationRegistry
  import java.util.concurrent.CountDownLatch
  import java.util.concurrent.TimeUnit
  import org.junit.After
  import org.junit.Assert.assertEquals
  import org.junit.Assert.assertTrue
  import org.junit.Assume.assumeTrue
  import org.junit.Before
  import org.junit.Test
  import org.junit.runner.RunWith
  import tech.hybriq.adbuddy.control.AccessSwitch

  /**
   * Validation 10 of M2 (M2.5 spec §7): an observer of the app on the hidden setting adb_wifi_enabled, registered
   * as CutAccess.observe registers its own, hears a change made by someone else (adb, the Settings app). The
   * change never turns wireless debugging on: the setting goes from "0" (or unset) to "00", which everyone reads as
   * 0 (Settings.Global.getInt, AccessSwitch, the system's own observer), and is put back afterwards.
   */
  @RunWith(AndroidJUnit4::class)
  class AdbWifiObserverTest {
      private val instrumentation = InstrumentationRegistry.getInstrumentation()
      private val resolver = instrumentation.targetContext.contentResolver
      private val name = AccessSwitch.ADB_WIFI_ENABLED
      private var original = ""

      /** A command as the shell user (uid 2000), in a process of its own, as adb runs it. */
      private fun shell(command: String): String {
          val pfd = instrumentation.uiAutomation.executeShellCommand(command)
          return ParcelFileDescriptor.AutoCloseInputStream(pfd).use { it.readBytes().decodeToString() }.trim()
      }

      @Before fun wirelessDebuggingIsOff() {
          original = shell("settings get global $name")
          // The test rewrites the setting: never while wireless debugging is on.
          assumeTrue("wireless debugging is on in this emulator ($name=$original)", original == "null" || original == "0")
      }

      @After fun putTheSettingBack() {
          if (original == "null") shell("settings delete global $name") else shell("settings put global $name $original")
          assertEquals(original, shell("settings get global $name"))
      }

      /** Whether an observer like CutAccess's heard a change of the setting within 10 s of change(). */
      private fun heardAfter(change: () -> Unit): Boolean {
          val heard = CountDownLatch(1)
          val observer = object : ContentObserver(Handler(Looper.getMainLooper())) {
              override fun onChange(selfChange: Boolean) {
                  heard.countDown()
              }
          }
          resolver.registerContentObserver(Settings.Global.getUriFor(name), false, observer)
          try {
              change()
              return heard.await(10, TimeUnit.SECONDS)
          } finally {
              resolver.unregisterContentObserver(observer)
          }
      }

      @Test fun aChangeFromTheShellReachesTheObserver() {
          assertTrue("the app did not hear $name change from the shell", heardAfter { shell("settings put global $name 00") })
      }

      @Test fun aChangeWithTheShellPermissionIdentityReachesTheObserver() {
          val ui = instrumentation.uiAutomation
          assertTrue("the app did not hear $name change with the shell's permissions", heardAfter {
              ui.adoptShellPermissionIdentity(Manifest.permission.WRITE_SECURE_SETTINGS)
              try {
                  Settings.Global.putString(resolver, name, "00")
              } finally {
                  ui.dropShellPermissionIdentity()
              }
          })
      }

      /** CutAccess saves adb_wifi_enabled before a cut so that Reactivar puts it back: the app must be able to read it. */
      @Test fun theAppCanReadAdbWifiEnabled() {
          shell("settings put global $name 0")
          assertEquals("0", Settings.Global.getString(resolver, name))
      }
  }
  ```

- [ ] **Paso 6: mutación (RED del observador).** En el fichero recién creado cambia temporalmente `Settings.Global.getUriFor(name)` por `Settings.Global.getUriFor(AccessSwitch.ADB_ENABLED)` y corre `EMU; cd android && gradle :app:connectedDebugAndroidTest`. Esperado: fallan `aChangeFromTheShellReachesTheObserver` y `aChangeWithTheShellPermissionIdentityReachesTheObserver` con "did not hear", tras unos 10 s cada uno. Vuelve a poner `getUriFor(name)`.

- [ ] **Paso 7: GREEN.** `EMU; cd android && gradle :app:connectedDebugAndroidTest; echo "exit $?"`. Esperado: `exit 0`, con `TokenProviderTest` 7, `LocalesTest` 5 y `AdbWifiObserverTest` 3, sin fallos ni saltados (un saltado de `AdbWifiObserverTest` quiere decir depuración inalámbrica activa en el emulador: detente). Después `EMU; "$ADB" -s "$S" shell settings get global adb_wifi_enabled` debe dar lo mismo que antes del paso 5 (anótalo).

  **Si `theAppCanReadAdbWifiEnabled` falla con `SecurityException`:** no lo cambies ni lo saltes. Es un hallazgo de la app (Android 17 no deja leer el ajuste a una app, `CutAccess` lo toma por "nunca escrito" y Reactivar no devolvería la depuración inalámbrica): detente con `BLOQUEADO` y la traza. El controlador decide.

- [ ] **Paso 8: README.** En `android/README.md` sustituye

  ```text
  Instrumented tests (`gradle :app:connectedDebugAndroidTest`) need an emulator:
  never run them on your own phone.
  ```

  por

  ````text
  Instrumented tests (`gradle :app:connectedDebugAndroidTest`) run only on an
  emulator, never on your own phone: Gradle refuses to start them unless
  `ANDROID_SERIAL` names an emulator (`emulator-<port>`). From the repository root:

  ```sh
  eval "$(scripts/emulator.sh start)"   # the emulator, with an adb server of its own
  cd android && ANDROID_SERIAL="$ADBUDDY_EMULATOR_SERIAL" gradle :app:connectedDebugAndroidTest
  ../scripts/emulator.sh stop
  ```

  They check that the provider answers only the shell user (`TokenProviderTest`),
  that the app hears changes of `adb_wifi_enabled` (`AdbWifiObserverTest`) and the
  strings in both languages (`LocalesTest`). The `emulator` workflow runs them.
  ````

- [ ] **Paso 9: lo que ve la CI de Android sigue verde** (sin dispositivo): `cd android && gradle -q :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest; echo "exit $?"` → `exit 0`. Para el emulador (`EMU; scripts/emulator.sh stop`).

- [ ] **Paso 10: commit.**

  ```bash
  git add android/app/build.gradle.kts android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/AdbWifiObserverTest.kt android/README.md
  git commit -m "test(android): instrumentados solo en un emulador; el observador oye adb_wifi_enabled

  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
  ```

---

