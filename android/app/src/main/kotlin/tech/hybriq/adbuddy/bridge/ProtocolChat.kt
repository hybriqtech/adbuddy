package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable

// Session and chat payloads (protocol spec §5): M3 and M4. Mirrors host/internal/bridge/protocol_chat.go.

/** A file the host keeps (spec §4.2). The app fetches it with file.get and never sees a path on the PC. */
@Serializable
data class Attachment(
    /** 32 lower-case hex digits: the id of its binary frames. */
    val id: String,
    val name: String,
    val mime: String,
    val size: Long,
    val sha256: String,
    /** Base64 JPEG of at most 16 KiB, images only. */
    val thumb: String = "",
)

/** One message of the chat. One with an id already seen replaces it. */
@Serializable
data class ChatMessage(
    val id: String,
    /** user or assistant. */
    val role: String,
    /** Markdown: shown without HTML nor remote images (spec §5.1). */
    val text: String = "",
    val attachments: List<Attachment> = emptyList(),
    val conversation: String = "",
)

/**
 * A message written on the phone. id, chosen like a rid, becomes the
 * ChatMessage's: the host drops a repeat, so retrying never duplicates it.
 * attachments: ids of attachments uploaded or captured.
 */
@Serializable
data class ChatSend(val id: String, val text: String, val attachments: List<String> = emptyList())

/** Starts a new conversation, with another agent if not empty. */
@Serializable
data class SessionNew(val agent: String = "")

/** What the agent is doing with a tool. One with an id already seen replaces it. */
@Serializable
data class ToolActivity(
    val id: String,
    val messageId: String = "",
    val summary: String,
    /** running, done or error. */
    val status: String,
)

/** A question of the agent itself (Claude Code's PermissionRequest hook), not one of ADBuddy's confirmations. */
@Serializable
data class PermissionRequest(
    val id: String,
    val tool: String,
    val summary: String,
    val detail: String = "",
    val expiresAt: String,
    /** Whether "yes, always" may be offered. */
    val allowAlways: Boolean = false,
)

/** decision: allow, allow_always or deny. */
@Serializable
data class PermissionReply(val id: String, val decision: String)

/** Takes a PermissionRequest off the phone, wherever it was answered. by: phone, terminal, timeout, agent or yolo.
 * summary: only with by yolo, what was approved (the app had no card for it).
 */
@Serializable
data class PermissionResolved(val id: String, val decision: String, val by: String, val summary: String = "")
