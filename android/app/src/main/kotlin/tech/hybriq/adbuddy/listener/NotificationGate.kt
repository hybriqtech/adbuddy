package tech.hybriq.adbuddy.listener

import tech.hybriq.adbuddy.listener.NotificationAccess.Listed

/**
 * Whether `notifications` may read now (M5c). Pure, so it is unit-tested. It needs three things, in
 * this order: the system still lists ADBuddy's listener (a revoked access clears the user's mark),
 * the user's own confirmation, and a bound listener; the confirmation is checked again after the read. What cannot be told is no access.
 */
object NotificationGate {
    fun read(
        confirmed: () -> Boolean,
        listed: () -> Listed?,
        onRevoked: () -> Unit,
        source: () -> List<RawNotification>?,
    ): NotificationRead {
        val l = listed()
        if (l == Listed.NO) {
            onRevoked()
            return NotificationRead.NoAccess
        }
        if (!confirmed()) return NotificationRead.NotConfirmed
        if (l != Listed.YES) return NotificationRead.NoAccess
        val items = source() ?: return NotificationRead.NoAccess
        // The user may have revoked while the listener was being read (it unbinds and clears the mark): that read
        // is discarded, never handed over.
        if (!confirmed()) return NotificationRead.NotConfirmed
        return NotificationRead.Ok(items)
    }

    /**
     * The system unbound the listener (the access was revoked, or the service is gone): the user's mark goes
     * with it at once, whether or not anyone is looking. Otherwise an agent could revoke the access and turn the
     * system's switch back on (`adbuddy tap`) before the gate or the screen noticed, and read without a new
     * confirmation. Allowing it again needs a new tap on ADBuddy's confirm button.
     */
    fun onUnbound(revoke: () -> Unit) = revoke()
}
