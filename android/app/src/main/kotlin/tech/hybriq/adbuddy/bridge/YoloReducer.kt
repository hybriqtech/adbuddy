package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull

/**
 * The YOLO mode's events (protocol spec §8.3). Three values: on, off, or unknown (null). A payload this app cannot
 * read is "unknown", never "off": a false off would hide the band of a mode that is on.
 */
object YoloReducer : Reducer {
    override val types = setOf(Protocol.YOLO_STATE)

    override fun event(s: BridgeState, m: Message): BridgeState = s.copy(yolo = read(m.payload))

    // Per area (protocol §4.3): a malformed yolo part must not make the whole snapshot unreadable.
    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s.copy(yolo = snap.yolo?.let(::read))

    // active has a default (false) and the decoder coerces nulls, so {} would read as "off": it must be there as a
    // real boolean, or the state is unknown.
    private fun read(el: JsonElement): YoloState? {
        val active = (el as? JsonObject)?.get("active") as? JsonPrimitive
        if (active == null || active.isString || active.booleanOrNull == null) return null
        return try {
            Protocol.json.decodeFromJsonElement(YoloState.serializer(), el)
        } catch (e: IllegalArgumentException) {
            null
        } catch (e: StackOverflowError) {
            null
        }
    }

    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(yolo = null)
        // The mode lives on the host and outlives the link (spec): what was known stays as it was.
        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
    }
}
