package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.FakePrefs
import tech.hybriq.adbuddy.a11y.Packages
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.bridge.UiLimits
import tech.hybriq.adbuddy.ui.ProtectedAppsUi.Reason

class ProtectedAppsUiTest {
    private val messages = "com.google.android.apps.messaging"
    private val gmail = "com.google.android.gm"
    private val installed = listOf(
        "com.example.mybank" to "Mi Banco",
        messages to "Messages",
        gmail to "Gmail",
        "com.whatsapp" to "WhatsApp",
        "com.android.settings" to "Settings",
        Packages.ADBUDDY to "ADBuddy",
        "not a package" to "Bank",
    )
    private val receivers = ProtectedApps.Heuristic.receivers(null)

    private val gms = "com.google.android.gms"

    @Test fun messagesAndGmailAreProposedAlongsideTheHeuristic() {
        val start = ProtectedAppsUi.initialSelection(installed, emptySet(), confirmed = false, receivers = receivers)
        assertEquals(setOf("com.example.mybank", messages, gmail, gms), start)
    }

    @Test fun onlyTheCodeReceiversThatAreInstalledAreProposed() {
        // Google Play services has no icon in the launcher, so it is proposed always; the others only if installed.
        val start = ProtectedAppsUi.initialSelection(listOf("com.whatsapp" to "WhatsApp"), emptySet(), confirmed = false, receivers = receivers)
        assertEquals(setOf(gms), start)
    }

    @Test fun theScreenOpensWithWhatIsAlreadyProtectedSoTheCountsAgree() {
        // What UiHelpers.install proposes (and the host protects) is what the screen and Settings count.
        val p = ProtectedApps(FakePrefs())
        p.propose(installed, receivers)
        assertEquals(p.list(), ProtectedAppsUi.initialSelection(installed, p.list(), confirmed = false, receivers = receivers))
        assertEquals(4, p.list().size)
    }

    @Test fun theDefaultSmsAppIsAReceiverToo() {
        val r = ProtectedApps.Heuristic.receivers("com.vendor.sms")
        assertTrue("com.vendor.sms" in r)
        assertEquals(ProtectedApps.Heuristic.CODE_RECEIVERS, ProtectedApps.Heuristic.receivers(null))
        assertEquals(ProtectedApps.Heuristic.CODE_RECEIVERS, ProtectedApps.Heuristic.receivers("not a package"))
        val apps = listOf("com.vendor.sms" to "Vendor SMS")
        assertEquals(setOf("com.vendor.sms", gms), ProtectedAppsUi.initialSelection(apps, emptySet(), false, r))
    }

    @Test fun aConfirmedListIsNeverTouchedByTheProposals() {
        // The user unchecked Messages and Gmail on purpose: they do not come back.
        val saved = setOf("com.example.mybank")
        assertEquals(saved, ProtectedAppsUi.initialSelection(installed, saved, confirmed = true, receivers = receivers))
        assertEquals(emptySet<String>(), ProtectedAppsUi.initialSelection(installed, emptySet(), confirmed = true, receivers = receivers))
    }

    @Test fun adbuddyIsNeverInTheSelectionNorTheRows() {
        val start = ProtectedAppsUi.initialSelection(installed, setOf(Packages.ADBUDDY), confirmed = true, receivers = receivers)
        assertFalse(Packages.ADBUDDY in start)
        val rows = ProtectedAppsUi.rows(installed, start, start, receivers, "")
        assertFalse(rows.any { it.pkg == Packages.ADBUDDY || it.pkg == "not a package" })
        assertEquals(start, ProtectedAppsUi.toggle(start, Packages.ADBUDDY, true))
        assertEquals(emptySet<String>(), ProtectedAppsUi.result(setOf(Packages.ADBUDDY)))
    }

    @Test fun rowsPutTheProposedFirstAndKeepTheirPlaceWhenToggled() {
        val initial = ProtectedAppsUi.initialSelection(installed, emptySet(), false, receivers)
        val first = ProtectedAppsUi.rows(installed, initial, initial, receivers, "")
        // Play services has no launcher icon: it shows by package name, so it can be unchecked.
        assertEquals(listOf(gms, "Gmail", "Messages", "Mi Banco", "Settings", "WhatsApp"), first.map { it.label })
        assertEquals(listOf(Reason.CODES, Reason.CODES, Reason.CODES, Reason.GUESS, Reason.NONE, Reason.NONE), first.map { it.reason })
        assertEquals(listOf(true, true, true, true, false, false), first.map { it.checked })
        val after = ProtectedAppsUi.toggle(ProtectedAppsUi.toggle(initial, "com.whatsapp", true), gmail, false)
        val second = ProtectedAppsUi.rows(installed, initial, after, receivers, "")
        assertEquals(first.map { it.pkg }, second.map { it.pkg }) // same order
        assertEquals(listOf(true, false, true, true, false, true), second.map { it.checked })
    }

    @Test fun searchMatchesTheLabelAndThePackage() {
        val sel = emptySet<String>()
        assertEquals(listOf("WhatsApp"), ProtectedAppsUi.rows(installed, sel, sel, receivers, "  WHATS ").map { it.label })
        assertEquals(listOf("Settings"), ProtectedAppsUi.rows(installed, sel, sel, receivers, "android.settings").map { it.label })
        assertTrue(ProtectedAppsUi.rows(installed, sel, sel, receivers, "zzz").isEmpty())
    }

    @Test fun anAppSavedWithoutALauncherIconCanStillBeUnchecked() {
        val saved = setOf("com.hidden.service")
        val rows = ProtectedAppsUi.rows(installed, saved, saved, receivers, "")
        val hidden = rows.single { it.pkg == "com.hidden.service" }
        assertEquals("com.hidden.service", hidden.label)
        assertTrue(hidden.checked)
        val unchecked = ProtectedAppsUi.rows(installed, saved, ProtectedAppsUi.toggle(saved, "com.hidden.service", false), receivers, "")
        assertFalse(unchecked.single { it.pkg == "com.hidden.service" }.checked) // still listed, to check it again
    }

    @Test fun theSelectionStopsAtTheProtocolLimit() {
        val full = (0 until UiLimits.MAX_PROTECTED_APPS).map { "com.a.app$it" }.toSet()
        assertEquals(full, ProtectedAppsUi.toggle(full, "com.b.extra", true))
        assertEquals(full - "com.a.app0", ProtectedAppsUi.toggle(full, "com.a.app0", false))
        assertEquals(UiLimits.MAX_PROTECTED_APPS, ProtectedAppsUi.result(full + "com.b.extra").size)
        assertEquals(setOf("com.ok.app"), ProtectedAppsUi.result(setOf("com.ok.app", "bad", "")))
    }

    @Test fun theSaveButtonIsOnForTheFirstConfirmationAndForChanges() {
        assertFalse("still loading", ProtectedAppsUi.canSave(loaded = false, onboarding = true, dirty = true, confirmed = false))
        assertTrue("onboarding always confirms", ProtectedAppsUi.canSave(loaded = true, onboarding = true, dirty = false, confirmed = true))
        assertTrue("an unconfirmed proposal can be confirmed unchanged", ProtectedAppsUi.canSave(loaded = true, onboarding = false, dirty = false, confirmed = false))
        assertTrue(ProtectedAppsUi.canSave(loaded = true, onboarding = false, dirty = true, confirmed = true))
        assertFalse("nothing to save", ProtectedAppsUi.canSave(loaded = true, onboarding = false, dirty = false, confirmed = true))
    }

    @Test fun dirtyOnlyWhenTheSelectionDiffersFromWhatIsSaved() {
        val saved = setOf("com.a.app", "com.b.app")
        assertFalse(ProtectedAppsUi.dirty(saved, saved))
        assertFalse(ProtectedAppsUi.dirty(saved + Packages.ADBUDDY, saved))
        assertTrue(ProtectedAppsUi.dirty(saved - "com.a.app", saved))
        assertTrue(ProtectedAppsUi.dirty(saved + "com.c.app", saved))
    }
}
