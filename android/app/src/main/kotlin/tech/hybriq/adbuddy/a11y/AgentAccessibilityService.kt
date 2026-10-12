package tech.hybriq.adbuddy.a11y

import android.accessibilityservice.AccessibilityService
import android.accessibilityservice.GestureDescription
import android.app.KeyguardManager
import android.graphics.Path
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.util.Log
import android.view.Display
import android.view.Surface
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
import android.view.accessibility.AccessibilityWindowInfo
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import tech.hybriq.adbuddy.AdbuddyApp

/**
 * ADBuddy's accessibility service (M5 spec §2, §6, §10): it reads the screen
 * and acts on it for the agent's helpers (Screen, run by UiExecutor) and draws
 * the overlay that shows the agent is using the screen. It does not override
 * dump(): `dumpsys activity service` is 🟢 for the agent.
 */
class AgentAccessibilityService : AccessibilityService(), Screen {
    @Volatile private var foreground: Pair<String, String> = "" to ""
    private var overlay: Overlay? = null

    override fun onServiceConnected() {
        Log.i(TAG, "connected")
        overlay = Overlay(this, isPaused = { (application as AdbuddyApp).bridge.state.value.sessionState.paused }) {
            (application as AdbuddyApp).bridge.pause(true)
        }
        current = this
    }

    override fun onUnbind(intent: android.content.Intent?): Boolean {
        current = null
        overlay?.hide()
        return super.onUnbind(intent)
    }

    override fun onDestroy() {
        current = null
        overlay?.hide()
        super.onDestroy()
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        if (event?.eventType == AccessibilityEvent.TYPE_NOTIFICATION_STATE_CHANGED) {
            notifications.accept(event.className, event.packageName?.toString().orEmpty())
            return
        }
        if (event?.eventType != AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED) return
        val pkg = event.packageName?.toString() ?: return
        val cls = event.className?.toString().orEmpty()
        // Only an application window that is in front: not a toast, the
        // keyboard or the overlay.
        val app = windows.firstOrNull { it.type == AccessibilityWindowInfo.TYPE_APPLICATION && it.isActive }
        if (app?.root?.packageName?.toString() == pkg) foreground = pkg to (if (cls.contains('.') && !cls.startsWith("android.")) cls else "")
    }

    override fun onInterrupt() = Unit

    // Screen

    override fun windows(): List<UiWindow> {
        var budget = MAX_NODES
        fun copy(n: AccessibilityNodeInfo, depth: Int): UiNode {
            budget--
            val r = Rect().also { n.getBoundsInScreen(it) }
            val children = if (depth >= Dumper.MAX_DEPTH) {
                emptyList()
            } else {
                (0 until n.childCount).mapNotNull { i -> if (budget <= 0) null else n.getChild(i)?.let { copy(it, depth + 1) } }
            }
            return UiNode(
                pkg = n.packageName?.toString().orEmpty(),
                cls = n.className?.toString().orEmpty(),
                id = n.viewIdResourceName.orEmpty(),
                text = n.text?.toString().orEmpty(),
                desc = n.contentDescription?.toString().orEmpty(),
                hint = n.hintText?.toString().orEmpty().takeUnless { it == n.text?.toString() }.orEmpty(),
                bounds = Box(r.left, r.top, r.right, r.bottom),
                visible = n.isVisibleToUser,
                clickable = n.isClickable || n.isLongClickable,
                editable = n.isEditable,
                scrollable = n.isScrollable,
                checkable = n.isCheckable,
                checked = n.isChecked,
                enabled = n.isEnabled,
                focused = n.isFocused,
                password = n.isPassword,
                children = children,
                ref = n,
            )
        }
        // A window without content (an app that hides it from accessibility
        // services) stays, with no package: nothing taps it and it counts as
        // protected (UiExecutor).
        return windows.map { w ->
            val root = w.root
            val r = Rect().also { w.getBoundsInScreen(it) }
            val type = when (w.type) {
                AccessibilityWindowInfo.TYPE_APPLICATION -> if (w.isActive || w.isFocused) "app" else "dialog"
                AccessibilityWindowInfo.TYPE_INPUT_METHOD -> "ime"
                AccessibilityWindowInfo.TYPE_SPLIT_SCREEN_DIVIDER -> "split"
                else -> "system"
            }
            UiWindow(type, w.layer, Box(r.left, r.top, r.right, r.bottom), root?.let { copy(it, 0) })
        }
    }

    override fun screen(): Box {
        val b = getSystemService(android.view.WindowManager::class.java).maximumWindowMetrics.bounds
        return Box(0, 0, b.width(), b.height())
    }

    override fun rotation(): Int = when (getSystemService(android.hardware.display.DisplayManager::class.java).getDisplay(Display.DEFAULT_DISPLAY)?.rotation) {
        Surface.ROTATION_90 -> 1
        Surface.ROTATION_180 -> 2
        Surface.ROTATION_270 -> 3
        else -> 0
    }

    override fun locked(): Boolean = getSystemService(KeyguardManager::class.java).isKeyguardLocked

    override fun foreground(): Pair<String, String> {
        val active = windows.firstOrNull { it.type == AccessibilityWindowInfo.TYPE_APPLICATION && it.isActive }
        // An active window without content says nothing: unknown, so the host
        // asks dumpsys instead of trusting an old answer.
        if (active != null && active.root == null) return "" to ""
        val app = active?.root?.packageName?.toString()
        val known = foreground
        return when {
            app == null -> known
            app == known.first -> known
            else -> app to ""
        }
    }

    override fun click(node: UiNode): Boolean = (node.ref as? AccessibilityNodeInfo)?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true

    override fun tap(x: Int, y: Int): Boolean {
        val path = Path().apply { moveTo(x.toFloat(), y.toFloat()) }
        val gesture = GestureDescription.Builder().addStroke(GestureDescription.StrokeDescription(path, 0, TAP_MS)).build()
        val done = CountDownLatch(1)
        var ok = false
        val sent = dispatchGesture(
            gesture,
            object : GestureResultCallback() {
                override fun onCompleted(g: GestureDescription?) {
                    ok = true
                    done.countDown()
                }

                override fun onCancelled(g: GestureDescription?) = done.countDown()
            },
            null,
        )
        return sent && done.await(2, TimeUnit.SECONDS) && ok
    }

    override fun focusedInput(): UiNode? {
        val n = findFocus(AccessibilityNodeInfo.FOCUS_INPUT) ?: return null
        val r = Rect().also { n.getBoundsInScreen(it) }
        return UiNode(
            pkg = n.packageName?.toString().orEmpty(), cls = n.className?.toString().orEmpty(), bounds = Box(r.left, r.top, r.right, r.bottom),
            editable = n.isEditable, focused = true, password = n.isPassword, ref = n,
        )
    }

    override fun setText(node: UiNode, text: String): Boolean {
        val n = node.ref as? AccessibilityNodeInfo ?: return false
        val args = Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, text) }
        return n.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args)
    }

    override fun acting(mark: Box?): Boolean {
        val app = application as AdbuddyApp
        if (app.bridge.state.value.sessionState.paused) return true // the executor refuses while paused
        return overlay?.show(mark) ?: false
    }

    override fun hideNotice(ms: Long): Boolean = overlay?.hideFor(ms) ?: true // no overlay object: nothing on the screen

    companion object {
        private const val TAG = "ADBuddyA11y"
        private const val MAX_NODES = 8000
        private const val TAP_MS = 50L

        /** Heads-up window shared with UiHelpers: a fresh service starts with none (the host covers that gap with the shade). */
        val notifications = NotificationWindow(SystemClock::elapsedRealtime)

        /** The service while it is bound, or null. */
        @Volatile var current: AgentAccessibilityService? = null
            private set
    }
}
