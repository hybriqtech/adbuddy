package tech.hybriq.adbuddy.listener

import android.app.Notification

/**
 * Whether a notification is secret because of what the system says about it (M5c). Pure, so it is
 * unit-tested: only what is known to be visible is visible. The channel's visibility
 * exists from API 26 and the Ranking's override from API 31 (null before). No Ranking, or none for this notification, is
 * secret (failing open here would let a code out whenever the system's ranking cannot be had).
 */
internal object Sensitivity {
    fun of(rankingKnown: Boolean, lockscreenOverride: Int?, channelVisibility: Int?): Boolean =
        !rankingKnown ||
            hides(lockscreenOverride) ||
            hides(channelVisibility)

    // SECRET, or an explicit PRIVATE (no content on the lock screen): the user, the assistant or the channel chose it.
    // VISIBILITY_NO_OVERRIDE (the default, "no choice") and PUBLIC do not hide it.
    private fun hides(visibility: Int?) =
        visibility == Notification.VISIBILITY_SECRET || visibility == Notification.VISIBILITY_PRIVATE
}
