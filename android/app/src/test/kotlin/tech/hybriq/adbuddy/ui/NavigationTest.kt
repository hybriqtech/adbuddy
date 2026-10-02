package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.FakePrefs

class NavigationTest {
    @Test fun backWalksUpToTheChatAndThenLeaves() {
        assertNull(Route.CHAT.parent)
        assertEquals(Route.CHAT, Route.ACTIVITY.parent)
        assertEquals(Route.CHAT, Route.SETTINGS.parent)
        assertEquals(Route.SETTINGS, Route.PROTECTED_APPS.parent)
        assertEquals(Route.SETTINGS, Route.NOTIFICATION_ACCESS.parent)
        // Every route reaches the chat, so Back can never loop.
        for (r in Route.entries) {
            var at: Route? = r
            var steps = 0
            while (at != null && at != Route.CHAT) { at = at.parent; steps++ }
            assertTrue("$r", at == Route.CHAT && steps <= 2)
        }
    }

    @Test fun theOnboardingGoesProtectedAppsThenNotificationAccess() {
        assertEquals(2, OnboardingStep.total)
        assertEquals(1, OnboardingStep.PROTECTED_APPS.number)
        assertEquals(2, OnboardingStep.NOTIFICATION_ACCESS.number)
        assertEquals(OnboardingStep.NOTIFICATION_ACCESS, OnboardingStep.PROTECTED_APPS.next())
        assertNull(OnboardingStep.NOTIFICATION_ACCESS.next())
        assertNull(OnboardingStep.PROTECTED_APPS.previous())
        assertEquals(OnboardingStep.PROTECTED_APPS, OnboardingStep.NOTIFICATION_ACCESS.previous())
    }

    @Test fun theBubbleNeverShowsTheOnboarding() {
        assertEquals(OnboardingStep.PROTECTED_APPS, OnboardingStep.initial(firstRun = true, done = false))
        assertNull(OnboardingStep.initial(firstRun = true, done = true))
        // The chat bubble opens no first-run screen (M4: that is MainActivity's), done or not.
        assertNull(OnboardingStep.initial(firstRun = false, done = false))
        assertNull(OnboardingStep.initial(firstRun = false, done = true))
    }

    @Test fun theOnboardingShowsUntilItIsFinishedAndThenNeverAgain() {
        val prefs = FakePrefs()
        assertFalse(Onboarding(prefs).done())
        Onboarding(prefs).finish()
        assertTrue(Onboarding(prefs).done()) // a new instance reads it from the same preferences
    }

    @Test fun theOnboardingIsNotDoneByDefaultEvenWithOtherKeysAndFinishWritesOnlyItsOwn() {
        val prefs = FakePrefs()
        prefs.edit().putBoolean("confirmed", true).putString("other", "x").commit()
        assertFalse(Onboarding(prefs).done())
        Onboarding(prefs).finish()
        assertTrue(Onboarding(prefs).done())
        assertEquals(setOf("confirmed", "other", "done"), prefs.map.keys)
        assertEquals(true, prefs.map["confirmed"])
        // On clean preferences, finish() writes just the done key.
        val clean = FakePrefs()
        Onboarding(clean).finish()
        assertEquals(setOf("done"), clean.map.keys)
    }
}
