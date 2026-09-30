package tech.hybriq.adbuddy

import android.content.ContentValues
import android.net.Uri
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Spec §10: the TokenProvider refuses anyone but the shell user. The test
 * runs in the app's own process (not uid 2000), like any other app would.
 */
@RunWith(AndroidJUnit4::class)
class TokenProviderTest {
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
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
}
