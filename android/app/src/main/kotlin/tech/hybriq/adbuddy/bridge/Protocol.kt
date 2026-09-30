package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement

/** The app channel protocol (spec §5.3), version 1. Mirrors host/internal/bridge. */
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

    val json = Json {
        ignoreUnknownKeys = true // newer hosts may add fields
        explicitNulls = false
        encodeDefaults = true
        coerceInputValues = true // Go writes nil slices and pointers as null: they take the default
    }

    fun encodeElement(type: String, payload: JsonElement, ts: String): String =
        json.encodeToString(Message.serializer(), Message(v = VERSION, type = type, ts = ts, payload = payload))

    inline fun <reified T> encode(type: String, payload: T, ts: String): String =
        encodeElement(type, json.encodeToJsonElement(payload), ts)

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

@Serializable
data class Message(
    val v: Int,
    val seq: Long = 0,
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
data class SessionState(val agent: String = "", val state: String = "idle", val activity: String = "", val paused: Boolean = false)

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
    val source: String = "",
    val argv: List<String> = emptyList(),
    val level: String = "",
    val description: String = "",
    val status: String = "",
    val reason: String = "",
    val inverse: Inverse? = null,
    val undoneBy: String? = null,
)

@Serializable
data class Inverse(val argv: List<String> = emptyList())

@Serializable
data class Snapshot(
    val state: SessionState = SessionState(),
    val pending: List<ConfirmRequest> = emptyList(),
    val journal: List<JsonElement> = emptyList(),
)

object Decision {
    const val ALLOW = "allow"
    const val DENY = "deny"
    const val ALLOW_SESSION = "allow_session"
}
