package tech.hybriq.adbuddy

import android.Manifest
import android.database.ContentObserver
import android.os.Handler
import android.os.Looper
import android.os.ParcelFileDescriptor
import android.provider.Settings
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import tech.hybriq.adbuddy.control.AccessSwitch

/**
 * Validation 10 of M2 (M2.5 spec §7): an observer of the app on the hidden setting adb_wifi_enabled, registered
 * as CutAccess.observe registers its own, hears a change made by someone else (adb, the Settings app). The
 * change never turns wireless debugging on: the setting goes from "0" (or unset) to "00", which everyone reads as
 * 0 (Settings.Global.getInt, AccessSwitch, the system's own observer), and is put back afterwards.
 */
@RunWith(AndroidJUnit4::class)
class AdbWifiObserverTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val resolver = instrumentation.targetContext.contentResolver
    private val name = AccessSwitch.ADB_WIFI_ENABLED
    private var original = ""

    /** A command as the shell user (uid 2000), in a process of its own, as adb runs it. */
    private fun shell(command: String): String {
        val pfd = instrumentation.uiAutomation.executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(pfd).use { it.readBytes().decodeToString() }.trim()
    }

    @Before fun wirelessDebuggingIsOff() {
        original = shell("settings get global $name")
        // The test rewrites the setting: never while wireless debugging is on.
        assumeTrue("wireless debugging is on in this emulator ($name=$original)", original == "null" || original == "0")
    }

    @After fun putTheSettingBack() {
        if (original == "null") shell("settings delete global $name") else shell("settings put global $name $original")
        assertEquals(original, shell("settings get global $name"))
    }

    /** Whether an observer like CutAccess's heard a change of the setting within 10 s of change(). */
    private fun heardAfter(change: () -> Unit): Boolean {
        val heard = CountDownLatch(1)
        val observer = object : ContentObserver(Handler(Looper.getMainLooper())) {
            override fun onChange(selfChange: Boolean) {
                heard.countDown()
            }
        }
        resolver.registerContentObserver(Settings.Global.getUriFor(name), false, observer)
        try {
            change()
            return heard.await(10, TimeUnit.SECONDS)
        } finally {
            resolver.unregisterContentObserver(observer)
        }
    }

    @Test fun aChangeFromTheShellReachesTheObserver() {
        assertTrue("the app did not hear $name change from the shell", heardAfter { shell("settings put global $name 00") })
    }

    @Test fun aChangeWithTheShellPermissionIdentityReachesTheObserver() {
        val ui = instrumentation.uiAutomation
        assertTrue("the app did not hear $name change with the shell's permissions", heardAfter {
            ui.adoptShellPermissionIdentity(Manifest.permission.WRITE_SECURE_SETTINGS)
            try {
                Settings.Global.putString(resolver, name, "00")
            } finally {
                ui.dropShellPermissionIdentity()
            }
        })
    }

    /** CutAccess saves adb_wifi_enabled before a cut so that Reactivar puts it back: the app must be able to read it. */
    @Test fun theAppCanReadAdbWifiEnabled() {
        shell("settings put global $name 0")
        assertEquals("0", Settings.Global.getString(resolver, name))
    }
}
