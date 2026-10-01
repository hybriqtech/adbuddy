package tech.hybriq.adbuddy.a11y

import android.accessibilityservice.AccessibilityService
import android.content.res.Configuration
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.PixelFormat
import android.graphics.drawable.GradientDrawable
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.util.Log
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.WindowManager
import android.widget.TextView
import java.util.concurrent.FutureTask
import java.util.concurrent.TimeUnit
import tech.hybriq.adbuddy.R

/**
 * The overlay of M5 spec §6: a thin frame round the screen, a pill "Agent
 * using the screen · Pause" and a fading mark where the agent touches. Only
 * the pill takes touches; it hides 4 s after the agent's last action. Its
 * windows are ADBuddy's, so the helpers never show nor touch them.
 */
class Overlay(private val service: AccessibilityService, private val onPause: () -> Unit) {
    private val main = Handler(Looper.getMainLooper())
    private val wm = service.getSystemService(WindowManager::class.java)
    @Volatile private var frame: FrameView? = null
    @Volatile private var pill: TextView? = null
    private val hider = Runnable { remove() }

    /**
     * Shows the overlay (from any thread but the main one), with a mark when
     * there is one, and waits for it. false when the frame and the pill could
     * not be put on screen: the agent must not act without them.
     */
    fun show(mark: Box?): Boolean {
        val task = FutureTask { showNow(mark) }
        main.post(task)
        return try {
            task.get(SHOW_WAIT_MS, TimeUnit.MILLISECONDS)
        } catch (e: Exception) {
            Log.e(TAG, "overlay did not show", e)
            false
        }
    }

    private fun showNow(mark: Box?): Boolean {
        if (frame == null) {
            try {
                add()
            } catch (e: RuntimeException) {
                Log.e(TAG, "overlay", e)
                return false
            }
        }
        mark?.let { frame?.mark(it) }
        main.removeCallbacks(hider)
        main.postDelayed(hider, HIDE_AFTER_MS)
        return true
    }

    /** Hides the overlay (from any thread). */
    fun hide() {
        main.post { remove() }
    }

    private fun remove() {
        main.removeCallbacks(hider)
        frame?.let { runCatching { wm.removeView(it) } }
        pill?.let { runCatching { wm.removeView(it) } }
        frame = null
        pill = null
    }

    private fun dp(v: Float) = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v, service.resources.displayMetrics)

    private fun accent(): Int {
        val dark = service.resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK == Configuration.UI_MODE_NIGHT_YES
        return service.getColor(if (dark) android.R.color.system_accent1_200 else android.R.color.system_accent1_600)
    }

    private fun add() {
        val color = accent()
        val f = FrameView(color, dp(3f), dp(24f))
        val frameParams =
            WindowManager.LayoutParams(
                WindowManager.LayoutParams.MATCH_PARENT, WindowManager.LayoutParams.MATCH_PARENT,
                WindowManager.LayoutParams.TYPE_ACCESSIBILITY_OVERLAY,
                WindowManager.LayoutParams.FLAG_NOT_TOUCHABLE or WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                    WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN or WindowManager.LayoutParams.FLAG_LAYOUT_NO_LIMITS,
                PixelFormat.TRANSLUCENT,
            )
        val p = TextView(service).apply {
            text = service.getString(R.string.overlay_pill)
            setTextColor(Color.WHITE)
            textSize = 14f
            val pad = dp(12f).toInt()
            setPadding(pad, pad / 2, pad, pad / 2)
            background = GradientDrawable().apply {
                cornerRadius = dp(20f)
                setColor(color)
            }
            contentDescription = service.getString(R.string.overlay_pill)
            setOnClickListener {
                onPause()
                remove()
            }
        }
        val pillParams =
            WindowManager.LayoutParams(
                WindowManager.LayoutParams.WRAP_CONTENT, WindowManager.LayoutParams.WRAP_CONTENT,
                WindowManager.LayoutParams.TYPE_ACCESSIBILITY_OVERLAY,
                WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE,
                PixelFormat.TRANSLUCENT,
            ).apply {
                gravity = Gravity.TOP or Gravity.CENTER_HORIZONTAL
                y = dp(48f).toInt()
            }
        // If the pill cannot be added, the frame is taken off again: no orphan frame.
        OverlayWindows.addBoth(
            first = { f.also { wm.addView(it, frameParams) } },
            second = { p.also { wm.addView(it, pillParams) } },
            remove = { runCatching { wm.removeView(it) } },
        )
        frame = f
        pill = p
    }

    /** The frame and the marks; it takes no touches. */
    private inner class FrameView(color: Int, private val stroke: Float, private val radius: Float) : View(service) {
        private val border = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeWidth = stroke
            this.color = color
        }
        private val dot = Paint(Paint.ANTI_ALIAS_FLAG).apply { this.color = color }
        private var at: Box? = null
        private var since = 0L
        private val loc = IntArray(2)

        fun mark(b: Box) {
            at = b
            since = SystemClock.uptimeMillis()
            invalidate()
        }

        override fun onDraw(canvas: Canvas) {
            canvas.drawRect(stroke / 2, stroke / 2, width - stroke / 2, height - stroke / 2, border)
            val b = at ?: return
            val age = SystemClock.uptimeMillis() - since
            if (age > MARK_MS) {
                at = null
                return
            }
            getLocationOnScreen(loc)
            dot.alpha = (255 * (1 - age / MARK_MS.toFloat())).toInt().coerceIn(0, 255)
            canvas.drawCircle((b.centerX - loc[0]).toFloat(), (b.centerY - loc[1]).toFloat(), radius, dot)
            postInvalidateOnAnimation()
        }
    }

    private companion object {
        const val TAG = "ADBuddyOverlay"
        const val HIDE_AFTER_MS = 4_000L
        const val SHOW_WAIT_MS = 2_000L
        const val MARK_MS = 600L
    }
}

/** Adding the overlay's two windows as one: all or none. */
internal object OverlayWindows {
    /**
     * Adds first, then second; if second fails, first is removed (it would stay
     * on screen with nobody holding it) and the failure goes on.
     */
    fun <V : Any> addBoth(first: () -> V, second: () -> V, remove: (V) -> Unit): Pair<V, V> {
        val a = first()
        try {
            return a to second()
        } catch (e: RuntimeException) {
            remove(a)
            throw e
        }
    }
}
