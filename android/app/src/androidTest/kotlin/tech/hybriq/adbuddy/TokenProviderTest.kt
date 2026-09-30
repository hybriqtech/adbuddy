package tech.hybriq.adbuddy

import android.content.ContentValues
import android.net.Uri
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import android.os.ParcelFileDescriptor
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Spec §10: the TokenProvider refuses anyone but the shell user. The test
 * runs in the app's own process (not uid 2000), like any other app would.
 */
@RunWith(AndroidJUnit4::class)
class TokenProviderTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val context = instrumentation.targetContext
    private val uri = Uri.parse("content://tech.hybriq.adbuddy.bridge")

    @Test fun callsFromAnAppAreRefused() {
        for (method in listOf("ping", "requestSetup", "getSetupConsent")) {
            val res = context.contentResolver.call(uri, method, null, null)!!
            assertEquals(method, "not the shell user", res.getString("error"))
        }
    }

    @Test fun handshakeFromAnAppIsRefused() {
        try {
            context.contentResolver.openOutputStream(Uri.parse("content://tech.hybriq.adbuddy.bridge/handshake"))!!.use {
                it.write("{}".toByteArray())
            }
            fail("an app could write the handshake")
        } catch (e: SecurityException) {
            // expected
        }
    }

    @Test fun queryFromAnAppGivesNoData() {
        try {
            context.contentResolver.query(Uri.parse("content://tech.hybriq.adbuddy.bridge/handshake"), null, null, null, null).use {
                assertNull("an app got a cursor", it)
            }
        } catch (e: SecurityException) {
            // expected
        }
    }

    @Test fun writesFromAnAppAreRefused() {
        val r = context.contentResolver
        val u = Uri.parse("content://tech.hybriq.adbuddy.bridge/handshake")
        for ((name, op) in listOf<Pair<String, () -> Unit>>(
            "insert" to { r.insert(u, ContentValues()) },
            "update" to { r.update(u, ContentValues(), null, null) },
            "delete" to { r.delete(u, null, null) },
        )) {
            try {
                op()
                fail("$name from an app was not refused")
            } catch (e: SecurityException) {
                // expected
            }
        }
    }

    @Test fun getTypeAndReadingFromAnAppAreRefused() {
        val u = Uri.parse("content://tech.hybriq.adbuddy.bridge/handshake")
        try {
            context.contentResolver.getType(u)
            fail("getType from an app was not refused")
        } catch (e: SecurityException) {
            // expected
        }
        try {
            context.contentResolver.openInputStream(u)?.close()
            fail("an app could open the handshake for reading")
        } catch (e: SecurityException) {
            // expected
        }
    }

    // The shell user (uid 2000), as the host reaches it over adb.

    private fun shell(command: String): String {
        val pfd = instrumentation.uiAutomation.executeShellCommand(command)
        return ParcelFileDescriptor.AutoCloseInputStream(pfd).use { it.readBytes().decodeToString() }
    }

    @Test fun theShellUserGetsAPing() {
        val out = shell("content call --uri content://tech.hybriq.adbuddy.bridge --method ping")
        assertTrue(out, out.contains("ok=true"))
        assertTrue(out, out.contains("port="))
        assertFalse(out, out.contains("not the shell user"))
    }

    @Test fun theShellUserCannotReadTheHandshake() {
        val out = shell("content read --uri content://tech.hybriq.adbuddy.bridge/handshake")
        assertFalse("the handshake was readable: $out", out.contains("token"))
    }
}
