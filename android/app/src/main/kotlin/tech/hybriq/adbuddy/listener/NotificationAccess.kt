package tech.hybriq.adbuddy.listener

/**
 * Whether the user has allowed ADBuddy to read notifications (M5c). Three
 * values, as the remote mode (M5b ruling): GRANTED, NOT_GRANTED and UNKNOWN,
 * and what cannot be told is UNKNOWN, never painted as granted nor as denied.
 * Pure, so it is unit-tested; NotificationAccessStatus reads the real setting.
 */
object NotificationAccess {
    enum class State { GRANTED, NOT_GRANTED, UNKNOWN }

    /** What the system's list of enabled listeners says about ADBuddy's. */
    enum class Listed { YES, NO, UNKNOWN }

    const val PACKAGE = "tech.hybriq.adbuddy"
    const val CLASS = "tech.hybriq.adbuddy.listener.AgentNotificationListener"

    /**
     * flat is Settings.Secure "enabled_notification_listeners": flattened
     * component names (package/class, with ".Class" standing for package.Class)
     * joined by ":". An absent or blank value means no listener is enabled; an
     * entry that cannot be read, with ADBuddy's not among the others, means
     * UNKNOWN: it could have been ours.
     */
    fun listed(flat: String?): Listed {
        if (flat.isNullOrBlank()) return Listed.NO
        var unreadable = false
        for (entry in flat.split(':')) {
            if (entry.isBlank()) continue
            val slash = entry.indexOf('/')
            if (slash <= 0 || slash == entry.length - 1) {
                unreadable = true
                continue
            }
            val pkg = entry.substring(0, slash)
            val cls = entry.substring(slash + 1).let { if (it.startsWith(".")) pkg + it else it }
            if (pkg == PACKAGE && cls == CLASS) return Listed.YES
        }
        return if (unreadable) Listed.UNKNOWN else Listed.NO
    }

    /**
     * What the two sources say: the system's own answer (NotificationManager.isNotificationListenerAccessGranted;
     * api null: it failed) and the setting (null: it could not be read). Both known and different is not an
     * answer, and neither known is none.
     */
    fun combine(api: Boolean?, setting: Listed?): Listed? {
        val a = api?.let { if (it) Listed.YES else Listed.NO }
        return when {
            a == null -> setting
            setting == null || setting == Listed.UNKNOWN -> a
            a == setting -> a
            else -> Listed.UNKNOWN
        }
    }

    /**
     * What the screens show: the system's state and whether the user confirmed it in ADBuddy. Allowed in
     * the system is not enough for the agent to read: NEEDS_CONFIRMATION until the person taps confirm.
     */
    enum class Effective { OFF, UNKNOWN, NEEDS_CONFIRMATION, ON }

    fun effective(state: State, confirmed: Boolean): Effective = when (state) {
        State.NOT_GRANTED -> Effective.OFF
        State.UNKNOWN -> Effective.UNKNOWN
        State.GRANTED -> if (confirmed) Effective.ON else Effective.NEEDS_CONFIRMATION
    }

    /**
     * listed is null when the setting could not be read. Allowed in the setting
     * but not connected is UNKNOWN too (the system has not bound the listener
     * yet, or the binding is stuck: turning access off and on fixes it).
     */
    fun state(listed: Listed?, connected: Boolean): State = when (listed) {
        null, Listed.UNKNOWN -> State.UNKNOWN
        Listed.NO -> State.NOT_GRANTED
        Listed.YES -> if (connected) State.GRANTED else State.UNKNOWN
    }
}
