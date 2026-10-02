package tech.hybriq.adbuddy.listener

import android.app.Notification
import android.os.Build
import android.os.Process
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * ADBuddy's notification listener (M5 spec §3.5): it only READS the
 * notifications that are on the phone, for `adbuddy notifications`, once the
 * user has allowed it in the system's notification access.
 *
 * It is read-only on purpose, and a unit test (ListenerIsReadOnlyTest) keeps
 * it so: it never dismisses, snoozes or acts on a notification, never opens
 * an intent of one, and never logs what one says. What it reads reaches the
 * agent only through NotificationReport, which leaves out the content of
 * ADBuddy and of the protected apps (a 2FA code never leaves the phone).
 * Nothing outside this package can reach the service itself: only read()
 * and connected are public.
 */
class AgentNotificationListener : NotificationListenerService() {
    override fun onListenerConnected() {
        current = this
        _connected.value = true
    }

    override fun onListenerDisconnected() {
        // The system unbinds the listener when the access is revoked: the user's mark goes at once, without waiting
        // for the gate or the screen to look (ListenerIsReadOnlyTest keeps this call here).
        NotificationGate.onUnbound { NotificationConsent.of(this).revoke() }
        if (current === this) {
            current = null
            _connected.value = false
        }
    }

    override fun onDestroy() {
        NotificationGate.onUnbound { NotificationConsent.of(this).revoke() }
        if (current === this) {
            current = null
            _connected.value = false
        }
        super.onDestroy()
    }

    companion object {
        /** The listener while the system has it bound, or null. Private: nothing else may touch the service. */
        @Volatile private var current: AgentNotificationListener? = null

        private val _connected = MutableStateFlow(false)

        /** Whether the system has the listener bound now: with the setting, it says if access is granted. */
        val connected: StateFlow<Boolean> = _connected

        /**
         * The active notifications, or null when there is no listener (access not
         * granted, or not connected yet) or the system would not give them.
         * Called from the helpers' thread, never the main one.
         */
        fun read(): List<RawNotification>? {
            val service = current ?: return null
            val active = try {
                service.activeNotifications
            } catch (e: RuntimeException) {
                null
            } ?: return null
            val ranking = try {
                service.currentRanking
            } catch (e: RuntimeException) {
                null
            }
            val r = NotificationListenerService.Ranking()
            return active.map { sbn -> raw(sbn, sensitive = sensitive(ranking, sbn, r)) }
        }

        /**
         * The user, or the app's channel, hides it from the lock screen. Whatever cannot be asked counts as
         * sensitive: no Ranking at all, or one without this key, or one that throws (Sensitivity.of).
         * (Ranking.hasSensitiveContent, which Android 15 fills for one-time codes, is not public API.)
         */
        private fun sensitive(ranking: NotificationListenerService.RankingMap?, sbn: StatusBarNotification, r: NotificationListenerService.Ranking): Boolean = try {
            val known = ranking != null && ranking.getRanking(sbn.key, r)
            // The channel (r.channel) exists from API 26; the Ranking's lockscreen override, from API 31 (S).
            Sensitivity.of(
                rankingKnown = known,
                lockscreenOverride = if (known && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) r.lockscreenVisibilityOverride else null,
                channelVisibility = if (known) r.channel?.lockscreenVisibility else null,
            )
        } catch (e: RuntimeException) {
            true
        }

        /**
         * What one notification says. One that cannot be read comes out secret, with no text: failing
         * closed, never skipped silently. So does one of another user or of the work profile, or that the
         * system or its channel hides from the lock screen. The time is the system's (postTime), never the
         * app's own Notification.when.
         */
        internal fun raw(sbn: StatusBarNotification, sensitive: Boolean = true): RawNotification {
            val pkg = try { sbn.packageName.orEmpty() } catch (e: RuntimeException) { "" }
            val key = try { sbn.key.orEmpty() } catch (e: RuntimeException) { "" }
            val postedAt = try { sbn.postTime } catch (e: RuntimeException) { 0L }
            return try {
                val n = sbn.notification
                val extras = n.extras
                RawNotification(
                    key = key,
                    pkg = pkg,
                    title = extras?.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty(),
                    text = (extras?.getCharSequence(Notification.EXTRA_BIG_TEXT) ?: extras?.getCharSequence(Notification.EXTRA_TEXT))?.toString().orEmpty(),
                    postedAtMs = postedAt,
                    ongoing = sbn.isOngoing,
                    secret = sensitive || n.visibility == Notification.VISIBILITY_SECRET || sbn.user != Process.myUserHandle(),
                )
            } catch (e: RuntimeException) {
                RawNotification(key, pkg, "", "", postedAt, ongoing = false, secret = true)
            }
        }
    }
}
