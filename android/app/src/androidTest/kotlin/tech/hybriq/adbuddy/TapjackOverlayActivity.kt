package tech.hybriq.adbuddy

import android.app.Activity
import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.PixelFormat
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.util.Log
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager

/**
 * The obscuring window of host/internal/emutest/tapjack_test.go (TestAnObscuringOverlayRefusesTheTapsThatGrant): a
 * full-screen TYPE_APPLICATION_OVERLAY that lets touches through (FLAG_NOT_TOUCHABLE) and is faint enough (alpha
 * [ALPHA], below the 0.8 of Android 12+'s untrusted-touch block) for the system to deliver them, marked as obscured,
 * to the window below: the tapjacking the app's ObscuredTouchGuard must refuse.
 *
 * Why an activity of the tests' APK and not an instrumented test: an instrumented test runs in the APP's process,
 * with the app's uid, and the input dispatcher never marks a touch as obscured by a window of the same uid (there is no
 * security boundary to protect). This activity runs in the tests' own package, tech.hybriq.adbuddy.test: another
 * process, another uid, like a real overlay app. And `am instrument` force-stops the app when it starts and when it
 * ends, which would close the screen under test; `am start` of this activity leaves the app alone.
 *
 * The host starts it with `am start -n tech.hybriq.adbuddy.test/tech.hybriq.adbuddy.TapjackOverlayActivity
 * --es nonce <hex>`. It adds the window through the application's context (it outlives the activity: no parent window)
 * and finishes at once, so the app it covers comes back to the front; a process with a visible overlay is perceptible
 * and is not reclaimed. It writes `overlay shown nonce=<hex>` to logcat (tag [TAG]) once the window has drawn. The
 * host takes it away with `am force-stop tech.hybriq.adbuddy.test`: the process dies and the window with it.
 *
 * Never on a phone: off an emulator (Build.HARDWARE ranchu or goldfish) it draws nothing and logs why.
 */
class TapjackOverlayActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val nonce = intent?.getStringExtra(EXTRA_NONCE).orEmpty()
        try {
            when {
                !NONCE.matches(nonce) -> Log.e(TAG, "refused: no nonce")
                Build.HARDWARE != "ranchu" && Build.HARDWARE != "goldfish" ->
                    Log.e(TAG, "refused nonce=$nonce: not an emulator (Build.HARDWARE=${Build.HARDWARE})")
                !Settings.canDrawOverlays(this) ->
                    Log.e(TAG, "refused nonce=$nonce: SYSTEM_ALERT_WINDOW is not allowed (appops set $packageName SYSTEM_ALERT_WINDOW allow)")
                shown != null -> Log.i(TAG, "overlay shown nonce=$nonce (already)")
                else -> show(applicationContext, nonce)
            }
        } catch (e: RuntimeException) {
            Log.e(TAG, "refused nonce=$nonce: the window could not be added", e)
        } finally {
            finish()
        }
    }

    private fun show(context: Context, nonce: String) {
        val view = object : View(context) {
            private var drawn = false

            override fun onDraw(canvas: Canvas) {
                super.onDraw(canvas)
                if (!drawn) {
                    drawn = true
                    Log.i(TAG, "overlay shown nonce=$nonce")
                }
            }
        }
        view.setWillNotDraw(false)
        // Visible to a person looking at the emulator or a screenshot, faint by the window's alpha.
        view.setBackgroundColor(Color.MAGENTA)
        val lp = WindowManager.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT,
            ViewGroup.LayoutParams.MATCH_PARENT,
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
            WindowManager.LayoutParams.FLAG_NOT_TOUCHABLE or
                WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN,
            PixelFormat.TRANSLUCENT,
        ).apply {
            alpha = ALPHA
            title = TITLE
            // The whole screen, system bars and cutout included: every button of the app is under it.
            fitInsetsTypes = 0
            layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS
        }
        context.getSystemService(WindowManager::class.java).addView(view, lp)
        shown = view
    }

    private companion object {
        const val TAG = "ADBuddyTapjack"
        const val EXTRA_NONCE = "nonce"
        /** The window's title, which `dumpsys window windows` shows: the host finds the window by it. */
        const val TITLE = "ADBuddyTapjackOverlay"
        /** Below InputManager's default maximum obscuring opacity (0.8): touches pass, marked as obscured. */
        const val ALPHA = 0.3f
        val NONCE = Regex("^[0-9a-f]{16}$")

        /** The window on screen, for the life of the process. */
        var shown: View? = null
    }
}
