package tech.hybriq.adbuddy.a11y

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationWindowTest {
    private var t = 1_000L
    private val w = NotificationWindow({ t })

    @Test fun noEventsNotActive() = assertFalse(w.active())

    @Test fun anotherPackageActivatesUntilTheWindowEnds() {
        w.onNotification("com.example.chat")
        assertTrue(w.active())
        t += 7_999
        assertTrue(w.active())
        t += 1
        assertFalse(w.active())
    }

    @Test fun adbuddyAndEmptyPackagesDoNotActivate() {
        w.onNotification("tech.hybriq.adbuddy")
        w.onNotification("")
        assertFalse(w.active())
    }

    @Test fun aSecondEventRestartsTheWindow() {
        w.onNotification("com.example.chat")
        t += 5_000
        w.onNotification("com.example.mail")
        t += 5_000
        assertTrue(w.active())
        t += 3_000
        assertFalse(w.active())
    }

    @Test fun acceptIgnoresToastsAndEventsWithoutANotification() {
        assertFalse(w.accept("android.widget.Toast\$TN", "com.example.chat", true))
        assertFalse(w.accept("android.widget.Toast", "com.example.chat", true))
        assertFalse(w.accept(null, "com.example.chat", false))
        assertFalse(w.accept("android.app.Notification", "com.example.chat", false))
        assertFalse(w.active())
        assertTrue(w.accept("android.app.Notification", "com.example.chat", true))
        assertTrue(w.active())
    }

    @Test fun acceptWithoutClassNameStillCountsANotification() {
        assertTrue(w.accept(null, "com.example.chat", true))
        assertTrue(w.active())
    }
}
