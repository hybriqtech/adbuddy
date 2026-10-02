package tech.hybriq.adbuddy.listener

import android.app.Notification
import org.junit.Assert.assertEquals
import org.junit.Test

class SensitivityTest {
    private val secret = Notification.VISIBILITY_SECRET
    private val private = Notification.VISIBILITY_PRIVATE
    private val public = Notification.VISIBILITY_PUBLIC
    private val noOverride = -1000 // Ranking.VISIBILITY_NO_OVERRIDE

    @Test fun withoutARankingForTheNotificationItIsSecretNeverVisible() {
        // The system's ranking could not be had (null, an error), or has no entry for this key: nothing is known.
        for (override in listOf(null, secret, private, public, noOverride)) {
            for (channel in listOf(null, secret, private, public, noOverride)) {
                assertEquals("$override/$channel", true, Sensitivity.of(rankingKnown = false, lockscreenOverride = override, channelVisibility = channel))
            }
        }
    }

    @Test fun theUserTheAssistantOrTheChannelHidingItFromTheLockScreenMakesItSecret() {
        // SECRET and an explicit PRIVATE (the lock screen shows no content) count, from either side, whatever the other says.
        for (hidden in listOf(secret, private)) {
            for (other in listOf(null, secret, private, public, noOverride)) {
                assertEquals("override $hidden/$other", true, Sensitivity.of(true, lockscreenOverride = hidden, channelVisibility = other))
                assertEquals("channel $other/$hidden", true, Sensitivity.of(true, lockscreenOverride = other, channelVisibility = hidden))
            }
        }
    }

    @Test fun aKnownRankingThatHidesNothingLeavesItVisible() {
        // No channel before API 26 and no override before API 31 are null; NO_OVERRIDE is the default, not a choice.
        for (override in listOf(null, public, noOverride)) {
            for (channel in listOf(null, public, noOverride)) {
                assertEquals("$override/$channel", false, Sensitivity.of(true, lockscreenOverride = override, channelVisibility = channel))
            }
        }
    }
}
