package tech.hybriq.adbuddy.notify

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.util.ReflectionHelpers
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.ChatBubbleActivity
import tech.hybriq.adbuddy.MainActivity

/** AdbuddyApp.foreground: true while at least one activity of the app is started (the bubble counts). */
@RunWith(RobolectricTestRunner::class)
class AppForegroundTest {
    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    @Test fun followsTheStartedActivities() {
        assertFalse(app.foreground.value)
        val main = Robolectric.buildActivity(MainActivity::class.java).create().start()
        assertTrue(app.foreground.value)
        val bubble = Robolectric.buildActivity(ChatBubbleActivity::class.java).create().start()
        main.stop()
        assertTrue("the bubble is still in front", app.foreground.value)
        bubble.stop()
        assertFalse(app.foreground.value)
    }

    // A recreation (rotation, dark mode, language) stops the activity and starts it again at once: foreground must not
    // flicker to false in between, or the collector would post the notice for a moment while the person looks at the card.
    @Test fun aConfigurationChangeNeverDropsForeground() {
        val seen = mutableListOf<Boolean>()
        val main = Robolectric.buildActivity(MainActivity::class.java).create().start()
        val job = CoroutineScope(Dispatchers.Unconfined).launch { app.foreground.collect { seen.add(it) } }
        // What the system does when it recreates: the activity is flagged as changing configurations, then stopped.
        ReflectionHelpers.setField(main.get(), "mChangingConfigurations", true)
        assertTrue(main.get().isChangingConfigurations)
        main.stop()
        assertTrue("still in front while it is recreated", app.foreground.value)
        val again = Robolectric.buildActivity(MainActivity::class.java).create().start()
        assertTrue(app.foreground.value)
        assertFalse("never false: $seen", seen.contains(false))
        // The recreated activity then stops for real: it drops.
        again.stop()
        assertFalse(app.foreground.value)
        job.cancel()
    }

    @Test fun aRealStopDropsForeground() {
        val main = Robolectric.buildActivity(MainActivity::class.java).create().start()
        assertFalse(main.get().isChangingConfigurations)
        main.stop()
        assertFalse(app.foreground.value)
    }

    @Test fun oneActivityStartsAndStops() {
        val main = Robolectric.buildActivity(MainActivity::class.java).create().start()
        assertTrue(app.foreground.value)
        main.stop()
        assertFalse(app.foreground.value)
    }
}
