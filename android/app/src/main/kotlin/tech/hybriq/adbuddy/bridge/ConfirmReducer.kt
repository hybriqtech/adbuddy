package tech.hybriq.adbuddy.bridge

/** M2: the session's state, the 🔴 questions and the journal. */
object ConfirmReducer : Reducer {
    override val types = setOf(Protocol.SESSION_STATE, Protocol.CONFIRM_REQUEST, Protocol.JOURNAL_ENTRY)

    override fun event(s: BridgeState, m: Message): BridgeState = when (m.type) {
        Protocol.SESSION_STATE -> s.copy(sessionState = Protocol.payload<SessionState>(m) ?: s.sessionState)
        Protocol.CONFIRM_REQUEST -> {
            val r = Protocol.payload<ConfirmRequest>(m)
            if (r == null) s else s.copy(pending = s.pending.filter { it.id != r.id } + r)
        }
        Protocol.JOURNAL_ENTRY -> {
            val e = Protocol.payload<JournalEntry>(m)
            if (e == null) {
                s
            } else {
                // A final record of a pending question means it was answered (here, on the PC or by expiry).
                val final = e.status != "running"
                val stillPending = if (final) s.pending.filter { it.id != e.id } else s.pending
                // A newer record of the same command takes its place in the list.
                val i = s.journal.indexOfFirst { it.id == e.id }
                val updated = if (i >= 0) s.journal.toMutableList().also { it[i] = e } else s.journal + e
                s.copy(
                    pending = stillPending, journal = updated.takeLast(BridgeState.MAX_JOURNAL),
                    answering = if (final) s.answering - e.id else s.answering,
                )
            }
        }
        else -> s
    }

    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState {
        val entries = snap.journal.mapNotNull {
            try {
                Protocol.json.decodeFromJsonElement(JournalEntry.serializer(), it)
            } catch (e: IllegalArgumentException) {
                null
            } catch (e: StackOverflowError) {
                null
            }
        }
        val ids = snap.pending.map { it.id }.toSet()
        return s.copy(
            sessionState = snap.state, pending = snap.pending, journal = entries.takeLast(BridgeState.MAX_JOURNAL),
            answering = s.answering.filter { it in ids }.toSet(),
        )
    }
}
