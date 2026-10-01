package tech.hybriq.adbuddy.bridge

/** The remote mode's events (M5, protocol spec §8.2). Three values: on, off, or unknown (null). */
object RemoteReducer : Reducer {
    override val types = setOf(Protocol.REMOTE_STATE)

    // A payload this app cannot read is "unknown", never "off": a false off would
    // hide an open port 5555.
    override fun event(s: BridgeState, m: Message): BridgeState =
        s.copy(remote = Protocol.payload<RemoteState>(m))

    // Per area (protocol §4.3): a malformed remote part must not make the whole snapshot unreadable.
    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s.copy(
        remote = snap.remote?.let {
            runCatching { Protocol.json.decodeFromJsonElement(RemoteState.serializer(), it) }.getOrNull()
        },
    )

    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(remote = null)
        // adbd restarting drops the channel: what was known stays as it was.
        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
    }
}
