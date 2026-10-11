package tech.hybriq.adbuddy

import android.app.Instrumentation
import android.app.UiAutomation
import android.content.Intent
import android.os.Build
import android.os.SystemClock
import android.provider.Settings
import android.util.Log
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.FileInputStream
import java.time.Instant
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import tech.hybriq.adbuddy.a11y.AgentAccessibilityService
import tech.hybriq.adbuddy.a11y.UiExecutor
import tech.hybriq.adbuddy.a11y.UiNode
import tech.hybriq.adbuddy.bridge.UiOp
import tech.hybriq.adbuddy.bridge.UiRequest
import tech.hybriq.adbuddy.bridge.UiResponse

/**
 * P5 task 1 (docs/superpowers/plans/2026-10-11-latencia-p5-foreground-ligero.md): how long the app's `foreground`
 * (UiExecutor.run(FOREGROUND), as the host's guard asks for it) and a `windows()` on their own take inside the app,
 * with the real accessibility service, over a window of about 2000 nested views, one of more than 8000 (the cap
 * MAX_NODES of the service) and Settings. Timed with System.nanoTime() around the calls: from the host the 120-165 ms
 * of transport would drown it. The figures go to the log (tag ADBPERF) and the test asserts only that they are
 * valid (the service bound, the window was the one measured, the answer ok and not protected).
 *
 * It enables the service through the shell and restores both settings in @After. It FAILS, it does not skip, if the
 * service does not bind. It only runs with `-e adbuddyPerf true` (the whole suite, TestInstrumentedSuite, skips it)
 * and only on an emulator.
 */
@RunWith(AndroidJUnit4::class)
class ForegroundHeadsOnEmulatorTest {
    private val instr: Instrumentation = InstrumentationRegistry.getInstrumentation()
    private lateinit var automation: UiAutomation
    private var savedServices: String? = null
    private var savedEnabled: String? = null
    private var armed = false

    @Before fun enableService() {
        assumeTrue("only with -e adbuddyPerf true", InstrumentationRegistry.getArguments().getString("adbuddyPerf") == "true")
        // Never on a phone: the emulator's hardware is ranchu (goldfish before it).
        assertTrue("not an emulator (Build.HARDWARE=${Build.HARDWARE})", Build.HARDWARE == "ranchu" || Build.HARDWARE == "goldfish")
        // Without this flag the system suppresses the other accessibility services while UiAutomation is connected.
        automation = instr.getUiAutomation(UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES)
        savedServices = shell("settings get secure enabled_accessibility_services").trim()
        savedEnabled = shell("settings get secure accessibility_enabled").trim()
        armed = true
        shell("settings put secure enabled_accessibility_services $SERVICE")
        shell("settings put secure accessibility_enabled 1")
        shell("wm dismiss-keyguard")
        shell("input keyevent KEYCODE_WAKEUP")
        val deadline = SystemClock.uptimeMillis() + 30_000
        while (AgentAccessibilityService.current == null && SystemClock.uptimeMillis() < deadline) Thread.sleep(200)
        assertNotNull("the accessibility service did not bind in 30 s", AgentAccessibilityService.current)
    }

    @After fun restore() {
        if (!armed) return
        restoreSetting("enabled_accessibility_services", savedServices)
        restoreSetting("accessibility_enabled", savedEnabled)
    }

    @Test fun foregroundAgainstWindows() {
        val service = AgentAccessibilityService.current!!
        for ((name, launch) in listOf<Pair<String, () -> Unit>>(
            "tree~2000" to { startTree(2000) },
            "tree>8000" to { startTree(9000) },
            "settings" to { startSettings() },
        )) {
            launch()
            // Wait for the window to be foreground, with its tree published.
            val deadline = SystemClock.uptimeMillis() + 20_000
            while (SystemClock.uptimeMillis() < deadline && service.foreground().first.let { it.isEmpty() || it == APP }) Thread.sleep(200)
            Thread.sleep(2_500)
            val fg = service.foreground().first
            assertTrue("$name: foreground is '$fg'", fg.isNotEmpty() && fg != APP)
            val nodes = service.windows().sumOf { w -> w.root?.let(::count) ?: 0 }
            // isProtected = false: the guard must not short-circuit, so windows() is read as in the normal case.
            val executor = UiExecutor(
                screen = { service }, send = {}, now = System::currentTimeMillis,
                isProtected = { false }, questionWaiting = { false }, paused = { false },
                notificationOnScreen = { false }, executor = { it.run() },
            )
            var seq = 0
            fun foregroundCall(): Long {
                val r = UiRequest("perf-${seq++}", UiOp.FOREGROUND, expiresAt = Instant.now().plusSeconds(3600).toString())
                val t0 = System.nanoTime()
                val resp: UiResponse = executor.run(r)
                val dt = System.nanoTime() - t0
                assertTrue("$name: foreground answered ${resp.error}", resp.ok)
                assertFalse("$name: the answer says protected: $resp", resp.result.toString().contains("\"protected\":true"))
                return dt
            }
            fun windowsCall(): Long {
                val t0 = System.nanoTime()
                service.windows()
                return System.nanoTime() - t0
            }
            repeat(3) { foregroundCall(); windowsCall() }
            // Interleaved, so neither one always runs after the other (the target process's own caches).
            val fgTimes = ArrayList<Long>()
            val winTimes = ArrayList<Long>()
            repeat(30) { fgTimes += foregroundCall(); winTimes += windowsCall() }
            Log.i(
                TAG,
                "$name fg=$fg nodes=$nodes foreground median=${ms(median(fgTimes))} p90=${ms(p90(fgTimes))} ms; " +
                    "windows() median=${ms(median(winTimes))} p90=${ms(p90(winTimes))} ms",
            )
        }
        shell("input keyevent KEYCODE_HOME")
    }

    private fun startTree(views: Int) {
        // am start, as TapjackOverlayActivity: the activity lives in the tests' own process, which the
        // instrumentation's startActivitySync refuses.
        val out = shell("am start -S -W -n ${instr.context.packageName}/tech.hybriq.adbuddy.BigTreeActivity --ei views $views")
        assertTrue("BigTreeActivity did not start: $out", out.contains("Status: ok"))
    }

    private fun startSettings() {
        instr.context.startActivity(Intent(Settings.ACTION_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK))
    }

    private fun count(n: UiNode): Int = 1 + n.children.sumOf(::count)

    private fun median(v: List<Long>) = v.sorted()[v.size / 2]
    private fun p90(v: List<Long>) = v.sorted()[(v.size * 9) / 10 - 1]
    private fun ms(ns: Long) = "%.1f".format(ns / 1e6)

    private fun restoreSetting(key: String, saved: String?) {
        if (saved == null || saved == "null" || saved.isEmpty()) shell("settings delete secure $key")
        else shell("settings put secure $key $saved")
    }

    private fun shell(cmd: String): String {
        val pfd = automation.executeShellCommand(cmd)
        return FileInputStream(pfd.fileDescriptor).use { it.readBytes().toString(Charsets.UTF_8) }
    }

    private companion object {
        const val TAG = "ADBPERF"
        const val APP = "tech.hybriq.adbuddy"
        const val SERVICE = "tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService"
    }
}
