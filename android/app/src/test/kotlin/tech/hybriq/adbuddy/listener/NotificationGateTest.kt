package tech.hybriq.adbuddy.listener

import android.content.SharedPreferences
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.FakePrefs
import tech.hybriq.adbuddy.listener.NotificationAccess.Listed

class NotificationGateTest {
    private val one = RawNotification("k", "com.whatsapp", "Ana", "hola", 0L, ongoing = false, secret = false)
    private var sourceCalls = 0
    private var revoked = 0

    private fun gate(confirmed: Boolean, listed: Listed?, source: List<RawNotification>? = listOf(one)) =
        NotificationGate.read({ confirmed }, { listed }, { revoked++ }, { sourceCalls++; source })

    @Test fun withoutTheUsersConfirmationItNeverReadsEvenIfTheSystemSaysAllowed() {
        assertEquals(NotificationRead.NotConfirmed, gate(confirmed = false, listed = Listed.YES))
        assertEquals(NotificationRead.NotConfirmed, gate(confirmed = false, listed = Listed.UNKNOWN))
        assertEquals(NotificationRead.NotConfirmed, gate(confirmed = false, listed = null))
        assertEquals("the listener is not even asked", 0, sourceCalls)
    }

    @Test fun confirmedAndAllowedReads() {
        assertEquals(NotificationRead.Ok(listOf(one)), gate(confirmed = true, listed = Listed.YES))
        assertEquals(1, sourceCalls)
    }

    @Test fun aRevokedAccessIsNoAccessAndInvalidatesTheMark() {
        assertEquals(NotificationRead.NoAccess, gate(confirmed = true, listed = Listed.NO))
        assertEquals(1, revoked)
        assertEquals(NotificationRead.NoAccess, gate(confirmed = false, listed = Listed.NO))
        assertEquals(0, sourceCalls)
    }

    @Test fun whatCannotBeToldIsNoAccessAndKeepsTheMark() {
        assertEquals(NotificationRead.NoAccess, gate(confirmed = true, listed = Listed.UNKNOWN))
        assertEquals(NotificationRead.NoAccess, gate(confirmed = true, listed = null))
        assertEquals(0, revoked)
        assertEquals(0, sourceCalls)
    }

    @Test fun anUnboundListenerIsNoAccess() {
        assertEquals(NotificationRead.NoAccess, gate(confirmed = true, listed = Listed.YES, source = null))
        assertEquals(0, revoked)
    }

    @Test fun theMarkPersistsAndRevokingTheAccessNeedsANewConfirmation() {
        val prefs = FakePrefs()
        assertFalse(NotificationConsent(prefs).confirmed())
        NotificationConsent(prefs).confirm() // the button of the screen
        val consent = NotificationConsent(prefs) // a new process reads it from the same preferences
        assertTrue(consent.confirmed())
        fun read(listed: Listed) = NotificationGate.read(consent::confirmed, { listed }, consent::revoke) { listOf(one) }
        assertEquals(NotificationRead.Ok(listOf(one)), read(Listed.YES))
        assertEquals(NotificationRead.NoAccess, read(Listed.NO)) // the user revokes in the system
        assertFalse(consent.confirmed())
        assertEquals("allowing again is not enough", NotificationRead.NotConfirmed, read(Listed.YES))
        consent.confirm()
        assertEquals(NotificationRead.Ok(listOf(one)), read(Listed.YES))
    }

    @Test fun unbindingTheListenerClearsTheMarkAtOnceWhoeverIsLooking() {
        val prefs = FakePrefs()
        val consent = NotificationConsent(prefs)
        consent.confirm()
        NotificationGate.onUnbound(consent::revoke) // the system revoked the access: no screen is open, no read is asked
        assertFalse(NotificationConsent(prefs).confirmed())
        // The agent turns the switch back on: still not enough.
        assertEquals(NotificationRead.NotConfirmed, NotificationGate.read(consent::confirmed, { Listed.YES }, consent::revoke) { listOf(one) })
        assertEquals("nothing was read", 0, sourceCalls)
        NotificationGate.onUnbound(consent::revoke) // already clear: nothing breaks
        assertFalse(consent.confirmed())
    }

    @Test fun aNewBindingOfTheListenerClearsTheMark() {
        // M5c-32 b: the system may bind the listener again without unbinding it first (a restart it had scheduled
        // before the access was revoked with the app dead). The confirmation belonged to the old binding.
        val prefs = FakePrefs()
        val consent = NotificationConsent(prefs)
        consent.confirm()
        NotificationGate.onConnected(consent::revoke)
        assertFalse(NotificationConsent(prefs).confirmed())
        assertEquals(NotificationRead.NotConfirmed, NotificationGate.read(consent::confirmed, { Listed.YES }, consent::revoke) { sourceCalls++; listOf(one) })
        assertEquals("nothing was read", 0, sourceCalls)
    }

    @Test fun aMarkRevokedWhileTheListenerIsReadDiscardsTheRead() {
        // The user revokes (the listener unbinds and clears the mark) between the checks and the end of the read.
        var mark = true
        val result = NotificationGate.read({ mark }, { Listed.YES }, { mark = false }) { mark = false; listOf(one) }
        assertEquals(NotificationRead.NotConfirmed, result)
        // The mark kept all along: the read stands.
        assertEquals(NotificationRead.Ok(listOf(one)), NotificationGate.read({ true }, { Listed.YES }, {}) { listOf(one) })
    }

    /** Preferences whose first commit() fails (returns false, writes nothing), as a real write can. */
    private class FlakyPrefs(private val inner: FakePrefs, var failures: Int) : SharedPreferences by inner {
        override fun edit(): SharedPreferences.Editor {
            val real = inner.edit()
            return object : SharedPreferences.Editor by real {
                override fun commit(): Boolean = if (failures-- > 0) false else real.commit()
                override fun apply() {} // asynchronous: nothing is written by the time the caller moves on
            }
        }
    }

    @Test fun revokeIsSynchronousAndRetriesOnceWhenTheWriteFails() {
        val inner = FakePrefs()
        NotificationConsent(inner).confirm()
        val once = FlakyPrefs(inner, failures = 1)
        NotificationConsent(once).revoke()
        assertFalse("the retry cleared it", NotificationConsent(inner).confirmed())
    }

    // --- The watch over who sets the mark -------------------------------------------------------------------
    // Accepted risk (ruling 15): a source scan cannot catch every spelling, so the Opus review reads the screen by hand;
    // it catches the plain ones and the real way round (writing the preference itself).

    private val confirmDecl = Regex("\\bfun\\s+confirm\\s*\\(")
    private val confirmCall = Regex("\\bconfirm\\s*\\(\\s*\\)|::confirm\\b")
    private val mayConfirm = setOf("NotificationAccessScreen.kt", "NotificationConsent.kt")

    /** A call to confirm() or a reference to it in [src], comments and strings left out, its declaration too. */
    private fun callsConfirm(src: String) = confirmCall.containsMatchIn(CodeScan.code(src).replace(confirmDecl, "fun x("))

    /** The files, by name, that set the mark when they should not: calling confirm(), or writing the preference itself. */
    private fun offenders(files: Map<String, String>): Set<String> = files.filter { (name, text) ->
        val writesDirectly = name != "NotificationConsent.kt" && (
            text.contains("\"notification_consent\"") ||
                // ProtectedApps keeps its own, unrelated "confirmed" key.
                (name != "ProtectedApps.kt" && text.contains("\"confirmed\"") && Regex("\\bedit\\b").containsMatchIn(text))
            )
        (name !in mayConfirm && callsConfirm(text)) || writesDirectly
    }.keys

    @Test fun nothingButTheButtonSetsTheMark() {
        val root = java.io.File("src/main/kotlin")
        assertTrue("no sources found, the scan would pass empty", root.isDirectory)
        val files = root.walkTopDown().filter { it.isFile && it.extension == "kt" }.associate { it.name to it.readText() }
        assertTrue("the scan found no sources", files.containsKey("NotificationConsent.kt"))
        assertEquals(emptySet<String>(), offenders(files))
    }

    @Test fun theMarkScannerSeesTheWaysToSetItAndNotTheLegitimateOnes() {
        val bad = listOf(
            "consent .confirm ()",
            "with(consent) { confirm() }",
            "val f = consent::confirm",
            "NotificationConsent.of(ctx).confirm()",
            "val p = ctx.getSharedPreferences(\"notification_consent\", 0)",
            "prefs.edit { putBoolean(\"confirmed\", true) }",
        )
        for (b in bad) assertEquals(b, setOf("Other.kt"), offenders(mapOf("Other.kt" to b)))
        // The brief's own: the declaration, the screen's button, and a comment or string that only says the word.
        val fine = mapOf(
            "NotificationConsent.kt" to "fun confirm() { prefs.edit { putBoolean(\"confirmed\", true) } }\nprivate const val PREFS = \"notification_consent\"",
            "NotificationAccessScreen.kt" to "Button(onClick = { consent.confirm() }) {}",
            "ProtectedApps.kt" to "fun confirm(list: Set<String>) {}\nprefs.edit { putBoolean(\"confirmed\", true) }",
            "Doc.kt" to "// the button calls consent.confirm()\nval s = \"consent.confirm()\"",
            "Other.kt" to "val c = consent.confirmed()\nprotectedApps.confirm(list)",
        )
        assertEquals(emptySet<String>(), offenders(fine))
    }
}
