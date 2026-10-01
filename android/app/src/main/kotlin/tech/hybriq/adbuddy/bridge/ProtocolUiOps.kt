package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable

// The operations of ui.request and the shape of their args and result (M5
// spec §3). Mirrors host/internal/bridge/ui_ops.go; UiOpsFixturesTest checks
// each one against protocol/fixtures.

/** The operations of UiRequest.op. */
object UiOp {
    const val DUMP = "dump"
    const val CLICK = "click"
    const val SET_TEXT = "setText"
    const val FOREGROUND = "foreground"
    const val NOTIFICATIONS = "notifications"
}

/** dump's args. all: every visible node, not only the informative or usable ones. */
@Serializable
data class UiDumpArgs(val all: Boolean? = null)

/** click's args: exactly one of id, text, or x and y. */
@Serializable
data class UiClickArgs(val id: String? = null, val text: String? = null, val x: Int? = null, val y: Int? = null)

/** setText's args: the text that replaces the focused field's. */
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
    val elements: List<UiElement>,
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
)

/** click's result. how: "action" (ACTION_CLICK on a node) or "gesture" (a tap at a point). */
@Serializable
data class UiClicked(val clicked: Boolean, val bounds: List<Int>, val how: String)

/** setText's result. */
@Serializable
data class UiTextSet(val set: Boolean)

/** foreground's result. protected: a protected app or ADBuddy itself. */
@Serializable
data class UiForeground(val `package`: String, val activity: String, val protected: Boolean)

/** notifications' result. */
@Serializable
data class UiNotifications(val notifications: List<UiNotification>)

/** One notification. title and text are empty for a protected app and for ADBuddy. */
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
