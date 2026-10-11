package tech.hybriq.adbuddy.bridge

import android.content.Context
import android.content.SharedPreferences
import android.util.Log

/**
 * What the host handed over (token, port, name) and the setup consent. Kept in
 * the app's private storage: the release build is not debuggable, so no other
 * app nor `run-as` can read it (and the host's classifier blocks `run-as` on
 * ADBuddy anyway).
 */
class BridgePrefs(context: Context) {
    private val prefs = context.getSharedPreferences("bridge", Context.MODE_PRIVATE)

    /** nonce: what the host sent with this token; ping returns it so the host knows the app took it. */
    data class Link(val token: String, val port: Int, val host: String, val nonce: String = "") {
        // Keeps the token and the nonce out of logs and crash reports.
        override fun toString() = "Link(token=<redacted>, port=$port, host=$host, nonce=<redacted>)"
    }

    var link: Link?
        get() {
            val token = prefs.getString("token", null) ?: return null
            val port = prefs.getInt("port", 0)
            if (port !in 1024..65535) return null
            return Link(token, port, prefs.getString("host", "") ?: "", prefs.getString("nonce", "") ?: "")
        }
        set(value) = save("link") {
            if (value == null) {
                remove("token"); remove("port"); remove("host"); remove("nonce")
            } else {
                putString("token", value.token); putInt("port", value.port); putString("host", value.host)
                putString("nonce", value.nonce)
            }
        }

    /** pending (asked, no answer yet), granted, denied, or "" (never asked). */
    var consent: String
        get() = prefs.getString("consent", "") ?: ""
        set(value) = save("consent") { putString("consent", value) }

    var consentHost: String
        get() = prefs.getString("consentHost", "") ?: ""
        set(value) = save("consentHost") { putString("consentHost", value) }

    /** The reason for the notification permission was shown and answered (either way): never asked again. */
    var notificationsAsked: Boolean
        get() = prefs.getBoolean("notifications_asked", false)
        set(value) = save("notifications_asked") { putBoolean("notifications_asked", value) }

    /** Writes at once (commit); a failure is logged by name only, never with the values. */
    private fun save(what: String, change: SharedPreferences.Editor.() -> Unit) {
        val editor = prefs.edit()
        editor.change()
        if (!editor.commit()) Log.w(TAG, "could not save $what")
    }

    private companion object {
        const val TAG = "BridgePrefs"
    }
}
