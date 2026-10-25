package tech.hybriq.adbuddy.control

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.shadows.ShadowLog
import tech.hybriq.adbuddy.AdbuddyApp

/** R22: the app owns the check after a cut; a second tap replaces the one still running instead of racing it. */
@RunWith(RobolectricTestRunner::class)
class AdbuddyAppCutCheckTest {
    private val app get() = RuntimeEnvironment.getApplication() as AdbuddyApp

    @After fun realProbe() {
        CutAccess.probe = CutConfirm::adbdListening
        CutAccess.waitMs = CutConfirm.WAIT_MS
    }

    private fun checksFinished() = ShadowLog.getLogs().count { it.tag == AccessDiag.TAG && it.msg.contains("cut.check=") }

    @Test fun twoTapsInARowPublishASingleResult() {
        CutAccess.probe = { true }
        CutAccess.waitMs = 400
        ShadowLog.clear()
        app.confirmCut(false)
        app.confirmCut(false)
        val end = System.currentTimeMillis() + 5_000
        while (app.cutCheck.value == null && System.currentTimeMillis() < end) Thread.sleep(20)
        assertEquals(CutCheck.STILL_CONNECTED, app.cutCheck.value)
        Thread.sleep(1_000) // a check that was not cancelled would finish and log here too
        assertEquals("the first check was cancelled by the second", 1, checksFinished())
        assertNotNull(app.cutCheck.value)
    }

    @Test fun givingAccessBackCancelsTheCheckInCourse() {
        CutAccess.probe = { true }
        CutAccess.waitMs = 400
        ShadowLog.clear()
        app.confirmCut(false)
        app.cutCheckShown()
        Thread.sleep(1_500)
        assertEquals(null, app.cutCheck.value)
        assertEquals(0, checksFinished())
    }
}
