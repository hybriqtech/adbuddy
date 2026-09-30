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
        if (m.type == Protocol.WELCOME) return welcome(m)
        // Too old a host or app: only a new welcome gets it out.
        if (connection == Connection.INCOMPATIBLE) return this
        // A snapshot starts over from the host's numbering; any other event already seen is a repeat.
        if (m.type == Protocol.SNAPSHOT) return snapshot(m)
        if (m.seq in 1..lastSeq) return this
        val seq = if (m.seq > lastSeq) m.seq else lastSeq
        return when (m.type) {
            Protocol.SESSION_STATE -> copy(lastSeq = seq, sessionState = Protocol.payload<SessionState>(m) ?: sessionState)
            Protocol.CONFIRM_REQUEST -> {
                val r = Protocol.payload<ConfirmRequest>(m) ?: return copy(lastSeq = seq)
                copy(lastSeq = seq, pending = pending.filter { it.id != r.id } + r)
            }
            Protocol.JOURNAL_ENTRY -> {
                val e = Protocol.payload<JournalEntry>(m) ?: return copy(lastSeq = seq)
                // A final record of a pending question means it was answered (here, on the PC or by expiry).
                val stillPending = if (e.status != "running") pending.filter { it.id != e.id } else pending
                // A newer record of the same command takes its place in the list.
                val i = journal.indexOfFirst { it.id == e.id }
                val updated = if (i >= 0) journal.toMutableList().also { it[i] = e } else journal + e
                copy(lastSeq = seq, pending = stillPending, journal = updated.takeLast(MAX_JOURNAL))
            }
            else -> copy(lastSeq = seq)
        }
    }

    private fun welcome(m: Message): BridgeState {
        val w = Protocol.payload<Welcome>(m)
        // A welcome this app cannot read comes from a host too old (or too odd) to talk to.
        if (w == null || w.protocol < Protocol.MIN_HOST_PROTOCOL) {
            return copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.HOST, pending = emptyList())
        }
        // A new session (the host restarted) starts over: nothing of the old one holds.
        if (w.session != session) {
            return copy(
                session = w.session, lastSeq = 0, sessionState = SessionState(), pending = emptyList(),
                connection = Connection.CONNECTED, outdated = Outdated.NONE,
            )
        }
        return copy(connection = Connection.CONNECTED, outdated = Outdated.NONE)
    }

    private fun snapshot(m: Message): BridgeState {
        val seq = if (m.seq > 0) m.seq else lastSeq
        // Unreadable: the old questions may be answered already, so none is kept.
        val s = Protocol.payload<Snapshot>(m) ?: return copy(lastSeq = seq, pending = emptyList())
        val entries = s.journal.mapNotNull {
            try {
                Protocol.json.decodeFromJsonElement(JournalEntry.serializer(), it)
            } catch (e: IllegalArgumentException) {
                null
            } catch (e: StackOverflowError) {
                null
            }
        }
        return copy(lastSeq = seq, sessionState = s.state, pending = s.pending, journal = entries.takeLast(MAX_JOURNAL))
    }

    /** The host closed the channel because this app is too old. */
    fun appOutdated(): BridgeState = copy(connection = Connection.INCOMPATIBLE, outdated = Outdated.APP, pending = emptyList())

    /** Removes a question the person just answered here. */
    fun answered(id: String): BridgeState = copy(pending = pending.filter { it.id != id })
}
