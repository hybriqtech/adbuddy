package tech.hybriq.adbuddy.listener

import android.app.NotificationManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.database.ContentObserver
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.conflate
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.flow.map
import tech.hybriq.adbuddy.ui.startFirst

/**
 * Binds NotificationAccess to the system: reads the list of enabled
 * listeners, follows it, and opens the screen where the user allows ADBuddy.
 * Only the user can grant access (the system's own screen); the app never does it.
 */
object NotificationAccessStatus {
    private const val SETTING = "enabled_notification_listeners"

    /**
     * What the system says now: its own answer, with the setting as a second source; null when neither
     * can be read (the screens then say unknown) and unknown when they disagree.
     */
    fun listed(context: Context): NotificationAccess.Listed? {
        val me = ComponentName(context, AgentNotificationListener::class.java)
        val api = try {
            context.getSystemService(NotificationManager::class.java)?.isNotificationListenerAccessGranted(me)
        } catch (e: RuntimeException) {
            null
        }
        val setting = try {
            NotificationAccess.listed(Settings.Secure.getString(context.contentResolver, SETTING))
        } catch (e: RuntimeException) {
            null
        }
        return NotificationAccess.combine(api, setting)
    }

    /** The state now and on every change of the setting or of the listener's connection. */
    fun observe(context: Context): Flow<NotificationAccess.State> {
        val resolver = context.contentResolver
        val setting = callbackFlow {
            // onChange runs on the main thread: it only signals, listed() (binder) is read below, off it.
            val observer = object : ContentObserver(Handler(Looper.getMainLooper())) {
                override fun onChange(selfChange: Boolean) {
                    trySend(Unit)
                }
            }
            try {
                resolver.registerContentObserver(Settings.Secure.getUriFor(SETTING), false, observer)
            } catch (e: RuntimeException) {
                // Not observable: the value read below still shows when the screen is opened again.
            }
            trySend(Unit)
            awaitClose { resolver.unregisterContentObserver(observer) }
        }.conflate().map { listed(context) }
        return combine(setting, AgentNotificationListener.connected) { listed, connected -> NotificationAccess.state(listed, connected) }
            .distinctUntilChanged()
            // listed() talks to the system (binder): the first read and each change run in the map above, off the main thread.
            .flowOn(Dispatchers.IO)
    }

    /** Opens ADBuddy's own entry of the system's notification access; the list if the phone has no such entry. */
    fun openSystemSettings(context: Context) {
        val me = ComponentName(context, AgentNotificationListener::class.java)
        context.startFirst(
            Intent(Settings.ACTION_NOTIFICATION_LISTENER_DETAIL_SETTINGS).putExtra(Settings.EXTRA_NOTIFICATION_LISTENER_COMPONENT_NAME, me.flattenToString()),
            Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS),
            Intent(Settings.ACTION_SETTINGS),
        )
    }
}
