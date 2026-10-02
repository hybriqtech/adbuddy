package tech.hybriq.adbuddy.listener

import android.app.Notification

/**
 * Whether a notification is secret because of what the system says about it (M5c). Pure, so it is
 * unit-tested: only what is known to be visible is visible. No Ranking, or none for this notification, is
 * secret (failing open here would let a code out whenever the system's ranking cannot be had).
 */
internal object Sensitivity {
    fun of(rankingKnown: Boolean, lockscreenOverride: Int?, channelVisibility: Int?): Boolean =
        !rankingKnown ||
            lockscreenOverride == Notification.VISIBILITY_SECRET ||
            channelVisibility == Notification.VISIBILITY_SECRET
}
