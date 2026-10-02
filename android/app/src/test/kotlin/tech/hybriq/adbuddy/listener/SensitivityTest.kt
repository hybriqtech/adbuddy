package tech.hybriq.adbuddy.listener

import android.app.Notification
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SensitivityTest {
    private val secret = Notification.VISIBILITY_SECRET
    private val noOverride = -1000 // Ranking.VISIBILITY_NO_OVERRIDE
    private val public = Notification.VISIBILITY_PUBLIC

    @Test fun withoutARankingForTheNotificationItIsSecretNeverVisible() {
        // The system's ranking could not be had, or has no entry for this key: nothing is known, so nothing is shown.
        assertTrue(Sensitivity.of(rankingKnown = false, lockscreenOverride = null, channelVisibility = null))
        assertTrue(Sensitivity.of(rankingKnown = false, lockscreenOverride = public, channelVisibility = public))
    }

    @Test fun theUserOrTheChannelHidingItFromTheLockScreenMakesItSecret() {
        assertTrue(Sensitivity.of(true, lockscreenOverride = secret, channelVisibility = null))
        assertTrue(Sensitivity.of(true, lockscreenOverride = null, channelVisibility = secret))
        assertTrue(Sensitivity.of(true, lockscreenOverride = noOverride, channelVisibility = secret))
    }

    @Test fun aKnownRankingThatHidesNothingLeavesItVisible() {
        assertFalse(Sensitivity.of(true, lockscreenOverride = null, channelVisibility = null)) // before Android 12, no channel
        assertFalse(Sensitivity.of(true, lockscreenOverride = noOverride, channelVisibility = noOverride))
        assertFalse(Sensitivity.of(true, lockscreenOverride = public, channelVisibility = public))
    }
}
