package tech.hybriq.adbuddy.listener

/** What the notifications source gives `notifications`: the list, or why it cannot (M5c). */
sealed interface NotificationRead {
    data class Ok(val items: List<RawNotification>) : NotificationRead

    /** No listener, or the system's access is off or cannot be told. */
    data object NoAccess : NotificationRead

    /** Allowed in the system, but the user has not confirmed it in ADBuddy's own screen. */
    data object NotConfirmed : NotificationRead
}
