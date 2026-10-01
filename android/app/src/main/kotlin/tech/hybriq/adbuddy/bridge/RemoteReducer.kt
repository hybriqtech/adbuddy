package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull

/** The remote mode's events (M5, protocol spec §8.2). Three values: on, off, or unknown (null). */
object RemoteReducer : Reducer {
    override val types = setOf(Protocol.REMOTE_STATE)

    // A payload this app cannot read is "unknown", never "off": a false off would
    // hide an open port 5555.
    override fun event(s: BridgeState, m: Message): BridgeState =
        s.copy(remote = read(m.payload))

    // Per area (protocol §4.3): a malformed remote part must not make the whole snapshot unreadable.
    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s.copy(
        remote = snap.remote?.let(::read),
    )

    // enabled has a default (false) and the decoder coerces nulls, so {} would read as "off":
    // it must be there as a real boolean, or the state is unknown.
    private fun read(el: JsonElement): RemoteState? {
        val enabled = (el as? JsonObject)?.get("enabled") as? JsonPrimitive
        if (enabled == null || enabled.isString || enabled.booleanOrNull == null) return null
        return try {
            Protocol.json.decodeFromJsonElement(RemoteState.serializer(), el)
        } catch (e: IllegalArgumentException) {
            null
        } catch (e: StackOverflowError) {
            null
        }
    }

    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(remote = null)
        // adbd restarting drops the channel: what was known stays as it was.
        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
    }
}
