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
import tech.hybriq.adbuddy.notify.ConversationNotifier
import tech.hybriq.adbuddy.notify.ConversationView
import tech.hybriq.adbuddy.notify.LastMessage
import tech.hybriq.adbuddy.ui.LinkStatus
import tech.hybriq.adbuddy.ui.StatusView

/**
 * T5 of the manual validations, the app's half: posts a notification and keeps it posted while
 * host/internal/emutest (guard_test.go) records what the host's guard reads of the screen with it.
 * - `-e adbuddyGuard headsup`: a plain notification on a channel of high importance, which the system shows as a
 *   heads-up (`cmd notification post` cannot: its channel is not of high importance). A title and a line, no code.
 * - `-e adbuddyGuard bubble`: the conversation notification as BridgeService builds it, posted on its own without
 *   the foreground service: only then can it bubble on API 34 (T1-8), and the bubble opens ChatBubbleActivity.
 * It ends when the host sends the broadcast [DONE] (or after holdMs) and cancels the notification whatever happens.
 * It only runs with -e adbuddyGuard, which only the host passes: in the whole suite (TestInstrumentedSuite) it is
 * skipped.
 */
@RunWith(AndroidJUnit4::class)
class GuardOnEmulatorTest {
    @Test fun postAndHold() {
        val args = InstrumentationRegistry.getArguments()
        val kind = args.getString("adbuddyGuard")
        assumeTrue("only host/internal/emutest runs it", kind == HEADS_UP || kind == BUBBLE)
        // Never on a phone, whoever starts it: the emulator's hardware is ranchu (goldfish before it).
        assertTrue("not an emulator (Build.HARDWARE=${Build.HARDWARE}): the guard's states are only set on one",
            Build.HARDWARE == "ranchu" || Build.HARDWARE == "goldfish")
        val holdMs = args.getString("holdMs")?.toLongOrNull() ?: 180_000L
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val nm = context.getSystemService(NotificationManager::class.java)
        assertTrue("notifications are off for the app: the host grants POST_NOTIFICATIONS first", nm.areNotificationsEnabled())
        val id: Int
        val n: Notification
        if (kind == HEADS_UP) {
            nm.createNotificationChannel(NotificationChannel(CHANNEL, "Guard test", NotificationManager.IMPORTANCE_HIGH))
            id = HEADS_UP_ID
            n = Notification.Builder(context, CHANNEL)
                .setSmallIcon(android.R.drawable.ic_dialog_info)
                .setContentTitle(TITLE)
                .setContentText("A heads-up for the guard test")
                .build()
        } else {
            id = BUBBLE_ID
            val view = ConversationView(StatusView(LinkStatus.CUT, "", null), "Guard test", "", "", LastMessage("g1", false, TITLE))
            n = ConversationNotifier.build(context, view, System.currentTimeMillis())
        }
        val done = CountDownLatch(1)
        val receiver = object : BroadcastReceiver() {
            override fun onReceive(c: Context, i: Intent) = done.countDown()
        }
        ContextCompat.registerReceiver(context, receiver, IntentFilter(DONE), ContextCompat.RECEIVER_EXPORTED)
        var released = false
        try {
            nm.notify(id, n)
            Log.i(TAG, "posted $kind id=$id")
            released = done.await(holdMs, TimeUnit.MILLISECONDS)
            Log.i(TAG, "released=$released")
        } finally {
            nm.cancel(id)
            if (kind == HEADS_UP) nm.deleteNotificationChannel(CHANNEL)
            context.unregisterReceiver(receiver)
        }
        assertTrue("the host did not send $DONE in $holdMs ms", released)
    }

    private companion object {
        const val TAG = "ADBuddyEmuGuard"
        const val HEADS_UP = "headsup"
        const val BUBBLE = "bubble"
        const val CHANNEL = "adbuddy_guard_test"
        const val HEADS_UP_ID = 7302
        const val BUBBLE_ID = 7303
        const val DONE = "tech.hybriq.adbuddy.test.GUARD_DONE"
        const val TITLE = "ADBuddy guard test"
    }
}
