# P5: `foreground` de la app sin copiar el árbol de ventanas

> **Para quien lo ejecute:** `superpowers:subagent-driven-development`. Este plan es solo plan: no se implementa sin el OK del líder. Spec: M5 (`docs/superpowers/specs/`, §3-§4 helpers y §11 pruebas) y la sección P5 de `2026-10-03-latencia.md`.

**Objetivo:** que la operación `foreground` de la app (la que la guarda del host pide en cada orden con pantalla) deje de copiar el árbol entero de todas las ventanas, sin cambiar **ni un bit** de lo que decide.

**Rama:** `perf/foreground-ligero`; solo toca `android/`, más una fila de docs. Commits con identidad local hybriqtech y el trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Contexto (código actual)

- `UiExecutor.foreground` (`android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/UiExecutor.kt`): con el móvil bloqueado devuelve `("", "", false)`; si no, `protected = notificationOnScreen() || (pkg.isNotEmpty() && (guarded(pkg) || guardedOnScreen(s.windows())))`.
- `guardedOnScreen(windows)` solo lee de cada ventana su `type` y su `pkg` (= `root?.pkg`): `windows.any { (type == "app" || type == "dialog") && (pkg.isEmpty() || guarded(pkg)) }`. Una ventana de app o diálogo **sin contenido** (`root == null`) cuenta como protegida.
- `AgentAccessibilityService.windows()` copia cada ventana a `UiWindow` con un `UiNode` recursivo (hasta `MAX_NODES` = 8000, profundidad `Dumper.MAX_DEPTH`): por cada nodo, `getChild(i)` (una ida y vuelta binder al proceso de la app) y ~16 lecturas de propiedades. Es lo que se quiere evitar; para `foreground` solo hace falta `type` y el paquete de la raíz.
- Medido (emulador, Ajustes, árbol pequeño): `foreground` de la app ≈ 10-16 ms de 132 ms de un `screen`. Con apps de árbol grande no está medido: de ahí la tarea 1.

## Reglas que el plan no puede romper

1. **Mismo veredicto.** Para cualquier lista de ventanas, `protected` sale igual que hoy. Una ventana `app`/`dialog` sin raíz o con raíz de paquete vacío sigue siendo protegida (falla cerrado).
2. **Una sola definición de tipo.** El mapeo `AccessibilityWindowInfo` → `"app"|"dialog"|"ime"|"split"|"system"` se extrae a una función privada usada por `windows()` y por la nueva lectura: no pueden divergir.
3. **El resto no cambia:** `dump`, `click`, `setText` y `Matcher` siguen usando `windows()` completo (necesitan los nodos). `foreground()` del servicio (paquete y actividad) no se toca.
4. **Protocolo y host sin cambios:** misma respuesta `UiForeground`. Apps viejas y nuevas valen igual.
5. **Nada de caché:** se lee en cada llamada (una caché abre un hueco de la guarda; ver «No hacer» del fragmento).

## Tarea 1: medir dentro de la app con un árbol grande (puerta de decisión)

**Archivos:** nuevo `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/ForegroundHeadsOnEmulatorTest.kt` (instrumentado) y una actividad de prueba solo de `androidTest` (o `debug`) con vistas anidadas.

La medición se hace **dentro de la app**, con `System.nanoTime()` alrededor de `UiExecutor.run(FOREGROUND)` y de `windows()` en el servicio real. No sirve `adbuddy foreground` desde el host (`latency_test.go`): suma ~120-165 ms de transporte (spec M5 §4.3) y una puerta de 30 ms no tendría sentido allí. La lista de Ajustes → Apps tampoco sirve: es un `RecyclerView` que solo expone las filas visibles (decenas de nodos).

- [ ] Actividad de prueba con ~2000 vistas anidadas y otra con >8000 (el tope `MAX_NODES`); opcionalmente un `WebView` con un DOM grande.
- [ ] El test activa el servicio de accesibilidad (`UiAutomation.executeShellCommand("settings put secure enabled_accessibility_services tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService")`, solo en emulador; guarda y restaura en `@After` el valor anterior de `enabled_accessibility_services` y `accessibility_enabled`), espera con tope de tiempo a que `AgentAccessibilityService.current != null` y **falla** (no `assumeTrue`) si no llega: un test que se salta en silencio sería vacuo.
- [ ] Mide 30 llamadas (mediana y p90) de `foreground` tal como está y de `windows()` solo, con el árbol de ~2000, el de >8000 y Ajustes. Anota las cifras en `2026-10-03-latencia.md`, diciendo que son del emulador en un PC rápido (un móvil real será más lento).
- **Puerta:** si la mediana de `foreground` con el árbol de ~2000 vistas es **< 20 ms** en el emulador, P5 se descarta por medición (el ahorro no compensa código nuevo en un camino de seguridad): se apunta en el ledger y el fragmento y se para. Si es ≥ 20 ms, se sigue.
- No toca el móvil del usuario: solo emulador (cupo con `emu-lock.sh`, puertos propios).

## Tarea 2: `Screen.windowHeads()`, `typeOf` puro y el servicio

**Archivos:** `a11y/UiNode.kt`, `a11y/UiExecutor.kt`, `a11y/AgentAccessibilityService.kt`, `src/test/.../UiExecutorTest.kt`, nuevo test JVM del `typeOf`.

Código literal (el implementador no inventa):

```kotlin
// UiNode.kt, junto a UiWindow
/** A window's type and its root's package ("" without content: it counts as protected). */
data class UiWindowHead(val type: String, val pkg: String)

// UiExecutor.kt, interfaz Screen
/** type and root package of each window, in windows() order; the service reads them without the tree. */
fun windowHeads(): List<UiWindowHead> = windows().map { UiWindowHead(it.type, it.pkg) }

// UiExecutor.kt
private fun guardedOnScreen(windows: List<UiWindowHead>) =
    windows.any { (it.type == "app" || it.type == "dialog") && (it.pkg.isEmpty() || guarded(it.pkg)) }
// y en foreground(): ... guardedOnScreen(s.windowHeads())

// AgentAccessibilityService.kt
override fun windowHeads(): List<UiWindowHead> =
    windows.map { w -> UiWindowHead(typeOf(w), w.root?.packageName?.toString().orEmpty()) }

private fun typeOf(w: AccessibilityWindowInfo): String = typeOf(w.type, w.isActive, w.isFocused)

companion object {
    internal fun typeOf(type: Int, active: Boolean, focused: Boolean): String = when (type) {
        AccessibilityWindowInfo.TYPE_APPLICATION -> if (active || focused) "app" else "dialog"
        AccessibilityWindowInfo.TYPE_INPUT_METHOD -> "ime"
        AccessibilityWindowInfo.TYPE_SPLIT_SCREEN_DIVIDER -> "split"
        else -> "system"
    }
}
```

- [ ] `windows()` pasa a usar `typeOf(w)` (el mismo `when`, extraído sin cambios): una sola definición de tipo. Si el servicio ya tiene un `companion object`, se añade ahí.
- [ ] `windows()` no recicla nodos (`ref = n` los guarda) y con minSdk 30 `recycle()` no hace nada desde API 33: `windowHeads()` tampoco recicla.
- [ ] `guardedOnScreen` solo la usa `foreground`; `click`, `setText` y `dump` no cambian.
- [ ] Tests JVM:
  - (a) Los casos que ya cubre `UiExecutorTest` con el `FakeScreen` actual (que **no** sobrescribe `windowHeads()`: ejercita el valor por defecto) siguen verdes sin tocar: `aSplitScreenWithABank`, `aWindowItCannotSee`, `nothingOnAProtectedAppNorADBuddy`, `aRecentNotificationIsAProtectedScreen`.
  - (b) Un fake aparte que sobrescribe `windowHeads()` y cuyo `windows()` lanza: `foreground` responde `ok == true` y `protected` correcto en los dos sentidos (una cabecera `app` con `pkg` vacío da `true`; `app` de Ajustes + `system` de SystemUI da `false`); así no basta con que no lance.
  - (c) Más casos: una cabecera `ime` y otra `system` sin raíz dan `false`; una ventana `app` de ADBuddy que no está al frente da `true`.
  - (d) `typeOf(Int, Boolean, Boolean)` con sus cuatro ramas (aplicación activa → `app`, inactiva y sin foco → `dialog`, teclado, divisor, otro → `system`).
  - (e) Mutación a mano: cambiar `it.pkg.isEmpty()` por `false` en `guardedOnScreen` hace fallar el caso de la ventana sin contenido; pegar la salida del fallo en el ledger.
- [ ] `:app:testDebugUnitTest` verde (reglas de RAM de gradle del proyecto) y el linter del proyecto si lo hay.
- Nota para el `Ruling:` del ledger: único cambio de comportamiento, que si `windows()` lanzaba al recorrer el árbol `foreground` daba `FAILED` y ahora da un veredicto; es igual de cerrado (la raíz que no se puede leer cuenta como paquete vacío, protegido).

## Tarea 3: equivalencia en el emulador

**Archivos:** el mismo `ForegroundHeadsOnEmulatorTest.kt`; depende de las tareas 1 y 2.

- [ ] Con el servicio real (activado como en la tarea 1, fallando si no se enlaza): para Ajustes solo, con un diálogo encima, con el teclado abierto y con el panel de notificaciones, `windowHeads()` coincide con `windows().map { type, pkg }` (mismo orden, mismos valores). Las dos lecturas no son atómicas: esperar a que la pantalla se asiente y repetir hasta 5 veces antes de dar el test por fallido.
- [ ] Repetir las 30 mediciones de la tarea 1: la mediana debe bajar al menos a la mitad; si no, se revierte y se apunta.
- [ ] Opcional, solo si se mide: `getRoot(0)` (API 33+, evita la precarga de descendientes) frente a `getRoot()`; exige comprobar la versión (minSdk 30), así que no se mete sin cifras.
- [ ] Las pruebas de guarda existentes (`host/internal/emutest`, `TestScreenAfterOnTheEmulator`) siguen verdes sin cambios.

## Tarea 4: docs

- [ ] Cifras antes/después en `2026-10-03-latencia.md`; ledger con `Ruling:`; fila en `docs/superpowers/fragmentos/perf-latencia.md` (P5 hecho o «descartado por medición»). La spec M5 §4 no cambia (el protocolo es el mismo).

## Revisión

Revisión del plan: `adbuddy-reviewer-opus`, con reservas (0 críticos, 4 importantes, 7 menores), todo aplicado arriba: puerta medida dentro de la app con árbol grande real, test instrumentado que activa el servicio y falla en vez de saltarse, código literal con `typeOf` puro probado en la JVM, tests (a)-(e) separados. Del diff: la tarea 2 es seguridad (guarda de la app) y la revisa `adbuddy-reviewer-opus`; el resto, `adbuddy-reviewer`. Límite de rondas: 5, con tope de tiempo a los revisores.

## Riesgos y qué cuesta si me equivoco

- Un `windowHeads()` que diverja de `windows()` (otro tipo o paquete) deja pasar una ventana protegida: lo cubren la extracción de `typeOf`, el test de equivalencia en emulador y la implementación por defecto en la interfaz.
- Leer `root` de cada ventana sigue siendo una ida y vuelta binder por ventana (no se evita), solo desaparece el recorrido del árbol.
- Si la tarea 1 no justifica el cambio, el coste de este plan es solo la medición.
