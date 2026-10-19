package tech.hybriq.adbuddy.yolo

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.Protocol

/**
 * Receives the Turn off of the YOLO notification (not exported: only this app's PendingIntent reaches it). Turning the
 * mode off is always safe, so it asks nothing, waits for nothing and needs no unlock. If the request cannot be sent (no
 * connection) the notification says so and names `adbuddy yolo off`, which also works from the PC.
 */
class YoloReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION_OFF) return
        val app = context.applicationContext as AdbuddyApp
        val sent = app.bridge.request(Protocol.YOLO_SET, YoloUi.offRequest()) { } != null
        if (!sent) YoloNotifier.offFailed(app, app.bridge.state.value, app.clock)
    }

    companion object {
        const val ACTION_OFF = "tech.hybriq.adbuddy.action.YOLO_OFF"
    }
}
