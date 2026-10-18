package tech.hybriq.adbuddy.notify

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import tech.hybriq.adbuddy.R

@RunWith(RobolectricTestRunner::class)
class AppIconsTest {
    @Test fun theAppHasItsOwnIcon() {
        val app = RuntimeEnvironment.getApplication()
        assertEquals(R.mipmap.ic_launcher, app.applicationInfo.icon)
    }

    @Test fun noNotificationUsesAFrameworkIcon() {
        val dir = File(System.getProperty("adbuddy.kotlinDir"))
        val offenders = dir.walkTopDown().filter { it.isFile && it.extension == "kt" && it.readText().contains("android.R.drawable.") }.map { it.name }.toList()
        assertTrue("framework drawables in: $offenders", offenders.isEmpty())
    }

    @Test fun theNotificationIconsExist() {
        val res = RuntimeEnvironment.getApplication().resources
        for (id in listOf(R.drawable.ic_stat_adbuddy, R.drawable.ic_stat_shield, R.drawable.ic_shield, R.drawable.ic_shield_off)) assertTrue(res.getDrawable(id, null) != null)
    }
}
