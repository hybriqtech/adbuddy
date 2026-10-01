package tech.hybriq.adbuddy.a11y

import java.time.Instant
import java.time.format.DateTimeParseException
import java.util.concurrent.Executor
import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import tech.hybriq.adbuddy.bridge.ErrorCode
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.UiClickArgs
import tech.hybriq.adbuddy.bridge.UiClicked
import tech.hybriq.adbuddy.bridge.UiDumpArgs
import tech.hybriq.adbuddy.bridge.UiForeground
import tech.hybriq.adbuddy.bridge.UiLimits
import tech.hybriq.adbuddy.bridge.UiOp
import tech.hybriq.adbuddy.bridge.UiRequest
import tech.hybriq.adbuddy.bridge.UiResponse
import tech.hybriq.adbuddy.bridge.UiSetTextArgs
import tech.hybriq.adbuddy.bridge.UiTextSet

/** What the helpers need of the accessibility service (AgentAccessibilityService); a fake in tests. */
interface Screen {
    fun windows(): List<UiWindow>
    fun screen(): Box
    fun rotation(): Int
    fun locked(): Boolean

    /** The app in the foreground and its activity ("" when unknown). */
    fun foreground(): Pair<String, String>

    /** ACTION_CLICK on node; false when it did not happen. */
    fun click(node: UiNode): Boolean

    /** A tap at a point; false when the gesture was cancelled. */
    fun tap(x: Int, y: Int): Boolean

    /** The node with the input focus, or null. */
    fun focusedInput(): UiNode?

    /** ACTION_SET_TEXT on node; false when it did not happen. */
    fun setText(node: UiNode, text: String): Boolean

    /** The agent is acting: show the overlay, with a mark where it touches (null: no mark). */
    fun acting(mark: Box?)
}

/**
 * Runs the host's ui.request (M5 spec §2-§4): one at a time, never twice the
 * same id, never once expired (in the host's clock), and never on ADBuddy, a
 * protected app or, while a 🔴 question waits, the system UI. It answers each
 * one with send.
 */
class UiExecutor(
    private val screen: () -> Screen?,
    private val send: (UiResponse) -> Unit,
    private val now: () -> Long,
    private val isProtected: (String) -> Boolean,
    private val questionWaiting: () -> Boolean,
    private val paused: () -> Boolean,
    private val executor: Executor,
) {
    private val seen = object : LinkedHashMap<String, Boolean>() {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, Boolean>?) = size > SEEN
    }

    /** Takes a ui.request from the host (Bridge.listen, on the reader thread): quick, the work goes to executor. */
    fun onMessage(m: Message) {
        val r = try {
            strict.decodeFromJsonElement(UiRequest.serializer(), m.payload)
        } catch (e: IllegalArgumentException) {
            return // without an id there is nobody to answer
        }
        synchronized(seen) {
            if (seen.put(r.id, true) != null) return
        }
        executor.execute { send(run(r)) }
    }

    /** Runs one request and returns its answer. */
    fun run(r: UiRequest): UiResponse {
        val expires = try {
            Instant.parse(r.expiresAt).toEpochMilli()
        } catch (e: DateTimeParseException) {
            return fail(r, ErrorCode.INVALID, "expiresAt is not a time.")
        }
        if (now() > expires) return fail(r, ErrorCode.EXPIRED, "The request expired before it ran.")
        val s = screen() ?: return fail(r, ErrorCode.UNAVAILABLE, "ADBuddy's accessibility service is off: turn it on in the phone's accessibility settings.")
        return try {
            when (r.op) {
                UiOp.DUMP -> dump(r, s, args(r, UiDumpArgs.serializer()) ?: return invalid(r))
                UiOp.CLICK -> click(r, s, args(r, UiClickArgs.serializer()) ?: return invalid(r))
                UiOp.SET_TEXT -> setText(r, s, args(r, UiSetTextArgs.serializer()) ?: return invalid(r))
                UiOp.FOREGROUND -> foreground(r, s)
                UiOp.NOTIFICATIONS -> fail(r, ErrorCode.UNAVAILABLE, "Reading notifications is not available yet.")
                else -> fail(r, ErrorCode.UNSUPPORTED, "This version of the ADBuddy app does not handle ${r.op.take(64)}.")
            }
        } catch (e: RuntimeException) {
            fail(r, ErrorCode.FAILED, "The app could not do it: ${e.javaClass.simpleName}.")
        }
    }

    private fun <T> args(r: UiRequest, serializer: KSerializer<T>): T? = try {
        strict.decodeFromJsonElement(serializer, r.args)
    } catch (e: IllegalArgumentException) {
        null
    }

    private fun invalid(r: UiRequest) = fail(r, ErrorCode.INVALID, "The args of ${r.op} are not valid.")

    private fun guarded(pkg: String) = pkg == Packages.ADBUDDY || isProtected(pkg)

    /**
     * Whether any app window on screen is guarded, or hides what it is (a
     * window whose content the service cannot see): with a split screen the
     * other half counts too.
     */
    private fun guardedOnScreen(windows: List<UiWindow>) =
        windows.any { (it.type == "app" || it.type == "dialog") && (it.pkg.isEmpty() || guarded(it.pkg)) }

    private fun pausedNow(r: UiRequest) = fail(r, ErrorCode.PROTECTED, "The agent is paused from the phone.")

    private fun dump(r: UiRequest, s: Screen, a: UiDumpArgs): UiResponse {
        val locked = s.locked()
        // Locked: there is no active app window, so the "last app seen" would be stale.
        val (pkg, activity) = if (locked) "" to "" else s.foreground()
        if (!locked && guarded(pkg)) return protectedApp(r, pkg)
        s.acting(null)
        val d = Dumper.dump(s.windows(), s.screen(), s.rotation(), pkg.takeUnless { it == Packages.ADBUDDY }.orEmpty(), activity, locked, a.all == true, ::guarded)
        return ok(r, d, tech.hybriq.adbuddy.bridge.UiDump.serializer())
    }

    private fun click(r: UiRequest, s: Screen, a: UiClickArgs): UiResponse {
        val n = listOf(a.id != null, a.text != null, a.x != null || a.y != null).count { it }
        if (n != 1 || (a.x == null) != (a.y == null) || (a.x ?: 0) < 0 || (a.y ?: 0) < 0) return invalid(r)
        if (paused()) return pausedNow(r)
        if (s.locked()) return fail(r, ErrorCode.PROTECTED, "The phone is locked: ask the user to unlock it.")
        val fg = s.foreground().first
        if (guarded(fg)) return protectedApp(r, fg)
        val windows = s.windows()
        if (a.x != null && a.y != null) {
            pointBlocked(r, windows, a.x, a.y)?.let { return it }
            if (paused()) return pausedNow(r)
            s.acting(Box(a.x, a.y, a.x + 1, a.y + 1))
            if (!s.tap(a.x, a.y)) return fail(r, ErrorCode.FAILED, "The tap did not happen.")
            return ok(r, UiClicked(true, listOf(a.x, a.y, a.x + 1, a.y + 1), "gesture"), UiClicked.serializer())
        }
        return when (val m = Matcher.find(windows, s.screen(), a.id, a.text, ::guarded)) {
            Matcher.Match.NotFound -> fail(r, ErrorCode.NOT_FOUND, "No element on the screen matches.")
            Matcher.Match.Protected -> fail(r, ErrorCode.PROTECTED, "That element belongs to ADBuddy or a protected app.")
            is Matcher.Match.Ambiguous -> fail(
                r, ErrorCode.AMBIGUOUS,
                "${m.n} elements match: " + m.bounds.joinToString(", ") { "(${it.l},${it.t})-(${it.r},${it.b})" } + ".",
            )
            is Matcher.Match.Found -> when {
                guarded(m.target.pkg) -> protectedApp(r, m.target.pkg)
                m.target.pkg == Packages.SYSTEM_UI && questionWaiting() -> fail(r, ErrorCode.PROTECTED, "A question of ADBuddy is waiting on the phone: the system UI cannot be touched now.")
                else -> {
                    if (paused()) return pausedNow(r)
                    s.acting(m.bounds)
                    if (s.click(m.target)) {
                        ok(r, UiClicked(true, m.bounds.list(), "action"), UiClicked.serializer())
                    } else {
                        // Not every clickable view takes ACTION_CLICK: tap its middle, but
                        // only after checking again what is on top of that point now.
                        pointBlocked(r, s.windows(), m.bounds.centerX, m.bounds.centerY)?.let { return it }
                        if (paused()) return pausedNow(r)
                        if (!s.tap(m.bounds.centerX, m.bounds.centerY)) return fail(r, ErrorCode.FAILED, "The tap did not happen.")
                        ok(r, UiClicked(true, m.bounds.list(), "gesture"), UiClicked.serializer())
                    }
                }
            }
        }
    }

    /** A refusal when the window on top at (x, y) cannot be touched, or null. */
    private fun pointBlocked(r: UiRequest, windows: List<UiWindow>, x: Int, y: Int): UiResponse? {
        val pkg = Matcher.windowAt(windows, x, y)?.pkg.orEmpty()
        return when {
            pkg.isEmpty() -> fail(r, ErrorCode.PROTECTED, "ADBuddy cannot see what is at that point: it does not tap there.")
            guarded(pkg) -> protectedApp(r, pkg)
            pkg == Packages.SYSTEM_UI && questionWaiting() -> fail(r, ErrorCode.PROTECTED, "A question of ADBuddy is waiting on the phone: the system UI cannot be touched now.")
            else -> null
        }
    }

    private fun setText(r: UiRequest, s: Screen, a: UiSetTextArgs): UiResponse {
        val n = a.text.codePointCount(0, a.text.length)
        if (n == 0 || n > UiLimits.MAX_SET_TEXT) return invalid(r)
        if (paused()) return pausedNow(r)
        if (s.locked()) return fail(r, ErrorCode.PROTECTED, "The phone is locked: ask the user to unlock it.")
        val fg = s.foreground().first
        if (guarded(fg)) return protectedApp(r, fg)
        val field = s.focusedInput() ?: return fail(r, ErrorCode.NOT_FOUND, "No text field has the focus: tap one first.")
        if (guarded(field.pkg)) return protectedApp(r, field.pkg)
        if (field.password) return fail(r, ErrorCode.PROTECTED, "ADBuddy does not type into password fields: ask the user to type it.")
        if (paused()) return pausedNow(r)
        s.acting(field.bounds)
        if (!s.setText(field, a.text)) return fail(r, ErrorCode.FAILED, "The field did not take the text.")
        return ok(r, UiTextSet(true), UiTextSet.serializer())
    }

    private fun foreground(r: UiRequest, s: Screen): UiResponse {
        // Locked: unknown (empty), so the host falls back to dumpsys instead of trusting a stale app.
        if (s.locked()) return ok(r, UiForeground("", "", false), UiForeground.serializer())
        val (pkg, activity) = s.foreground()
        val protected = pkg.isNotEmpty() && (guarded(pkg) || guardedOnScreen(s.windows()))
        return ok(r, UiForeground(pkg, activity, protected), UiForeground.serializer())
    }

    private fun protectedApp(r: UiRequest, pkg: String) = fail(
        r, ErrorCode.PROTECTED,
        if (pkg == Packages.ADBUDDY) "ADBuddy does not act on itself." else "$pkg is a protected app: ask the user to do this part.",
    )

    private fun <T> ok(r: UiRequest, result: T, serializer: KSerializer<T>): UiResponse =
        UiResponse(r.id, ok = true, result = Protocol.json.encodeToJsonElement(serializer, result))

    private fun fail(r: UiRequest, code: String, text: String) = UiResponse(r.id, ok = false, error = ErrorReply(code, text))

    companion object {
        /** The ids remembered so none runs twice. */
        const val SEEN = 64

        private val strict = Json(Protocol.json) { ignoreUnknownKeys = false }

        /** The JSON element a ui.response carries. */
        fun encode(r: UiResponse): JsonElement = Protocol.json.encodeToJsonElement(UiResponse.serializer(), r)
    }
}
