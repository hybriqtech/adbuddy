package tech.hybriq.adbuddy.bridge

/**
 * One area of BridgeState (protocol spec §10): the event types it handles and
 * the part of a snapshot it takes. Each milestone keeps its reducer in its own
 * file, so lines of work in parallel do not edit the same code.
 */
interface Reducer {
    /** The event types it handles; no two reducers share one (BridgeStateTest). */
    val types: Set<String>

    /** Applies one event of its types. s already has the event's lastSeq. */
    fun event(s: BridgeState, m: Message): BridgeState

    /** Takes its part of a readable snapshot. s already has the snapshot's lastSeq. */
    fun snapshot(s: BridgeState, snap: Snapshot): BridgeState

    /** Forgets what of its part can no longer be trusted, for the reason why. */
    fun reset(s: BridgeState, why: Reset): BridgeState
}

/** Why BridgeState asks every reducer to forget part of what it knows. */
enum class Reset {
    /** The host restarted: nothing of the old session holds. */
    NEW_SESSION,

    /** A snapshot could not be read: what it should have replaced may be stale. */
    UNREADABLE_SNAPSHOT,

    /** The socket is gone: what was sent on it may never have reached the host. */
    DISCONNECTED,

    /** The host or the app is too old: nothing more comes until a new welcome. */
    INCOMPATIBLE,
}
