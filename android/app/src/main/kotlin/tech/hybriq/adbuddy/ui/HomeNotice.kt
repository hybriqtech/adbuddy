package tech.hybriq.adbuddy.ui

/**
 * The one card under the header (spec 2026-10-18 R8), instead of stacked strips. Red questions first (they expire),
 * then what stops everything (cut, versions, not set up), then blocked notifications, pause and waiting.
 * banner: the LinkStatus that deserves one (ChatScreen keeps the last one through CONNECTING), or null.
 */
enum class HomeNotice(val hasAction: Boolean) {
    QUESTIONS(true), CUT(true), UPDATE_APP(false), UPDATE_HOST(false), NOT_SET_UP(false), NOTIFICATIONS(true), PAUSED(true), WAITING(true);

    companion object {
        fun of(banner: LinkStatus?, pendingQuestions: Int, notificationsBlocked: Boolean): HomeNotice? = when {
            pendingQuestions > 0 -> QUESTIONS
            banner == LinkStatus.CUT -> CUT
            banner == LinkStatus.UPDATE_APP -> UPDATE_APP
            banner == LinkStatus.UPDATE_HOST -> UPDATE_HOST
            banner == LinkStatus.NOT_SET_UP -> NOT_SET_UP
            notificationsBlocked -> NOTIFICATIONS
            banner == LinkStatus.PAUSED -> PAUSED
            banner == LinkStatus.WAITING -> WAITING
            else -> null
        }
    }
}
