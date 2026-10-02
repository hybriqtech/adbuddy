package tech.hybriq.adbuddy.ui

import java.time.Instant
import java.time.format.DateTimeParseException
import kotlinx.serialization.json.JsonElement
import tech.hybriq.adbuddy.bridge.BridgeState
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.RemoteSet
import tech.hybriq.adbuddy.bridge.RemoteState

/**
 * What the remote-mode card shows (M5). Pure, so it is unit-tested.
 * Rule: an unknown state, or one whose expiry has passed, is never painted as
 * off: a false off would hide an open port 5555.
 */
object RemoteUi {
    enum class Kind { UNKNOWN, OFF, ON, CLOSING, PENDING, DISCONNECTED }

    data class RemoteView(val kind: Kind, val address: String, val expiresAt: Instant?, val canToggle: Boolean) {
        /** Unknown and disconnected draw no switch: an unchecked one would read as "off". */
        val showSwitch: Boolean get() = kind != Kind.UNKNOWN && kind != Kind.DISCONNECTED
    }

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

    /**
     * Whether the card must still say unknown. The mark is the state.remote the card had when a
     * remote.set was answered with null; it holds while the current one is that same object
     * (identity): every remote.state or snapshot builds a new one (or null, which is unknown anyway).
     */
    fun unknownSince(hasMark: Boolean, mark: RemoteState?, current: RemoteState?): Boolean = hasMark && mark === current

    /** Ms to wait before the card must be recomputed so ON turns CLOSING; null if the expiry is missing or already passed. */
    fun msUntilClosing(expiresAt: Instant?, nowMs: Long): Long? {
        val left = (expiresAt ?: return null).toEpochMilli() - nowMs
        return if (left <= 0) null else left + 500
    }

    /**
     * Whether the app must behave as in remote mode (the tighter download limit): on, closing, or
     * not known. Only a known "off" is not remote: unknown counts as remote, never as a false off.
     */
    fun treatAsRemote(remote: RemoteState?): Boolean = remote?.enabled != false

    fun request(enabled: Boolean): JsonElement = Protocol.json.encodeToJsonElement(RemoteSet.serializer(), RemoteSet(enabled))
}
