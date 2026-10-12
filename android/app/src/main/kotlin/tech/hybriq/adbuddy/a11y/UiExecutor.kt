package tech.hybriq.adbuddy.a11y

import java.time.Instant
import java.util.concurrent.Executor
import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import tech.hybriq.adbuddy.bridge.Empty
import tech.hybriq.adbuddy.bridge.ErrorCode
import tech.hybriq.adbuddy.bridge.ErrorReply
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.UiClickArgs
import tech.hybriq.adbuddy.bridge.UiClicked
import tech.hybriq.adbuddy.bridge.UiDumpArgs
import tech.hybriq.adbuddy.bridge.UiForeground
import tech.hybriq.adbuddy.bridge.UiHideNoticeArgs
import tech.hybriq.adbuddy.bridge.UiLimits
import tech.hybriq.adbuddy.bridge.UiNotifications
import tech.hybriq.adbuddy.bridge.UiOp
import tech.hybriq.adbuddy.bridge.UiRequest
import tech.hybriq.adbuddy.bridge.UiResponse
import tech.hybriq.adbuddy.bridge.UiSetTextArgs
import tech.hybriq.adbuddy.bridge.UiTextSet
import tech.hybriq.adbuddy.listener.NotificationRead
import tech.hybriq.adbuddy.listener.NotificationReport

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

    /** The agent is acting: show the overlay, with a mark where it touches (null: no mark). false when the notice could not be shown: the agent must not act. */
    fun acting(mark: Box?): Boolean

    /**
     * The host is about to capture the screen by adb: take the overlay off the
     * screen for ms at most and wait until it is gone from what is drawn. It
     * comes back by itself (never while paused), or with the next acting().
     * false when it could not be taken off.
     */
    fun hideNotice(ms: Long): Boolean
}

/**
 * Runs the host's ui.request (M5 spec §2-§4): one at a time, never twice the
 * same id, never once expired (in the host's clock), and never on ADBuddy, a
 * protected app or, while a 🔴 question waits, the system UI, nor while a
 * notification may be on screen (a heads-up). It answers each
 * one with send. notifications is the one op that does not need the
 * accessibility service: it reads what the notification listener has.
 */
class UiExecutor(
    private val screen: () -> Screen?,
    private val send: (UiResponse) -> Unit,
    private val now: () -> Long,
    private val isProtected: (String) -> Boolean,
    private val questionWaiting: () -> Boolean,
    private val paused: () -> Boolean,
    private val notificationOnScreen: () -> Boolean,
    private val executor: Executor,
    /** The active notifications from the listener, or why not (M5c): no access, or not confirmed by the user. */
    private val notifications: () -> NotificationRead = { NotificationRead.NoAccess },
) {
    private val seen = object : LinkedHashMap<String, Boolean>() {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, Boolean>?) = size > SEEN
    }

    /** Takes a ui.request from the host (Bridge.listen, on the reader thread): quick, the work goes to executor. */
    fun onMessage(m: Message) {
        val r = try {
            strict.decodeFromJsonElement(UiRequest.serializer(), m.payload)
        } catch (e: IllegalArgumentException) {
            // Unknown or malformed fields: the host is told (it would wait 10 s otherwise),
            // if the id can be read; without one there is nobody to answer.
            val id = rescueId(m.payload) ?: return
            synchronized(seen) {
                if (seen.put(id, true) != null) return
            }
            executor.execute { deliver(UiResponse(id, ok = false, error = ErrorReply(ErrorCode.INVALID, "The request is not valid."))) }
            return
        }
        synchronized(seen) {
            if (seen.put(r.id, true) != null) return
        }
        executor.execute { answer(r) }
    }

    /** Runs r and sends its answer; nothing escapes (an exception on the helpers thread would kill the app). */
    private fun answer(r: UiRequest) {
        val response = try {
            run(r)
        } catch (e: RuntimeException) {
            fail(r, ErrorCode.FAILED, "The app could not do it: ${e.javaClass.simpleName}.")
        }
        deliver(response)
    }

    private fun deliver(response: UiResponse) {
        try {
            send(response)
        } catch (e: RuntimeException) {
            // The channel failed: there is nobody left to tell.
        }
    }

    /** The id of a request that did not decode: a minimal parse, a non-empty string of at most MAX_ID characters, or null. */
    private fun rescueId(payload: JsonElement): String? {
        val id = (payload as? JsonObject)?.get("id") as? JsonPrimitive ?: return null
        return id.takeIf { it.isString }?.content?.takeIf { it.isNotEmpty() && it.length <= MAX_ID }
    }

    /** Runs one request and returns its answer. */
    fun run(r: UiRequest): UiResponse {
        val expires = try {
            Instant.parse(r.expiresAt).toEpochMilli()
        } catch (e: RuntimeException) {
            // Not a time (DateTimeParseException) or out of range (DateTimeException, ArithmeticException).
            return fail(r, ErrorCode.INVALID, "expiresAt is not a time.")
        }
        if (now() > expires) return fail(r, ErrorCode.EXPIRED, "The request expired before it ran.")
        if (r.op == UiOp.NOTIFICATIONS) return notificationsOp(r)
        val s = screen() ?: return fail(r, ErrorCode.UNAVAILABLE, "ADBuddy's accessibility service is off: turn it on in the phone's accessibility settings.")
        return try {
            when (r.op) {
                UiOp.DUMP -> dump(r, s, args(r, UiDumpArgs.serializer()) ?: return invalid(r))
                UiOp.CLICK -> click(r, s, args(r, UiClickArgs.serializer()) ?: return invalid(r))
                UiOp.SET_TEXT -> setText(r, s, args(r, UiSetTextArgs.serializer()) ?: return invalid(r))
                UiOp.FOREGROUND -> foreground(r, s)
                UiOp.NOTICE -> if (args(r, Empty.serializer()) == null) invalid(r) else notice(r, s)
                UiOp.HIDE_NOTICE -> hideNotice(r, s, args(r, UiHideNoticeArgs.serializer()) ?: return invalid(r))
                else -> fail(r, ErrorCode.UNSUPPORTED, "This version of the ADBuddy app does not handle ${r.op.take(64)}.")
            }
        } catch (e: RuntimeException) {
            fail(r, ErrorCode.FAILED, "The app could not do it: ${e.javaClass.simpleName}.")
        }
    }

    /** The host is about to read the screen by adb (screencap, uiautomator): show the notice first. It reads nothing, so no heads-up check; the pause does count. */
    private fun notice(r: UiRequest, s: Screen): UiResponse = when {
        paused() -> pausedNow(r) // acting() is true without drawing while paused: the host must not read then
        s.acting(null) -> ok(r, Empty, Empty.serializer())
        else -> noNoticeToRead(r)
    }

    /** The host is about to capture the screen by adb: take the notice off the screen for the capture. It reads nothing, so no heads-up check; the pause does count. */
    private fun hideNotice(r: UiRequest, s: Screen, a: UiHideNoticeArgs): UiResponse = when {
        a.ms < 1 || a.ms > UiLimits.MAX_HIDE_NOTICE_MS -> invalid(r)
        paused() -> pausedNow(r)
        // The host, not the app, decides the pause for a capture: the app learns of a pause only from session.state, and the host re-checks it after hideNotice.
        s.hideNotice(a.ms.toLong()) -> ok(r, Empty, Empty.serializer())
        else -> fail(r, ErrorCode.FAILED, "ADBuddy could not take its notice off the screen for the capture.")
    }

    /**
     * The notifications of the phone, without the content of ADBuddy and the
     * protected apps (NotificationReport). No access is `unavailable`, with how
     * to grant it; there is no other source (the host has no fallback).
     */
    private fun notificationsOp(r: UiRequest): UiResponse = try {
        if (args(r, Empty.serializer()) == null) {
            invalid(r)
        } else {
            when (val read = notifications()) {
                is NotificationRead.Ok -> ok(r, NotificationReport.build(read.items, ::guarded), UiNotifications.serializer())
                NotificationRead.NoAccess -> fail(r, ErrorCode.UNAVAILABLE, NO_NOTIFICATION_ACCESS)
                NotificationRead.NotConfirmed -> fail(r, ErrorCode.UNAVAILABLE, NOTIFICATIONS_NOT_CONFIRMED)
            }
        }
    } catch (e: RuntimeException) {
        fail(r, ErrorCode.FAILED, "The app could not do it: ${e.javaClass.simpleName}.")
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

    private fun noNotice(r: UiRequest) = fail(r, ErrorCode.FAILED, "ADBuddy can't show the on-screen notice, so it won't act. Ask the user to do this part.")

    private fun noNoticeToRead(r: UiRequest) = fail(r, ErrorCode.FAILED, "ADBuddy can't show the on-screen notice, so it won't read the screen. Ask the user to do this part.")

    private fun pausedNow(r: UiRequest) = fail(r, ErrorCode.PROTECTED, "The agent is paused from the phone.")

    private fun dump(r: UiRequest, s: Screen, a: UiDumpArgs): UiResponse {
        if (notificationOnScreen()) return notificationBlocked(r)
        val locked = s.locked()
        // Locked: there is no active app window, so the "last app seen" would be stale.
        val (pkg, activity) = if (locked) "" to "" else s.foreground()
        if (!locked && guarded(pkg)) return protectedApp(r, pkg)
        if (paused()) return pausedNow(r)
        if (!s.acting(null)) return noNoticeToRead(r)
        val d = Dumper.dump(s.windows(), s.screen(), s.rotation(), pkg.takeUnless { it == Packages.ADBUDDY }.orEmpty(), activity, locked, a.all == true, ::guarded)
        // A heads-up may have appeared while the tree was read: drop what was read.
        if (notificationOnScreen()) return notificationBlocked(r)
        return ok(r, d, tech.hybriq.adbuddy.bridge.UiDump.serializer())
    }

    private fun click(r: UiRequest, s: Screen, a: UiClickArgs): UiResponse {
        val n = listOf(a.id != null, a.text != null, a.x != null || a.y != null).count { it }
        if (n != 1 || (a.x == null) != (a.y == null) || (a.x ?: 0) < 0 || (a.y ?: 0) < 0) return invalid(r)
        if (paused()) return pausedNow(r)
        if (notificationOnScreen()) return notificationBlocked(r)
        if (s.locked()) return fail(r, ErrorCode.PROTECTED, "The phone is locked: ask the user to unlock it.")
        val fg = s.foreground().first
        if (guarded(fg)) return protectedApp(r, fg)
        val windows = s.windows()
        if (a.x != null && a.y != null) {
            pointBlocked(r, windows, a.x, a.y)?.let { return it }
            lateBlock(r)?.let { return it }
            if (!s.acting(Box(a.x, a.y, a.x + 1, a.y + 1))) return noNotice(r)
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
                    lateBlock(r)?.let { return it }
                    if (!s.acting(m.bounds)) return noNotice(r)
                    if (s.click(m.target)) {
                        ok(r, UiClicked(true, m.bounds.list(), "action"), UiClicked.serializer())
                    } else {
                        // Not every clickable view takes ACTION_CLICK: tap its middle, but
                        // only after checking again what is on top of that point now.
                        pointBlocked(r, s.windows(), m.bounds.centerX, m.bounds.centerY)?.let { return it }
                        lateBlock(r)?.let { return it }
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
        if (notificationOnScreen()) return notificationBlocked(r)
        if (s.locked()) return fail(r, ErrorCode.PROTECTED, "The phone is locked: ask the user to unlock it.")
        val fg = s.foreground().first
        if (guarded(fg)) return protectedApp(r, fg)
        val field = s.focusedInput() ?: return fail(r, ErrorCode.NOT_FOUND, "No text field has the focus: tap one first.")
        if (guarded(field.pkg)) return protectedApp(r, field.pkg)
        if (field.password) return fail(r, ErrorCode.PROTECTED, "ADBuddy does not type into password fields: ask the user to type it.")
        lateBlock(r)?.let { return it }
        if (!s.acting(field.bounds)) return noNotice(r)
        if (!s.setText(field, a.text)) return fail(r, ErrorCode.FAILED, "The field did not take the text.")
        return ok(r, UiTextSet(true), UiTextSet.serializer())
    }

    private fun foreground(r: UiRequest, s: Screen): UiResponse {
        // Locked: unknown (empty), so the host falls back to dumpsys instead of trusting a stale app.
        if (s.locked()) return ok(r, UiForeground("", "", false), UiForeground.serializer())
        val (pkg, activity) = s.foreground()
        val protected = notificationOnScreen() || (pkg.isNotEmpty() && (guarded(pkg) || guardedOnScreen(s.windows())))
        return ok(r, UiForeground(pkg, activity, protected), UiForeground.serializer())
    }

    /** Checked again just before acting: the agent was paused, or a notification appeared, since the first check. */
    private fun lateBlock(r: UiRequest): UiResponse? = when {
        paused() -> pausedNow(r)
        notificationOnScreen() -> notificationBlocked(r)
        else -> null
    }

    private fun notificationBlocked(r: UiRequest) = fail(r, ErrorCode.PROTECTED, "A notification is on screen: ask the user to do this part.")

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

        /** What the agent is told when the listener is not there: how the user grants it. */
        const val NO_NOTIFICATION_ACCESS =
            "ADBuddy cannot read the notifications: the user has not allowed notification access to ADBuddy. " +
                "Ask them to allow it in the ADBuddy app (Settings, Notification access) or in the phone's Settings, " +
                "Notifications, Device & app notifications. If it is already allowed, turning it off and on again fixes it."

        /** Allowed in the system but not confirmed by the user in ADBuddy: only they can, the agent cannot press it. */
        const val NOTIFICATIONS_NOT_CONFIRMED =
            "ADBuddy cannot read the notifications yet: the user has to confirm it in the ADBuddy app " +
                "(Settings, Notification access). Ask them to; it cannot be done for them."

        /** The longest id that is answered when the request does not decode. */
        const val MAX_ID = 128

        private val strict = Json(Protocol.json) { ignoreUnknownKeys = false }

        /** The JSON element a ui.response carries. */
        fun encode(r: UiResponse): JsonElement = Protocol.json.encodeToJsonElement(UiResponse.serializer(), r)
    }
}
