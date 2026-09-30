package tech.hybriq.adbuddy.a11y

import android.accessibilityservice.AccessibilityService
import android.util.Log
import android.view.accessibility.AccessibilityEvent

/**
 * ADBuddy's accessibility service. In M2 it only has to exist and bind, so the
 * host can enable it over adb (spec §3.1 step 6) and M2 can validate that on
 * another maker's Android 13-15. Reading the screen, tapping and the overlay
 * arrive in M5.
 */
class AgentAccessibilityService : AccessibilityService() {
    override fun onServiceConnected() {
        Log.i(TAG, "connected")
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) = Unit

    override fun onInterrupt() = Unit

    companion object {
        private const val TAG = "ADBuddyA11y"
    }
}
