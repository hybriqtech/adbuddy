package tech.hybriq.adbuddy.notify

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
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

    @Test fun oneActivityStartsAndStops() {
        val main = Robolectric.buildActivity(MainActivity::class.java).create().start()
        assertTrue(app.foreground.value)
        main.stop()
        assertFalse(app.foreground.value)
    }
}
