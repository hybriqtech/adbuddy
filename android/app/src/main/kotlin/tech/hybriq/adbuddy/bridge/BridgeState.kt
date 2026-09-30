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
) {
    /** INCOMPATIBLE: the app or the host must be updated; the app stops reconnecting. */
    enum class Connection { WAITING, CONNECTING, CONNECTED, INCOMPATIBLE }

    /** Which side is too old when the connection is INCOMPATIBLE. */
    enum class Outdated { NONE, APP, HOST }

    companion object {
        const val MAX_JOURNAL = 500
    }

    /** Applies one message from the host. */
    fun apply(m: Message): BridgeState {
        val seq = if (m.seq > lastSeq) m.seq else lastSeq
        return when (m.type) {
            Protocol.WELCOME -> {
                val w = Protocol.payload<Welcome>(m) ?: return this
                if (w.protocol < Protocol.MIN_HOST_PROTOCOL) {
                    return copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.HOST)
                }
                // A new session (the host restarted) starts over.
                if (w.session != session) copy(session = w.session, lastSeq = 0, pending = emptyList(), connection = Connection.CONNECTED)
                else copy(connection = Connection.CONNECTED)
            }
            Protocol.SNAPSHOT -> {
                val s = Protocol.payload<Snapshot>(m) ?: return copy(lastSeq = seq)
                val entries = s.journal.mapNotNull {
                    try { Protocol.json.decodeFromJsonElement(JournalEntry.serializer(), it) } catch (e: IllegalArgumentException) { null }
                }
                copy(lastSeq = seq, sessionState = s.state, pending = s.pending, journal = entries.takeLast(MAX_JOURNAL))
            }
            Protocol.SESSION_STATE -> copy(lastSeq = seq, sessionState = Protocol.payload<SessionState>(m) ?: sessionState)
            Protocol.CONFIRM_REQUEST -> {
                val r = Protocol.payload<ConfirmRequest>(m) ?: return copy(lastSeq = seq)
                copy(lastSeq = seq, pending = pending.filter { it.id != r.id } + r)
            }
            Protocol.JOURNAL_ENTRY -> {
                val e = Protocol.payload<JournalEntry>(m) ?: return copy(lastSeq = seq)
                // A final record of a pending question means it was answered (here, on the PC or by expiry).
                val stillPending = if (e.status != "running") pending.filter { it.id != e.id } else pending
                copy(lastSeq = seq, pending = stillPending, journal = (journal.filter { it.id != e.id } + e).takeLast(MAX_JOURNAL))
            }
            else -> copy(lastSeq = seq)
        }
    }

    /** The host closed the channel because this app is too old. */
    fun appOutdated(): BridgeState = copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.APP, pending = emptyList())

    /** Removes a question the person just answered here. */
    fun answered(id: String): BridgeState = copy(pending = pending.filter { it.id != id })
}
