package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable

// The operations of ui.request and the shape of their args and result (M5
// spec §3). Mirrors host/internal/bridge/ui_ops.go; UiOpsFixturesTest checks
// the operations, the limits and validPackage against
// protocol/fixtures/ui.json, which the host's tests read too.

/** The operations of UiRequest.op. */
object UiOp {
    const val DUMP = "dump"
    const val CLICK = "click"
    const val SET_TEXT = "setText"
    const val FOREGROUND = "foreground"
    const val NOTIFICATIONS = "notifications"
    const val NOTICE = "notice"

    /** Each operation and the serializers of its args and result. */
    val ALL: Map<String, Pair<KSerializer<*>, KSerializer<*>>> = mapOf(
        DUMP to (UiDumpArgs.serializer() to UiDump.serializer()),
        CLICK to (UiClickArgs.serializer() to UiClicked.serializer()),
        SET_TEXT to (UiSetTextArgs.serializer() to UiTextSet.serializer()),
        FOREGROUND to (Empty.serializer() to UiForeground.serializer()),
        NOTIFICATIONS to (Empty.serializer() to UiNotifications.serializer()),
        NOTICE to (Empty.serializer() to Empty.serializer()),
    )
}

/** Limits of the helpers (M5 spec §3, §4.3). Text is counted in code points, the same as Go. */
object UiLimits {
    /** Bytes of a ui.response's result, as encoded JSON (UTF-8). */
    const val MAX_UI_RESULT = 512 * 1024
    const val MAX_NOTIFICATIONS = 50
    const val MAX_NOTIFICATION_TEXT = 1024
    const val MAX_SET_TEXT = 4096
    const val MAX_PROTECTED_APPS = 1000
    const val MAX_PACKAGE = 255

    private val packageRe = Regex("[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z][A-Za-z0-9_]*)+")

    /** An Android package name: at least two dot-separated parts of letters, digits and _, each starting with a letter. */
    fun validPackage(s: String): Boolean = s.length <= MAX_PACKAGE && packageRe.matches(s)
}

/** dump's args. all: every visible node, not only the informative or usable ones. */
@Serializable
data class UiDumpArgs(val all: Boolean? = null)

/** click's args: exactly one of id, text, or x and y. */
@Serializable
data class UiClickArgs(val id: String? = null, val text: String? = null, val x: Int? = null, val y: Int? = null)

/** setText's args: the text that replaces the focused field's, at most MAX_SET_TEXT code points. */
@Serializable
data class UiSetTextArgs(val text: String)

/** dump's result: the screen as the agent may see it. elements is empty while the phone is locked. */
@Serializable
data class UiDump(
    val locked: Boolean,
    val `package`: String,
    val activity: String,
    val width: Int,
    val height: Int,
    val rotation: Int,
    val truncated: Boolean,
    val elements: List<UiElement> = emptyList(),
)

/**
 * One element of a dump. bounds: left, top, right, bottom in screen pixels,
 * clipped to what is on screen. text and hint are empty for a password field;
 * checked is null (absent) when it cannot be checked.
 */
@Serializable
data class UiElement(
    val id: String,
    val text: String,
    val desc: String,
    val hint: String,
    val `class`: String,
    val bounds: List<Int>,
    val click: Boolean,
    val edit: Boolean,
    val scroll: Boolean,
    val focused: Boolean,
    val checked: Boolean? = null,
    val enabled: Boolean,
    val password: Boolean,
    /** app, dialog, ime, system or split. */
    val window: String,
) {
    init {
        require(bounds.size == 4) { "bounds needs 4 numbers" }
    }
}

/**
 * click's result. how: "action" (ACTION_CLICK on a node, bounds are the
 * node's) or "gesture" (a tap at a point, bounds are [x, y, x+1, y+1]).
 * clicked is always true: a click that did not happen is an error.
 */
@Serializable
data class UiClicked(val clicked: Boolean, val bounds: List<Int>, val how: String) {
    init {
        require(bounds.size == 4) { "bounds needs 4 numbers" }
    }
}

/** setText's result. set is always true, as UiClicked.clicked. */
@Serializable
data class UiTextSet(val set: Boolean)

/** foreground's result. An empty package means unknown. protected: a protected app or ADBuddy itself. */
@Serializable
data class UiForeground(val `package`: String, val activity: String, val protected: Boolean)

/** notifications' result: at most MAX_NOTIFICATIONS, the newest first; truncated when there were more. */
@Serializable
data class UiNotifications(val notifications: List<UiNotification> = emptyList(), val truncated: Boolean)

/** One notification. title and text are empty for a protected app and for ADBuddy, and cut to MAX_NOTIFICATION_TEXT code points. */
@Serializable
data class UiNotification(
    val key: String,
    val `package`: String,
    val title: String,
    val text: String,
    /** RFC 3339, UTC. */
    val postedAt: String,
    val ongoing: Boolean,
)
