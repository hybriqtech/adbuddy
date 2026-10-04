# M2-8: Permitir (y Responder, Enviar captura) tras desbloquear con PIN

> Plan corto, una tarea de seguridad. Ejecuta con `superpowers:subagent-driven-development`; revisión opus del plan (hecha, v2 recoge sus correcciones C1-C3, I1-I4, M1-M5) y del diff. Rama `fix/m2-8-unlock-allow`, desde `fix/confirm-arming`.

**Objetivo:** con el móvil bloqueado, el botón de la notificación que pide desbloqueo (`setAuthenticationRequired`, API 31+) debe poder completar la acción cuando la persona teclea el PIN, sin pulsar dos veces. Hoy SystemUI entrega el `PendingIntent` unos 300 ms antes de que `KeyguardManager.isDeviceLocked` pase a false (medido en API 34, 7 de 7) y la app lo descarta en silencio. `TestAllowFromTheLockedShadeAsksToUnlock` lo fija como `authActionsMayBeDropped = true`.

**Regla fija:** nunca se acepta un Permitir, una respuesta o una captura que llegue con el móvil bloqueado salvo que la APP lea `isDeviceLocked == false` después de la entrega y dentro de la ventana. Ante la duda, se descarta como hoy. Denegar no cambia (nunca espera).

**Decisión del controlador (ronda 2):** un Permitir (no Denegar) entregado con el móvil bloqueado y NO armado en el instante de la entrega (`ConfirmNotifier.arming.isArmed(id)` leído en `onReceive`) se descarta sin esperar: cierra el residual «toque desviado al aparecer la notificación + desbloqueo casual con un SystemUI que ignore el flag». Una lectura del bloqueo que falla cuenta como bloqueado.

**Decisiones (revisión opus):** ventana 2000 ms (medido 300 ms; fallo seguro si se pasa); sondeo cada 50 ms; el armado de 600 ms NO se cuenta desde el desbloqueo (el desbloqueo ocurre 0-300 ms antes de reevaluar: anularía la función; con el bouncer de por medio el toque desviado + PIN es un paso deliberado) y `ConfirmArming.isArmed(id)` se mantiene tal cual en la reevaluación; sin atar a `ACTION_USER_PRESENT`/`isKeyguardLocked` (no discriminan un desbloqueo casual); alternativa (b) (actividad translúcida con `requestDismissKeyguard`) NO en este hito: queda en pendientes como plan B; residual aceptado y documentado en `safety.md`: un SystemUI de fabricante que ignore `setAuthenticationRequired` + el dueño desbloqueando por otra vía en <2 s.

## Código (v2, lo implementa tal cual; ajustes técnicos solo si no compila)

`android/app/src/main/kotlin/tech/hybriq/adbuddy/confirm/UnlockWait.kt`:

```kotlin
package tech.hybriq.adbuddy.confirm

import java.util.concurrent.atomic.AtomicBoolean

/**
 * An action (Allow, a direct reply, a screenshot) that a notification button delivered while the phone was locked
 * (M2-8). On Android 12+ the system holds the button until the PIN is typed, but it delivers it about 300 ms before
 * KeyguardManager.isDeviceLocked turns false. This waits for the APP to read "unlocked", for at most [WINDOW_MS]
 * by [now] (not by the number of polls: a stalled main thread that polls late gets false). Whatever else happens
 * (the window ends, an exception, Android 11 and before) the answer is false: the action is dropped, as before.
 * Android 11 has no setAuthenticationRequired: its button arrives from the lock screen with no authentication, so it
 * never waits.
 */
internal class UnlockWait(
    private val sdk: Int,
    private val isLocked: () -> Boolean,
    private val now: () -> Long,
    private val schedule: (delayMs: Long, step: () -> Unit) -> Unit,
    private val onResult: (unlocked: Boolean) -> Unit,
) {
    private val done = AtomicBoolean(false)
    private var start = 0L

    fun begin() {
        if (sdk < MIN_SDK) return finish(false)
        start = now()
        step()
    }

    private fun step() {
        if (done.get()) return
        val elapsed = now() - start
        val locked = try { isLocked() } catch (e: RuntimeException) { true }
        when {
            !locked && elapsed <= WINDOW_MS -> finish(true)
            elapsed >= WINDOW_MS -> finish(false)
            else -> try { schedule(POLL_MS, ::step) } catch (e: RuntimeException) { finish(false) }
        }
    }

    private fun finish(unlocked: Boolean) {
        if (done.compareAndSet(false, true)) onResult(unlocked)
    }

    companion object {
        const val WINDOW_MS = 2000L
        const val POLL_MS = 50L
        const val MIN_SDK = 31 // Build.VERSION_CODES.S: setAuthenticationRequired

        /**
         * What every button that needs the phone unlocked does: [onResult] gets true at once when it was not locked
         * at delivery (the caller still decides with a fresh read), after the wait when it was, false when it never
         * unlocked. Calls [onResult] exactly once.
         */
        fun afterUnlock(
            lockedAtDelivery: Boolean, sdk: Int, isLocked: () -> Boolean, now: () -> Long,
            schedule: (Long, () -> Unit) -> Unit, onResult: (Boolean) -> Unit,
        ) {
            if (!lockedAtDelivery) return onResult(true)
            UnlockWait(sdk, isLocked, now, schedule, onResult).begin()
        }
    }
}
```

`ConfirmReceiver` (la lógica en una función `internal fun handle` probable con funciones falsas; `onReceive` es un envoltorio fino):

```kotlin
override fun onReceive(context: Context, intent: Intent) {
    if (intent.action != ACTION_REPLY) return
    val id = intent.getStringExtra(EXTRA_ID) ?: return
    val decision = intent.getStringExtra(EXTRA_DECISION) ?: return
    val app = context.applicationContext as AdbuddyApp
    val km = context.getSystemService(KeyguardManager::class.java)
    val pending = goAsync() // may be null (tests call onReceive directly): finish is then a no-op
    val handler = Handler(Looper.getMainLooper())
    handle(
        decision, km.isDeviceLocked, Build.VERSION.SDK_INT, { km.isDeviceLocked }, SystemClock::elapsedRealtime,
        { d, step -> handler.postDelayed(step, d) },
        mayReplyNow = { locked -> mayReply(app.bridge.state.value, id, decision, app.clock.now(), locked, ConfirmNotifier.arming) },
        send = { app.bridge.reply(id, decision) },
        finish = { pending?.finish() },
    )
}

internal fun handle(
    decision: String, lockedAtDelivery: Boolean, sdk: Int, isLocked: () -> Boolean, now: () -> Long,
    schedule: (Long, () -> Unit) -> Unit, mayReplyNow: (locked: Boolean) -> Boolean, send: () -> Unit, finish: () -> Unit,
) {
    val finished = AtomicBoolean(false)
    val finishOnce = { if (finished.compareAndSet(false, true)) finish() }
    try {
        if (decision == Decision.DENY) { // never waits, never needs the unlock
            if (mayReplyNow(lockedAtDelivery)) send()
            finishOnce(); return
        }
        UnlockWait.afterUnlock(lockedAtDelivery, sdk, isLocked, now, schedule) { unlocked ->
            try {
                // A fresh read of the lock, with the state and the time of now: everything is decided again.
                val lockedNow = try { isLocked() } catch (e: RuntimeException) { true }
                if (unlocked && mayReplyNow(lockedNow)) send()
            } finally { finishOnce() }
        }
    } catch (e: RuntimeException) { finishOnce(); throw e }
}
```
(Un solo `finish()` garantizado por el `AtomicBoolean`: un segundo lanza `IllegalStateException`.)

`BridgeService` (Reply y Capture), con el mismo `UnlockWait.afterUnlock`, un token para quitar callbacks (`main.postDelayed(step, TOKEN, delay)`) y `main.removeCallbacksAndMessages(TOKEN)` en `onDestroy`; en el callback, `if (!foreground || destroyed) return` antes de actuar (I1: nada tras destruir el servicio):

```kotlin
ACTION_REPLY -> {
    // The text is read now, from this intent: START_STICKY would redeliver a null intent, never this one.
    val text = RemoteInput.getResultsFromIntent(intent)?.getCharSequence(ConversationNotifier.KEY_REPLY)?.toString()
    whenUnlocked { unlocked -> if (unlocked && !locked() && !text.isNullOrBlank()) app.chat.send(text); shown?.let { post(it) } }
}
ACTION_CAPTURE -> whenUnlocked { unlocked -> if (unlocked && !locked()) app.captures.take { a, t -> if (a != null) app.chat.send("", listOf(a)) else toast(t) } }

private fun whenUnlocked(act: (unlocked: Boolean) -> Unit) = UnlockWait.afterUnlock(
    locked(), Build.VERSION.SDK_INT, ::locked, SystemClock::elapsedRealtime,
    { d, step -> main.postDelayed(step, TOKEN, d) },
) { unlocked -> if (foreground && !destroyed) act(unlocked) }
```

## Tareas (cada una compila y pasa sus tests por sí sola)

1. **`UnlockWait` + tests JVM puros** (reloj, `schedule` e `isLocked` falsos, sin Robolectric ni Looper; el SDK es un parámetro: NADA de `@Config(sdk=[30])`, que no corre aquí porque Robolectric va offline con un solo `android-all` de la API 34): desbloquea a los 300 ms → true; nunca → false a los 2000 ms medidos por el reloj; ya desbloqueado en el primer sondeo → true; `sdk=30` → false sin sondear (cero llamadas a `isLocked` y a `schedule`); `isLocked` lanza → sigue y acaba false; un sondeo que llega tarde (el reloj salta a 2500 ms) con `isLocked=false` → false; `onResult` exactamente una vez aunque se llame `step` dos veces; `schedule` lanza → false; `afterUnlock` con `lockedAtDelivery=false` → true inmediato sin esperar. Mutaciones: medir por número de sondeos → falla el del reloj; aceptar el sondeo tardío → falla; sin `AtomicBoolean` → falla el de resultado único; `sdk<31` esperando → falla.
2. **`ConfirmReceiver.handle`** con funciones falsas (sin `goAsync`, que es null fuera del sistema): (a) bloqueado y `isLocked` pasa a false a los 300 ms → `send` una vez con `mayReplyNow(lockedNow=false)`; (b) bloqueado y nunca desbloquea → ni `send` ni `mayReplyNow` verdadero; (c) desbloquea pero `mayReplyNow` da false (caducó, `answering`, ya no armada tras `rearmAll`) → ni `send`; (d) Denegar bloqueado → `send` inmediato, sin `schedule`; (e) `sdk=30` bloqueado → ni envía ni espera; (f) desbloqueado al llegar → `send` síncrono; (g) `finish` exactamente una vez en todos los caminos, incluida una excepción en `send`/`mayReplyNow`/`schedule`; (h) dos `handle` del mismo `id` (doble entrega): el segundo ve la pregunta `answering` (usar `mayReply` real con el estado actualizado) y no envía. Mutaciones: no reevaluar tras el desbloqueo → (c) falla; esperar en API 30 → (e); `send` sin comprobar `unlocked` → (b); `finish` sin guarda o doble → (g).
3. **`BridgeService` Reply y Capture** (`Robolectric.buildService`, como `BridgeServiceScreenOnTest`; `ShadowKeyguardManager.setIsDeviceLocked`; el Looper principal controlado con `shadowOf(Looper.getMainLooper())`): bloqueado y desbloquea tras la entrega → un `chat.send(text)` con el texto de la intent; vacío → no envía; Capture → solo tras el desbloqueo; la notificación se republica siempre; el servicio se destruye durante la espera → no `post`, no `chat.send`, no `captures.take` (I1); API 34 únicamente (el SDK 30 se cubre en la tarea 1 con el parámetro). Mutaciones: sin `removeCallbacksAndMessages`/sin la guarda `foreground && !destroyed` → falla; actuar sin `unlocked` → falla.
4. **Emulador** (`host/internal/emutest/confirm_test.go`; lo corre el controlador, local API 34, repetido ≥5 veces, luego el job del CI): `authActionsMayBeDropped = false` y retirar las ramas «known defect» (Permitir y Enviar captura deben llegar tras el PIN sin segunda pulsación); se MANTIENEN los dos `l.take(..., 5 s)` que exigen que NADA llegue con el bouncer abierto. Casos nuevos: (i) NEGATIVO: Permitir con el móvil bloqueado, aparece el bouncer, Atrás, y más tarde se desbloquea por otra vía (PIN en la pantalla de bloqueo): no llega ningún `confirm.reply`; (ii) POSITIVO largo: Permitir, esperar más de la ventana (≥3 s) con el bouncer abierto, teclear el PIN → llega (la ventana empieza en la entrega, no en el toque); (iii) Responder: antes de escribir el test el controlador mide en el emulador si SystemUI pide el bouncer ANTES de abrir el campo de texto (I2); si es así, el test es «bouncer, PIN, escribir, enviar» y la espera se mantiene por defensa; si no existe el campo en la pantalla de bloqueo, queda como validación manual en `docs/superpowers/fragmentos/fix-m2-8-unlock-allow.md`.
5. **Docs:** fragmento `docs/superpowers/fragmentos/fix-m2-8-unlock-allow.md` (NO se edita `pendientes-m3-m5.md`): «Post-v1: Permitir tras desbloquear con PIN» pasa a CERRADA (ventana 2 s, sin espera en Android 11, residual aceptado), plan B (b) apuntado como alternativa si el campo enseña descartes, y la amenaza M5 de la revisión («una app con acceso a las notificaciones puede ejecutar los `PendingIntent` de los botones; `setAuthenticationRequired` solo lo aplica SystemUI; la acción Reply es `FLAG_MUTABLE`; con el móvil desbloqueado ya permite aprobar») como riesgo conocido del modelo, fuera de esta tarea. `docs/safety.md` (confirmaciones: tras el PIN el botón funciona sin segunda pulsación en Android 12+; en Android 11 hay que desbloquear antes y volver a pulsar; el residual) y spec M2 §4.5 (qué hace el armado con la espera).

## Lo que NO cambia
`ConfirmHost` y `Screens.kt` (ninguna actividad usa `showWhenLocked`: el diálogo, la burbuja y `Screens.kt` no aparecen sobre la pantalla de bloqueo); las notificaciones de permiso del agente (no tienen botones, `ConversationNotifier` solo ofrece Reply, Capture, Pause, Resume, Cut); Pause, Resume y Cut (no piden desbloqueo); `setAuthenticationRequired` en los botones; el armado de 600 ms y su re-armado (`rearmAll` durante la espera puede provocar el caso (c)).
