package tech.hybriq.adbuddy.bridge

import android.content.ContentProvider
import android.content.ContentValues
import android.database.Cursor
import android.net.Uri
import android.os.Binder
import android.os.Bundle
import android.os.ParcelFileDescriptor
import android.util.Log
import java.io.FileNotFoundException
import java.io.IOException
import tech.hybriq.adbuddy.BuildConfigInfo

/**
 * The host's door into the app (spec §5.2, §3.1 step 6). Exported, so adb can
 * reach it, but it answers only the shell user (uid 2000): every other app
 * gets an error, and every entry point checks the caller (call, openFile,
 * query, insert, update, delete and getType).
 *
 * Only the shell user, on purpose: with `adb root` (uid 0, as on emulators and
 * userdebug builds) the host talks as root and the handshake is refused too.
 *
 * - `content write --uri content://tech.hybriq.adbuddy.bridge/handshake` with
 *   the token, port and PC name as JSON on stdin (never in a command line).
 * - `content call … --method requestSetup --extra host:s:<name>` opens a
 *   consent request; `getSetupConsent` answers pending, granted or denied.
 * - `content call … --method ping` answers the port the app will use and the
 *   nonce of the last handshake it took (not a secret: it proves which one).
 *
 * It does not override dump(), and must not: `dumpsys activity provider` is
 * 🟢 for the agent, so anything dumped here would reach it.
 */
class TokenProvider : ContentProvider() {
    private lateinit var prefs: BridgePrefs

    override fun onCreate(): Boolean {
        prefs = BridgePrefs(context!!)
        return true
    }

    override fun call(method: String, arg: String?, extras: Bundle?): Bundle {
        if (!isShell()) return error("not the shell user")
        return when (method) {
            "ping" -> Bundle().apply {
                putBoolean("ok", true)
                putString("appVersion", BuildConfigInfo.versionName(context!!))
                putInt("port", prefs.link?.port ?: 0)
                putString("nonce", prefs.link?.nonce ?: "")
            }
            "requestSetup" -> {
                val host = extras?.getString("host")?.takeIf { Regex("^[A-Za-z0-9 ._-]{1,40}$").matches(it) } ?: "PC"
                prefs.consentHost = host
                prefs.consent = "pending"
                Bundle().apply { putBoolean("ok", true) }
            }
            "getSetupConsent" -> Bundle().apply { putString("consent", prefs.consent.ifEmpty { "pending" }) }
            else -> error("unknown method")
        }
    }

    override fun openFile(uri: Uri, mode: String): ParcelFileDescriptor {
        requireShell()
        if (uri.path != "/handshake" || !mode.startsWith("w")) throw FileNotFoundException(uri.toString())
        val (read, write) = ParcelFileDescriptor.createReliablePipe()
        val ctx = context!!.applicationContext
        Thread {
            val bytes = try {
                ParcelFileDescriptor.AutoCloseInputStream(read).use { HandshakeParser.readLimited(it) }
            } catch (e: IOException) {
                // The writer went away or closed with an error: keep the previous link.
                Log.w(TAG, "could not read the handshake", e)
                return@Thread
            }
            val h = HandshakeParser.parse(bytes)
            if (h != null) {
                prefs.link = BridgePrefs.Link(h.token, h.port, h.host, h.nonce)
                try {
                    BridgeService.start(ctx)
                } catch (e: IllegalStateException) {
                    // Android 12+ may refuse a foreground service started from the
                    // background (ForegroundServiceStartNotAllowedException): the
                    // link is saved, and the service starts when the app is opened.
                    Log.w(TAG, "could not start the channel service", e)
                }
            }
        }.start()
        return write
    }

    private fun isShell() = Caller.isShell(Binder.getCallingUid())

    private fun requireShell() {
        if (!isShell()) throw SecurityException("not the shell user")
    }

    private fun error(message: String) = Bundle().apply { putString("error", message) }

    // The provider exposes no table; still, only the shell user gets an answer.
    override fun query(uri: Uri, p: Array<String>?, s: String?, a: Array<String>?, o: String?): Cursor? {
        requireShell()
        return null
    }

    override fun getType(uri: Uri): String? {
        requireShell()
        return null
    }

    override fun insert(uri: Uri, values: ContentValues?): Uri? {
        requireShell()
        return null
    }

    override fun delete(uri: Uri, s: String?, a: Array<String>?): Int {
        requireShell()
        return 0
    }

    override fun update(uri: Uri, v: ContentValues?, s: String?, a: Array<String>?): Int {
        requireShell()
        return 0
    }

    private companion object {
        const val TAG = "ADBuddy"
    }
}
