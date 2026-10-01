package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

// Helper and remote mode payloads (protocol spec §8): M5. Mirrors host/internal/bridge/protocol_ui.go.

/**
 * Asks the app to act on the screen. A stream message: never resent after a
 * reconnection; dropped once expired (judged with the host's clock) and never
 * run twice for the same id.
 */
@Serializable
data class UiRequest(
    val id: String,
    /** dump, click, setText, foreground or notifications. */
    val op: String,
    val args: JsonElement = JsonObject(emptyMap()),
    val expiresAt: String,
)

/** Answers a UiRequest: result when ok, error when not. */
@Serializable
data class UiResponse(val id: String, val ok: Boolean, val result: JsonElement? = null, val error: ErrorReply? = null)

/**
 * How to reach the phone away from home (empty and 0: unknown), and the
 * protected apps the host's guard refuses to act on (M5 spec §4.3).
 * protectedApps null (absent) means "no change": the host keeps the last
 * list; present, even empty, it is the whole list.
 */
@Serializable
data class DeviceInfo(val tailscaleIp: String = "", val wifiAdbPort: Int = 0, val protectedApps: List<String>? = null)

/** Turns remote mode on (adb tcpip 5555) or off (adb usb). */
@Serializable
data class RemoteSet(val enabled: Boolean)

/** The remote mode as the host has it. address: ip:port while enabled. */
@Serializable
data class RemoteState(val enabled: Boolean = false, val address: String = "", val expiresAt: String = "")
