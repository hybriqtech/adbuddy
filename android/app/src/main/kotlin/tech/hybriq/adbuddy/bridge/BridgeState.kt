package tech.hybriq.adbuddy.bridge

/** What the app knows about the host, rebuilt from the channel's messages. Pure, so it is unit-tested. */
data class BridgeState(
    val connection: Connection = Connection.WAITING,
    val hostName: String = "",
    val session: String = "",
    val lastSeq: Long = 0,
    val sessionState: SessionState = SessionState(),
    val pending: List<ConfirmRequest> = emptyList(),
    val journal: List<JournalEntry> = emptyList(), // newest last
    val outdated: Outdated = Outdated.NONE,
    /** Questions answered here whose final journal entry has not come yet: the UI shows them as sending. */
    val answering: Set<String> = emptySet(),
    /** The chat (M3, M4): ChatReducer. */
    val chat: ChatState = ChatState(),
    /** The remote mode (M5): RemoteReducer. null = unknown (never read as off). */
    val remote: RemoteState? = null,
    /** The YOLO mode (spec 2026-10-18): YoloReducer. null = unknown (never read as off). */
    val yolo: YoloState? = null,
) {
    /** True only once the host's welcome has come (the other states are not a live link). */
    val isConnected: Boolean get() = connection == Connection.CONNECTED

    /** INCOMPATIBLE: the app or the host must be updated; the app stops reconnecting. */
    enum class Connection { WAITING, CONNECTING, CONNECTED, INCOMPATIBLE }

    /** Which side is too old when the connection is INCOMPATIBLE. */
    enum class Outdated { NONE, APP, HOST }

    companion object {
        const val MAX_JOURNAL = 500

        /** Each area's events (protocol spec §10). A type no reducer handles only moves lastSeq. */
        val REDUCERS: List<Reducer> = listOf(ConfirmReducer, ChatReducer, RemoteReducer, YoloReducer)
    }

    /** Applies one message from the host. */
    fun apply(m: Message): BridgeState {
        if (m.type == Protocol.WELCOME) return welcome(m)
        // Too old a host or app: only a new welcome gets it out.
        if (connection == Connection.INCOMPATIBLE) return this
        // A snapshot starts over from the host's numbering; any other event already seen is a repeat.
        if (m.type == Protocol.SNAPSHOT) return snapshot(m)
        if (m.seq in 1..lastSeq) return this
        // Replies and stream messages (seq 0) keep lastSeq.
        val base = copy(lastSeq = if (m.seq > lastSeq) m.seq else lastSeq)
        val reducer = REDUCERS.firstOrNull { m.type in it.types } ?: return base
        return reducer.event(base, m)
    }

    private fun welcome(m: Message): BridgeState {
        val w = Protocol.payload<Welcome>(m)
        // A welcome this app cannot read comes from a host too old (or too odd) to talk to.
        if (w == null || w.protocol < Protocol.MIN_HOST_PROTOCOL) {
            return reset(Reset.INCOMPATIBLE).copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.HOST)
        }
        // A new session (the host restarted) starts over: nothing of the old one holds.
        if (w.session != session) {
            return reset(Reset.NEW_SESSION).copy(session = w.session, lastSeq = 0, connection = Connection.CONNECTED, outdated = Outdated.NONE)
        }
        return copy(connection = Connection.CONNECTED, outdated = Outdated.NONE)
    }

    private fun snapshot(m: Message): BridgeState {
        val seq = if (m.seq > 0) m.seq else lastSeq
        // Unreadable: what it should have replaced may be stale.
        val snap = Protocol.payload<Snapshot>(m) ?: return copy(lastSeq = seq).reset(Reset.UNREADABLE_SNAPSHOT)
        return REDUCERS.fold(copy(lastSeq = seq)) { s, r -> r.snapshot(s, snap) }
    }

    /** Every reducer forgets what of its part can no longer be trusted (Reducer.reset). */
    private fun reset(why: Reset): BridgeState = REDUCERS.fold(this) { s, r -> r.reset(s, why) }

    /** The host closed the channel because this app is too old. */
    fun appOutdated(): BridgeState =
        reset(Reset.INCOMPATIBLE).copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.APP)

    /**
     * The socket is gone. An answer sent on it may never have reached the host,
     * so its question can be answered again (the host ignores a repeat).
     */
    fun disconnected(): BridgeState = reset(Reset.DISCONNECTED).copy(connection = Connection.WAITING)

    /**
     * Marks a question the person just answered here. It stays in pending until
     * its final journal entry, so it does not vanish if the answer is lost.
     */
    fun answered(id: String): BridgeState =
        if (pending.none { it.id == id }) this else copy(answering = answering + id)
}
