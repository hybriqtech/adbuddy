package tech.hybriq.adbuddy.ui

import java.time.Instant
import java.time.format.DateTimeParseException
import kotlinx.serialization.json.JsonElement
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.RemoteSet

/**
 * What the remote-mode card shows (M5). Pure, so it is unit-tested.
 * Rule: an unknown state, or one whose expiry has passed, is never painted as
 * off: a false off would hide an open port 5555.
 */
object RemoteUi {
    enum class Kind { UNKNOWN, OFF, ON, CLOSING, PENDING, DISCONNECTED }

    data class RemoteView(val kind: Kind, val address: String, val expiresAt: Instant?, val canToggle: Boolean)

    /**
     * now is the host's clock (HostClock), not the phone's. unknownSince: a
     * remote.set was answered with null (the tunnel dropped when adbd restarted),
     * so what the reducer kept is stale until the next remote.state or snapshot.
     */
    fun view(state: BridgeState, pending: Boolean, unknownSince: Boolean, now: Instant): RemoteView {
        if (state.connection != BridgeState.Connection.CONNECTED) return RemoteView(Kind.DISCONNECTED, "", null, false)
        if (pending) return RemoteView(Kind.PENDING, "", null, false)
        val remote = state.remote
        if (remote == null || unknownSince) return RemoteView(Kind.UNKNOWN, "", null, false)
        if (!remote.enabled) return RemoteView(Kind.OFF, "", null, true)
        val expiry = try {
            if (remote.expiresAt.isEmpty()) null else Instant.parse(remote.expiresAt)
        } catch (e: DateTimeParseException) {
            null
        }
        // Enabled but past its expiry: the host is closing the port (it deduces it the same way).
        if (expiry != null && !expiry.isAfter(now)) return RemoteView(Kind.CLOSING, remote.address, expiry, true)
        return RemoteView(Kind.ON, remote.address, expiry, true)
    }

    fun request(enabled: Boolean): JsonElement = Protocol.json.encodeToJsonElement(RemoteSet.serializer(), RemoteSet(enabled))
}
