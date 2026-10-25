package tech.hybriq.adbuddy.yolo

import java.util.Locale
import kotlinx.serialization.json.JsonElement
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.YoloSet
import tech.hybriq.adbuddy.confirm.ConfirmUi

/**
 * What the YOLO mode's card, band, dialog and notification show and do (spec 2026-10-18). Pure, so it is
 * unit-tested. Times are in the host's clock (HostClock.now()), which `until` is written in.
 */
object YoloUi {
    /** UNKNOWN: the host has not said (or is too old to have the mode); never painted as off. */
    enum class Kind { UNKNOWN, OFF, TIMED, SESSION }

    data class View(val kind: Kind, val msLeft: Long = 0) {
        val active: Boolean get() = kind == Kind.TIMED || kind == Kind.SESSION
    }

    /**
     * An expiry that has passed while the mode still reads as active is 0:00, not off: only the host says it is off
     * (it tells at the expiry), and a false off would hide a mode that is on. The link being down changes nothing: the
     * mode lives on the host.
     */
    fun view(state: BridgeState, now: Long): View {
        val y = state.yolo ?: return View(Kind.UNKNOWN)
        return when {
            !y.active -> View(Kind.OFF)
            y.session -> View(Kind.SESSION)
            else -> View(Kind.TIMED, (y.until - now).coerceAtLeast(0))
        }
    }

    /** m:ss or h:mm:ss, rounded up to whole seconds (so 0:00 only means the time is up). */
    fun countdown(msLeft: Long): String {
        val s = (msLeft.coerceAtLeast(0) + 999) / 1000
        val h = s / 3600
        val m = s % 3600 / 60
        return if (h > 0) String.format(Locale.ROOT, "%d:%02d:%02d", h, m, s % 60) else String.format(Locale.ROOT, "%d:%02d", m, s % 60)
    }

    /** The three durations the host accepts. */
    enum class Choice(val minutes: Int, val session: Boolean) { QUARTER(15, false), HOUR(60, false), SESSION(0, true) }

    fun request(choice: Choice): JsonElement =
        Protocol.json.encodeToJsonElement(YoloSet.serializer(), YoloSet(minutes = choice.minutes, session = choice.session))

    fun offRequest(): JsonElement = Protocol.json.encodeToJsonElement(YoloSet.serializer(), YoloSet(off = true))

    /** The switch turns the mode on only from a known off with the PC connected. Turning it off is never limited. */
    fun canTurnOn(view: View, connected: Boolean): Boolean = view.kind == Kind.OFF && connected

    /** What the persistent notification says it is about, or null when it must not be up: it changes only when the mode or its expiry does. */
    fun notificationKey(state: BridgeState): String? = state.yolo?.takeIf { it.active }?.let { if (it.session) "session" else "until:${it.until}" }

    /**
     * Whether the button of the activation dialog may send: a duration was chosen (there is no default), the PC is
     * connected, the phone is unlocked now (read again at the tap: the dialog could have been left open) and the button
     * has been shown for ConfirmUi.ARMING_MS ([shownAt] and [now] in the phone's elapsed time), so a stray tap or a
     * burst as the dialog pops up turns nothing on.
     */
    fun mayConfirm(choice: Choice?, shownAt: Long?, now: Long, locked: Boolean, connected: Boolean): Boolean =
        choice != null && connected && !locked && ConfirmUi.armed(shownAt, now)
}
