package tech.hybriq.adbuddy.a11y

import android.accessibilityservice.AccessibilityService
import android.content.res.Configuration
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.PixelFormat
import android.graphics.drawable.GradientDrawable
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.util.Log
import android.util.TypedValue
import android.view.Choreographer
import android.view.Gravity
import android.view.View
import android.view.WindowManager
import android.widget.TextView
import java.util.concurrent.CompletableFuture
import java.util.concurrent.FutureTask
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import tech.hybriq.adbuddy.R

/**
 * The overlay of M5 spec §6: a thin frame round the screen, a pill "Agent
 * using the screen · Pause" and a fading mark where the agent touches. Only
 * the pill takes touches; it hides 4 s after the agent's last action. Its
 * windows are ADBuddy's, so the helpers never show nor touch them. For a
 * capture by adb the host asks it to go off the screen for a moment
 * (hideFor); it comes back by itself, never while the agent is paused.
 */
class Overlay(
    private val service: AccessibilityService,
    private val isPaused: () -> Boolean,
    private val onPause: () -> Unit,
) {
    private val main = Handler(Looper.getMainLooper())
    private val wm = service.getSystemService(WindowManager::class.java)
    @Volatile private var frame: FrameView? = null
    @Volatile private var pill: TextView? = null
    @Volatile private var shownAt = 0L
    private val hider = Runnable { remove() }

    /** Puts the overlay back after hideFor's time, unless the agent was paused meanwhile. */
    private val restorer = Runnable { if (!isPaused()) showNow(null) }

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
        main.removeCallbacks(restorer)
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
        main.removeCallbacks(restorer)
        removeViews()
    }

    private fun removeViews() {
        frame?.let { runCatching { wm.removeView(it) } }
        pill?.let { runCatching { wm.removeView(it) } }
        frame = null
        pill = null
    }

    /**
     * Takes the frame and the pill off the screen for a capture by adb (from
     * any thread but the main one) and waits until the screen has been drawn
     * without them. They have been on the screen for MIN_SHOWN_MS at least
     * first, so that the person can see the notice before the capture. They
     * come back by themselves ms after they went, unless the agent is paused
     * then; show() and remove() cancel that. true when they are off the
     * screen or were not on it; false when they could not be taken off (they
     * are put back at once).
     */
    fun hideFor(ms: Long): Boolean {
        val done = CompletableFuture<Boolean>()
        val abandoned = AtomicBoolean(false)
        main.post {
            val dwell = (MIN_SHOWN_MS - (SystemClock.uptimeMillis() - shownAt)).coerceAtLeast(0)
            main.postDelayed({ if (!abandoned.get()) takeOff(ms, done) }, dwell)
        }
        return try {
            done.get(HIDE_WAIT_MS, TimeUnit.MILLISECONDS)
        } catch (e: Exception) {
            Log.e(TAG, "overlay did not hide in time", e)
            abandoned.set(true)
            main.post(restorer)
            false
        }
    }

    /** hideFor's work, on the main thread. */
    private fun takeOff(ms: Long, done: CompletableFuture<Boolean>) {
        try {
            if (frame == null) { // nothing on the screen: nothing to hide
                done.complete(true)
                return
            }
            main.removeCallbacks(hider)
            main.removeCallbacks(restorer)
            removeViews()
            main.postDelayed(restorer, ms)
            val removedAt = SystemClock.uptimeMillis()
            afterFrames(FRAMES_AFTER_HIDE) {
                // A fast display draws three frames in 25 ms: wait a minimum too.
                val rest = MIN_GONE_MS - (SystemClock.uptimeMillis() - removedAt)
                if (rest > 0) main.postDelayed({ done.complete(true) }, rest) else done.complete(true)
            }
        } catch (e: RuntimeException) {
            Log.e(TAG, "overlay did not hide", e)
            restorer.run()
            done.complete(false)
        }
    }

    /** Calls done on the main thread once n frames have been drawn. */
    private fun afterFrames(n: Int, done: () -> Unit) {
        val choreographer = Choreographer.getInstance()
        var left = n
        choreographer.postFrameCallback(object : Choreographer.FrameCallback {
            override fun doFrame(frameTimeNanos: Long) {
                if (--left <= 0) done() else choreographer.postFrameCallback(this)
            }
        })
    }

    private fun dp(v: Float) = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v, service.resources.displayMetrics)

    private fun colors(): OverlayColors.Pair {
        val dark = service.resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK == Configuration.UI_MODE_NIGHT_YES
        return OverlayColors.of(dark)
    }

    private fun add() {
        val colors = colors()
        val f = FrameView(colors.background, dp(3f), dp(24f))
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
            setTextColor(colors.text)
            textSize = 14f
            val pad = dp(12f).toInt()
            setPadding(pad, pad / 2, pad, pad / 2)
            background = GradientDrawable().apply {
                cornerRadius = dp(20f)
                setColor(colors.background)
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
        shownAt = SystemClock.uptimeMillis()
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

        /** The overlay is on the screen this long at least before it is taken off for a capture: the person sees the notice first (ruling R10). */
        const val MIN_SHOWN_MS = 400L

        /** At least this long off the screen before hideFor answers, whatever the display's rate. */
        const val MIN_GONE_MS = 50L

        /** hideFor gives up after this long (the dwell, three frames and the minimum fit well inside). */
        const val HIDE_WAIT_MS = 2_000L
        const val FRAMES_AFTER_HIDE = 3

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
