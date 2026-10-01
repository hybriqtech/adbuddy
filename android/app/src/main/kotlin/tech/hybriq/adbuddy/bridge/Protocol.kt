package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement

/**
 * The app channel protocol, version 1. Mirrors host/internal/bridge; the
 * contract is docs/superpowers/specs/2026-10-01-adbuddy-protocolo-m3-m5-design.md
 * and the fixtures in protocol/fixtures/ (FixturesTest).
 */
object Protocol {
    const val VERSION = 1

    /** The oldest host protocol this app talks to. */
    const val MIN_HOST_PROTOCOL = 1

    /** Close code the host uses for an app that is too old (bridge.CloseIncompatible). */
    const val CLOSE_INCOMPATIBLE = 4001

    const val HELLO = "hello"
    const val WELCOME = "welcome"
    const val SNAPSHOT = "snapshot"
    const val SESSION_STATE = "session.state"
    const val CONFIRM_REQUEST = "confirm.request"
    const val CONFIRM_REPLY = "confirm.reply"
    const val JOURNAL_ENTRY = "journal.entry"
    const val NOTICE = "notice"
    const val PAUSE = "pause"
    const val RESUME = "resume"

    // Common (§4)
    const val ERROR = "error"

    // Session and chat (§5): M3, M4
    const val SESSION_NEW = "session.new"
    const val SESSION_STOP = "session.stop"
    const val AGENT_INTERRUPT = "agent.interrupt"
    const val CHAT_SEND = "chat.send"
    const val CHAT_MESSAGE = "chat.message"
    const val TOOL_ACTIVITY = "tool.activity"
    const val PERMISSION_REQUEST = "permission.request"
    const val PERMISSION_REPLY = "permission.reply"
    const val PERMISSION_RESOLVED = "permission.resolved"

    // Attachments and files (§6): M3
    const val ATTACH_BEGIN = "attach.begin"
    const val ATTACH_READY = "attach.ready"
    const val ATTACH_END = "attach.end"
    const val ATTACH_DONE = "attach.done"
    const val FILE_GET = "file.get"
    const val FILE_END = "file.end"
    const val FILE_CANCEL = "file.cancel"
    const val CAPTURE_REQUEST = "capture.request"
    const val CAPTURE_READY = "capture.ready"

    // Terminal (§7): M3
    const val TERMINAL_WATCH = "terminal.watch"
    const val TERMINAL_UNWATCH = "terminal.unwatch"
    const val TERMINAL_SNAPSHOT = "terminal.snapshot"
    const val TERMINAL_OUTPUT = "terminal.output"
    const val TERMINAL_SIZE = "terminal.size"
    const val TERMINAL_INPUT = "terminal.input"
    const val TERMINAL_RESIZE = "terminal.resize"

    // Helpers and remote mode (§8): M5
    const val UI_REQUEST = "ui.request"
    const val UI_RESPONSE = "ui.response"
    const val DEVICE_INFO = "device.info"
    const val REMOTE_SET = "remote.set"
    const val REMOTE_STATE = "remote.state"

    /** The longest request id (spec §3). */
    const val MAX_RID = 64

    /** A request id: 1 to MAX_RID ASCII letters, digits, "_" or "-" (spec §3). */
    fun validRid(rid: String): Boolean =
        rid.length in 1..MAX_RID && rid.all { it in 'a'..'z' || it in 'A'..'Z' || it in '0'..'9' || it == '_' || it == '-' }

    val json = Json {
        ignoreUnknownKeys = true // newer hosts may add fields
        explicitNulls = false
        encodeDefaults = true
        coerceInputValues = true // Go writes nil slices and pointers as null: they take the default
    }

    fun encodeElement(type: String, payload: JsonElement, ts: String, rid: String? = null): String =
        json.encodeToString(Message.serializer(), Message(v = VERSION, type = type, rid = rid, ts = ts, payload = payload))

    inline fun <reified T> encode(type: String, payload: T, ts: String, rid: String? = null): String =
        encodeElement(type, json.encodeToJsonElement(payload), ts, rid)

    fun decode(text: String): Message? = try {
        json.decodeFromString(Message.serializer(), text).takeIf { it.v == VERSION && it.type.isNotEmpty() }
    } catch (e: IllegalArgumentException) { // SerializationException is one
        null
    } catch (e: StackOverflowError) { // absurdly deep nesting
        null
    }

    inline fun <reified T> payload(m: Message): T? = try {
        json.decodeFromJsonElement<T>(m.payload)
    } catch (e: IllegalArgumentException) {
        null
    } catch (e: StackOverflowError) {
        null
    }
}

/** The envelope. seq > 0: an event; 0: a reply or a stream message (spec §3). rid: the request it answers. */
@Serializable
data class Message(
    val v: Int,
    val seq: Long = 0,
    val rid: String? = null,
    val type: String,
    val ts: String = "",
    val payload: JsonElement = JsonObject(emptyMap()),
)

@Serializable
data class Hello(
    val token: String,
    val lastSeq: Long,
    val session: String = "", // the session of the last welcome: another one means the host restarted
    val appVersion: String,
    val deviceSerial: String = "",
    val protocol: Int = Protocol.VERSION,
) {
    // Keeps the token out of logs and crash reports.
    override fun toString() =
        "Hello(token=<redacted>, lastSeq=$lastSeq, session=$session, appVersion=$appVersion, deviceSerial=$deviceSerial, protocol=$protocol)"
}

@Serializable
data class Welcome(val session: String, val fromSeq: Long, val hostVersion: String = "", val protocol: Int = 0)

@Serializable
data class SessionState(
    val agent: String = "",
    val state: String = "idle",
    val activity: String = "",
    val paused: Boolean = false,
    /** Changes with each session.new and each relaunch (M3). */
    val conversation: String = "",
    /** chat (an adapter) or terminal (M3). */
    val view: String = "",
    /** A PC terminal is attached: its size rules the PTY (M3). */
    val pcAttached: Boolean = false,
)

/** A 🔴 command waiting for an answer. Title and detail come from the host's rules, never from the agent. */
@Serializable
data class ConfirmRequest(
    val id: String,
    val level: String = "red",
    val title: String,
    val detail: String = "",
    val rawCommand: String,
    val expiresAt: String,
    val special: Boolean = false,
    val allowSession: Boolean = false,
)

@Serializable
data class ConfirmReply(val id: String, val decision: String)

@Serializable
data class Notice(val level: String = "info", val text: String)

/** The fields of a journal entry the app shows (spec §4.4). */
@Serializable
data class JournalEntry(
    val id: String,
    val ts: String = "",
    val session: String = "",
    val serial: String = "",
    val source: String = "",
    val argv: List<String> = emptyList(),
    val level: String = "",
    val rule: String = "",
    val description: String = "",
    val status: String = "",
    val exitCode: Int? = null,
    val inverse: Inverse? = null,
    val backups: List<String> = emptyList(),
    val undoneBy: String? = null,
    val undoes: String = "",
    val reason: String = "",
    val mac: String = "",
)

@Serializable
data class Inverse(val argv: List<String> = emptyList())

/**
 * Replaces what the app missed (spec §4.3). The terminal is not in it: it has
 * its own stream. The parts of later milestones stay JSON, like the journal:
 * each reducer reads its own, and a bad one does not make the whole snapshot
 * unreadable (which would drop the 🔴 questions of pending).
 */
@Serializable
data class Snapshot(
    val state: SessionState = SessionState(),
    val pending: List<ConfirmRequest> = emptyList(),
    val journal: List<JsonElement> = emptyList(),
    /** The last 100 ChatMessage, oldest first (M3). */
    val chat: List<JsonElement> = emptyList(),
    /** The PermissionRequest waiting for an answer (M4). */
    val permissions: List<JsonElement> = emptyList(),
    /** RemoteState (M5). */
    val remote: JsonElement? = null,
)

/** Answers a request (by its rid) that failed (spec §4.1). text is for the person, written by the host. */
@Serializable
data class ErrorReply(val code: String, val text: String = "")

/** The codes of ErrorReply (spec §4.1 and §8.1). */
object ErrorCode {
    const val UNSUPPORTED = "unsupported"
    const val INVALID = "invalid"
    const val NOT_FOUND = "not_found"
    const val NO_AGENT = "no_agent"
    const val BUSY = "busy"
    const val TOO_LARGE = "too_large"
    const val CHECKSUM = "checksum"
    const val FAILED = "failed"
    const val UNAVAILABLE = "unavailable"
    const val PROTECTED = "protected"
    const val EXPIRED = "expired"
}

/** The payload of the messages that carry nothing: {}. */
@Serializable
object Empty

object Decision {
    const val ALLOW = "allow"
    const val DENY = "deny"
    const val ALLOW_SESSION = "allow_session"
}
