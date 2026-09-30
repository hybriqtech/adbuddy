package tech.hybriq.adbuddy.bridge

import android.content.Context
import androidx.core.content.edit

/**
 * What the host handed over (token, port, name) and the setup consent. Kept in
 * the app's private storage: the release build is not debuggable, so no other
 * app nor `run-as` can read it (and the host's classifier blocks `run-as` on
 * ADBuddy anyway).
 */
class BridgePrefs(context: Context) {
    private val prefs = context.getSharedPreferences("bridge", Context.MODE_PRIVATE)

    /** nonce: what the host sent with this token; ping returns it so the host knows the app took it. */
    data class Link(val token: String, val port: Int, val host: String, val nonce: String = "")

    var link: Link?
        get() {
            val token = prefs.getString("token", null) ?: return null
            val port = prefs.getInt("port", 0)
            if (port !in 1024..65535) return null
            return Link(token, port, prefs.getString("host", "") ?: "", prefs.getString("nonce", "") ?: "")
        }
        set(value) = prefs.edit(commit = true) {
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
        set(value) = prefs.edit(commit = true) { putString("consent", value) }

    var consentHost: String
        get() = prefs.getString("consentHost", "") ?: ""
        set(value) = prefs.edit(commit = true) { putString("consentHost", value) }
}
