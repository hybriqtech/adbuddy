package tech.hybriq.adbuddy

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.util.Log
import androidx.core.content.ContextCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * M5b-18 / M5c-35, the app's half: posts a notification of its own with a ticker that starts with a code and a
 * text with another one, and keeps it posted while host/internal/emutest (notification_dump_test.go) reads what
 * `dumpsys notification` and `cmd notification` show of it. `cmd notification post` cannot set a ticker (T1),
 * hence an instrumented test. It ends when the host sends the broadcast [DONE] (or after holdMs) and cancels the
 * notification whatever happens. It only runs with -e adbuddyTicker 1, which only the host passes: in the whole
 * suite (TestInstrumentedSuite) it is skipped.
 */
@RunWith(AndroidJUnit4::class)
class TickerOnEmulatorTest {
    @Test fun postAndHold() {
        val args = InstrumentationRegistry.getArguments()
        assumeTrue("only host/internal/emutest runs it", args.getString("adbuddyTicker") == "1")
        // Never on a phone, whoever starts it: the emulator's hardware is ranchu (goldfish before it).
        assertTrue("not an emulator (Build.HARDWARE=${Build.HARDWARE}): the ticker is only tested on one",
            Build.HARDWARE == "ranchu" || Build.HARDWARE == "goldfish")
        val holdMs = args.getString("holdMs")?.toLongOrNull() ?: 180_000L
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val nm = context.getSystemService(NotificationManager::class.java)
        assertTrue("notifications are off for the app: the host grants POST_NOTIFICATIONS first", nm.areNotificationsEnabled())
        nm.createNotificationChannel(NotificationChannel(CHANNEL, "Ticker test", NotificationManager.IMPORTANCE_DEFAULT))
        val done = CountDownLatch(1)
        val receiver = object : BroadcastReceiver() {
            override fun onReceive(c: Context, i: Intent) = done.countDown()
        }
        ContextCompat.registerReceiver(context, receiver, IntentFilter(DONE), ContextCompat.RECEIVER_EXPORTED)
        var released = false
        try {
            val n = Notification.Builder(context, CHANNEL)
                .setSmallIcon(android.R.drawable.ic_dialog_info)
                .setContentTitle("ADBuddy ticker test")
                .setContentText(TEXT)
                .setTicker(TICKER)
                .build()
            nm.notify(ID, n)
            Log.i(TAG, "posted id=$ID")
            released = done.await(holdMs, TimeUnit.MILLISECONDS)
            Log.i(TAG, "released=$released")
        } finally {
            nm.cancel(ID)
            nm.deleteNotificationChannel(CHANNEL)
            context.unregisterReceiver(receiver)
        }
        assertTrue("the host did not send $DONE in $holdMs ms", released)
    }

    private companion object {
        const val TAG = "ADBuddyEmuTicker"
        const val CHANNEL = "adbuddy_ticker_test"
        const val ID = 7301
        const val DONE = "tech.hybriq.adbuddy.test.TICKER_DONE"
        // The ticker starts with the code, as many verification messages do (the redacted dump keeps its first
        // characters, AOSP NotificationRecord.dump); the text carries another one.
        const val TICKER = "G-482913 is your ADBuddy test code"
        const val TEXT = "Your other ADBuddy test code is 735102"
    }
}
