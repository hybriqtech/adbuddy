package tech.hybriq.adbuddy.listener

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.UiLimits
import tech.hybriq.adbuddy.bridge.UiNotifications

class NotificationReportTest {
    private val at = Instant.parse("2026-10-01T09:58:00Z").toEpochMilli()
    private val bank = setOf("com.example.bank")
    private val guarded: (String) -> Boolean = { it in bank }

    private fun raw(
        pkg: String,
        title: String = "t",
        text: String = "x",
        key: String = "0|$pkg|1|null|1",
        postedAt: Long = at,
        ongoing: Boolean = false,
        secret: Boolean = false,
    ) = RawNotification(key, pkg, title, text, postedAt, ongoing, secret)

    @Test fun newestFirstWithTheTimeInUtc() {
        val r = NotificationReport.build(
            listOf(raw("com.a.old", postedAt = at), raw("com.a.new", postedAt = at + 90_000, ongoing = true), raw("com.a.mid", postedAt = at + 1_999)),
            guarded,
        )
        assertEquals(listOf("com.a.new", "com.a.mid", "com.a.old"), r.notifications.map { it.`package` })
        assertEquals("2026-10-01T09:59:30Z", r.notifications[0].postedAt)
        assertEquals("2026-10-01T09:58:01Z", r.notifications[1].postedAt) // milliseconds are cut
        assertTrue(r.notifications[0].ongoing)
        assertFalse(r.truncated)
    }

    @Test fun aProtectedAppShowsNoContentButItsPackage() {
        val r = NotificationReport.build(
            listOf(raw("com.example.bank", "Bank", "Your code is 482913", key = "0|com.example.bank|7|otp-482913|10140"), raw("com.whatsapp", "Ana", "¿Comemos?")),
            guarded,
        )
        val b = r.notifications.single { it.`package` == "com.example.bank" }
        assertEquals("", b.title)
        assertEquals("", b.text)
        assertFalse("the key carries the app's tag", b.key.contains("482913"))
        val w = r.notifications.single { it.`package` == "com.whatsapp" }
        assertEquals("Ana", w.title)
        assertEquals("¿Comemos?", w.text)
        assertEquals("0|com.whatsapp|1|null|1", w.key)
    }

    @Test fun adbuddyItselfShowsNoContent() {
        val r = NotificationReport.build(listOf(raw("tech.hybriq.adbuddy", "The agent wants to: reboot", "adb shell reboot")), { false })
        assertEquals("tech.hybriq.adbuddy", r.notifications[0].`package`)
        assertEquals("", r.notifications[0].title)
        assertEquals("", r.notifications[0].text)
    }

    @Test fun aNotificationMarkedSecretShowsNoContent() {
        val r = NotificationReport.build(listOf(raw("com.other.app", "Code", "482913", secret = true)), { false })
        assertEquals("com.other.app", r.notifications[0].`package`)
        assertEquals("", r.notifications[0].text)
        assertEquals("", r.notifications[0].title)
    }

    @Test fun aPackageThatIsNotOneIsHiddenAndNotShown() {
        for (pkg in listOf("", "bank", "a b.c", "x\u001b[2J.y")) {
            val r = NotificationReport.build(listOf(raw(pkg, "t", "482913")), { false })
            assertEquals("", r.notifications[0].`package`)
            assertEquals("", r.notifications[0].text)
        }
    }

    @Test fun theProtectedListIsAskedEachTime() {
        val live = mutableSetOf<String>()
        val list = listOf(raw("com.example.bank", text = "482913"))
        assertEquals("482913", NotificationReport.build(list) { it in live }.notifications[0].text)
        live += "com.example.bank"
        assertEquals("", NotificationReport.build(list) { it in live }.notifications[0].text)
    }

    @Test fun noSecretLeavesInTheEncodedAnswer() {
        val r = NotificationReport.build(
            listOf(
                raw("com.example.bank", "Bank", "Your code is 482913", key = "0|com.example.bank|7|otp-482913|10140"),
                raw("tech.hybriq.adbuddy", "Question", "482913"),
                raw("com.other.app", "x", "482913", secret = true),
                raw("not a package", "x", "482913"),
            ),
            guarded,
        )
        val json = Protocol.json.encodeToString(UiNotifications.serializer(), r)
        assertFalse(json, json.contains("482913"))
        assertFalse(json, json.contains("otp-"))
    }

    @Test fun atMostFiftyTheNewestKept() {
        val many = (0 until 60).map { raw("com.a.app", key = "k$it", postedAt = at + it * 1000L) }
        val r = NotificationReport.build(many, guarded)
        assertEquals(UiLimits.MAX_NOTIFICATIONS, r.notifications.size)
        assertTrue(r.truncated)
        assertEquals("k59", r.notifications[0].key)
        assertEquals("k10", r.notifications.last().key)
    }

    @Test fun textIsCutToTheLimitInCodePoints() {
        val emoji = "😀".repeat(UiLimits.MAX_NOTIFICATION_TEXT + 5) // 2 UTF-16 units each
        val r = NotificationReport.build(listOf(raw("com.a.app", emoji, emoji)), guarded)
        for (s in listOf(r.notifications[0].title, r.notifications[0].text)) {
            assertEquals(UiLimits.MAX_NOTIFICATION_TEXT, s.codePointCount(0, s.length))
            assertEquals(s.length, UiLimits.MAX_NOTIFICATION_TEXT * 2) // no half pair
        }
        assertEquals("short", NotificationReport.clip("short"))
    }

    @Test fun theAnswerFitsWhatTheHostAccepts() {
        // Control characters take 6 bytes each once escaped: 50 of these do not fit in 512 KiB.
        val heavy = "\u0001".repeat(UiLimits.MAX_NOTIFICATION_TEXT)
        val many = (0 until 50).map { raw("com.a.app", heavy, heavy, key = "k$it", postedAt = at + it * 1000L) }
        val r = NotificationReport.build(many, guarded)
        val bytes = Protocol.json.encodeToString(UiNotifications.serializer(), r).toByteArray(Charsets.UTF_8).size
        assertTrue("$bytes bytes", bytes <= UiLimits.MAX_UI_RESULT)
        assertTrue(r.truncated)
        assertTrue(r.notifications.isNotEmpty())
        assertEquals("k49", r.notifications[0].key) // the oldest were dropped, not the newest
    }

    @Test fun nothingIsNothing() {
        val r = NotificationReport.build(emptyList(), guarded)
        assertTrue(r.notifications.isEmpty())
        assertFalse(r.truncated)
    }
}
