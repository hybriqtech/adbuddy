package tech.hybriq.adbuddy;

import android.app.Activity;
import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.PixelFormat;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import android.util.Log;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;

import java.util.regex.Pattern;

/**
 * The obscuring window of host/internal/emutest/tapjack_test.go (TestAnObscuringOverlayRefusesTheTapsThatGrant): a
 * full-screen TYPE_APPLICATION_OVERLAY that lets touches through (FLAG_NOT_TOUCHABLE) and is faint enough (alpha
 * ALPHA, below the 0.8 of Android 12+'s untrusted-touch block) for the system to deliver them, marked as obscured,
 * to the window below: the tapjacking the app's ObscuredTouchGuard must refuse.
 *
 * Why an activity of the tests' APK and not an instrumented test: an instrumented test runs in the APP's process,
 * with the app's uid, and the input dispatcher never marks a touch as obscured by a window of the same uid. This
 * activity runs in the tests' own package, tech.hybriq.adbuddy.test: another process, another uid, like a real
 * overlay app. And `am instrument` force-stops the app when it starts and when it ends; `am start` of this
 * activity leaves the app alone.
 *
 * Why Java and not Kotlin: this package's APK does not carry the Kotlin stdlib (it lives in the app's APK, which is
 * not on this process's classpath), so Kotlin code here dies with ClassNotFoundException: kotlin.text.Regex.
 * Only the Android framework and java.* are used.
 *
 * The host starts it with `am start -n tech.hybriq.adbuddy.test/tech.hybriq.adbuddy.TapjackOverlayActivity
 * --es nonce <hex>`. It adds the window through the application's context (it outlives the activity: no parent window)
 * and finishes at once, so the app it covers comes back to the front. It writes `overlay shown nonce=<hex>` to
 * logcat (tag TAG) once the window has drawn. The host takes it away with
 * `am force-stop tech.hybriq.adbuddy.test`: the process dies and the window with it.
 *
 * Never on a phone: off an emulator (Build.HARDWARE ranchu or goldfish) it draws nothing and logs why.
 */
public class TapjackOverlayActivity extends Activity {
    private static final String TAG = "ADBuddyTapjack";
    private static final String EXTRA_NONCE = "nonce";
    /** The window's title, which `dumpsys window windows` shows: the host finds the window by it. */
    private static final String TITLE = "ADBuddyTapjackOverlay";
    /** Below InputManager's default maximum obscuring opacity (0.8): touches pass, marked as obscured. */
    private static final float ALPHA = 0.3f;
    private static final Pattern NONCE = Pattern.compile("^[0-9a-f]{16}$");

    /** The window on screen, for the life of the process. */
    private static View shown;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        String extra = getIntent() == null ? null : getIntent().getStringExtra(EXTRA_NONCE);
        String nonce = extra == null ? "" : extra;
        try {
            if (!NONCE.matcher(nonce).matches()) {
                Log.e(TAG, "refused: no nonce");
            } else if (!"ranchu".equals(Build.HARDWARE) && !"goldfish".equals(Build.HARDWARE)) {
                Log.e(TAG, "refused nonce=" + nonce + ": not an emulator (Build.HARDWARE=" + Build.HARDWARE + ")");
            } else if (!Settings.canDrawOverlays(this)) {
                Log.e(TAG, "refused nonce=" + nonce + ": SYSTEM_ALERT_WINDOW is not allowed (appops set "
                        + getPackageName() + " SYSTEM_ALERT_WINDOW allow)");
            } else if (shown != null) {
                Log.i(TAG, "overlay shown nonce=" + nonce + " (already)");
            } else {
                show(getApplicationContext(), nonce);
            }
        } catch (RuntimeException e) {
            Log.e(TAG, "refused nonce=" + nonce + ": the window could not be added", e);
        } finally {
            finish();
        }
    }

    private static void show(Context context, final String nonce) {
        View view = new View(context) {
            private boolean drawn = false;

            @Override
            protected void onDraw(Canvas canvas) {
                super.onDraw(canvas);
                if (!drawn) {
                    drawn = true;
                    Log.i(TAG, "overlay shown nonce=" + nonce);
                }
            }
        };
        view.setWillNotDraw(false);
        // Visible to a person looking at the emulator or a screenshot, faint by the window's alpha.
        view.setBackgroundColor(Color.MAGENTA);
        WindowManager.LayoutParams lp = new WindowManager.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT,
                WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
                WindowManager.LayoutParams.FLAG_NOT_TOUCHABLE
                        | WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE
                        | WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN,
                PixelFormat.TRANSLUCENT);
        lp.alpha = ALPHA;
        lp.setTitle(TITLE);
        // The whole screen, system bars and cutout included: every button of the app is under it.
        lp.setFitInsetsTypes(0);
        lp.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_ALWAYS;
        context.getSystemService(WindowManager.class).addView(view, lp);
        shown = view;
    }
}
