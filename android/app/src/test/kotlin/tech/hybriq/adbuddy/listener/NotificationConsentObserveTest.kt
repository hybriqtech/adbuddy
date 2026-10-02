package tech.hybriq.adbuddy.listener

import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertEquals
import org.junit.Test
import tech.hybriq.adbuddy.FakePrefs

class NotificationConsentObserveTest {
    @Test fun theScreenSeesTheMarkAsItIsClearedAndSetAndStopsListeningWhenItLeaves() = runBlocking {
        val prefs = FakePrefs()
        val consent = NotificationConsent(prefs)
        val seen = Channel<Boolean>(Channel.UNLIMITED)
        val job = launch { consent.observe().collect { seen.send(it) } }
        withTimeout(5000) {
            assertEquals("the value now, without a change", false, seen.receive())
            // Whoever sets it (the button, in the real app) is seen; observing itself never sets it.
            NotificationConsent(prefs).confirm()
            assertEquals(true, seen.receive())
            // The listener clears it as the system unbinds it, in another instance: the screen follows.
            NotificationConsent(prefs).revoke()
            assertEquals(false, seen.receive())
        }
        job.cancel()
        job.join()
        assertEquals("no listener is left registered", 0, prefs.listenerCount)
        assertEquals("observing never set the mark", false, consent.confirmed())
    }
}
