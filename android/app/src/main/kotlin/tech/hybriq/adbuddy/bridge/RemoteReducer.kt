package tech.hybriq.adbuddy.bridge

/** The remote mode's events (M5, protocol spec §8.2). Until M5 implements them, it changes nothing. */
object RemoteReducer : Reducer {
    override val types = setOf(Protocol.REMOTE_STATE)

    override fun event(s: BridgeState, m: Message): BridgeState = s

    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s
}
