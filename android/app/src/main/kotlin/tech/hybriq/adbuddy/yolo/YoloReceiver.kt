package tech.hybriq.adbuddy.yolo

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import kotlinx.serialization.json.JsonElement
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

/**
 * Receives the Turn off of the YOLO notification (not exported: only this app's PendingIntent reaches it). Turning the
 * mode off is always safe, so it asks nothing, waits for nothing and needs no unlock. If the request cannot be sent (no
 * connection), or the host refuses or does not answer, the notification says so and names `adbuddy yolo off`, which
 * also works from the PC.
 */
class YoloReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION_OFF) return
        val app = context.applicationContext as AdbuddyApp
        turnOff(app) { payload, onResult -> app.bridge.request(Protocol.YOLO_SET, payload, onResult) }
    }

    companion object {
        const val ACTION_OFF = "tech.hybriq.adbuddy.action.YOLO_OFF"

        /** [request] sends yolo.set and returns its rid, or null when it could not go. */
        internal fun turnOff(app: AdbuddyApp, request: (JsonElement, (Message?) -> Unit) -> String?) {
            val failed = { YoloNotifier.offFailed(app, app.bridge.state.value, app.clock) }
            val sent = request(YoloUi.offRequest()) { reply ->
                if (reply == null || reply.type == Protocol.ERROR) failed()
            }
            if (sent == null) failed()
        }
    }
}
