package tech.hybriq.adbuddy.notify

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.Person
import androidx.core.app.RemoteInput
import androidx.core.content.LocusIdCompat
import androidx.core.content.pm.ShortcutInfoCompat
import androidx.core.content.pm.ShortcutManagerCompat
import androidx.core.graphics.drawable.IconCompat
import tech.hybriq.adbuddy.AdbuddyApp
import tech.hybriq.adbuddy.ChatBubbleActivity
import tech.hybriq.adbuddy.MainActivity
import tech.hybriq.adbuddy.R
import tech.hybriq.adbuddy.bridge.BridgeService

/**
 * The persistent conversation notification (spec M3 section 7.4): MessagingStyle with the
 * last message, direct reply, bubble, and the "Done" notice. Nothing here logs text.
 */
object ConversationNotifier {
    const val KEY_REPLY = "reply"
    const val SHORTCUT_ID = "conversation"
    const val DONE_ID = 3

    @Volatile private var pushedFor: String? = null

    fun build(ctx: Context, v: ConversationView, ts: Long): Notification {
        val h = v.headline()
        val p = v.publicHeadline()
        val publicLine = if (p.arg != null) ctx.getString(p.res, p.arg) else ctx.getString(p.res)
        val headline = if (h.arg != null) ctx.getString(h.res, h.arg) else ctx.getString(h.res)
        val agentName = v.agent.ifEmpty { ctx.getString(R.string.app_name) }
        val me = Person.Builder().setName(ctx.getString(R.string.notif_you)).build()
        val agent = Person.Builder().setName(agentName).setKey(SHORTCUT_ID).setImportant(true).build()
        pushShortcut(ctx, agentName, agent)

        val style = NotificationCompat.MessagingStyle(me)
        val last = v.last
        if (last != null) style.addMessage(last.text, ts, if (last.fromUser) me else agent)
        else style.addMessage(headline, ts, agent)

        val open = PendingIntent.getActivity(ctx, 0, Intent(ctx, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val bubbleIntent = PendingIntent.getActivity(ctx, 5, Intent(ctx, ChatBubbleActivity::class.java),
            PendingIntent.FLAG_MUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val bubble = NotificationCompat.BubbleMetadata.Builder(bubbleIntent, IconCompat.createWithResource(ctx, android.R.drawable.sym_action_chat))
            .setDesiredHeight(600)
            .build()
        // What the lock screen shows: the headline, never the text of the last message.
        val public = NotificationCompat.Builder(ctx, AdbuddyApp.CHANNEL_CONVERSATION)
            .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
            .setContentTitle(ctx.getString(R.string.app_name))
            .setContentText(publicLine)
            .build()

        val b = NotificationCompat.Builder(ctx, AdbuddyApp.CHANNEL_CONVERSATION)
            .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
            .setStyle(style)
            .setSubText(headline)
            .setCategory(NotificationCompat.CATEGORY_MESSAGE)
            .setShortcutId(SHORTCUT_ID)
            .setLocusId(LocusIdCompat(SHORTCUT_ID))
            .setBubbleMetadata(bubble)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setPublicVersion(public)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setContentIntent(open)

        // Reply: the system fills the result in, hence MUTABLE. Locked: setAuthenticationRequired (API 31+) and,
        // on every version, BridgeService ignores the action while the device is locked.
        for (a in v.actions()) when (a) {
            NotifAction.REPLY -> b.addAction(NotificationCompat.Action.Builder(0, ctx.getString(R.string.notif_reply),
                service(ctx, 3, BridgeService.ACTION_REPLY, mutable = true))
                .addRemoteInput(RemoteInput.Builder(KEY_REPLY).setLabel(ctx.getString(R.string.notif_reply_hint)).build())
                .setSemanticAction(NotificationCompat.Action.SEMANTIC_ACTION_REPLY)
                .setAllowGeneratedReplies(false)
                .setAuthenticationRequired(true)
                .build())
            NotifAction.CAPTURE -> b.addAction(NotificationCompat.Action.Builder(0, ctx.getString(R.string.notif_capture),
                service(ctx, 4, BridgeService.ACTION_CAPTURE, mutable = false))
                .setAuthenticationRequired(true)
                .build())
            NotifAction.PAUSE -> b.addAction(0, ctx.getString(R.string.action_pause), service(ctx, 1, BridgeService.ACTION_PAUSE, mutable = false))
            NotifAction.RESUME -> b.addAction(0, ctx.getString(R.string.action_resume), service(ctx, 2, BridgeService.ACTION_RESUME, mutable = false))
            // Cut access: no unlock needed. It only narrows what the agent can do, so it is always safe and urgent.
            NotifAction.CUT -> b.addAction(0, ctx.getString(R.string.action_cut), service(ctx, 6, BridgeService.ACTION_CUT, mutable = false))
        }
        return b.build()
    }

    /** "Done": the agent ended a stretch of work. Plain NotificationManager.notify (lint MissingPermission with the Compat one). */
    fun done(ctx: Context, line: String?) {
        if (!NotificationManagerCompat.from(ctx).areNotificationsEnabled()) return
        val open = PendingIntent.getActivity(ctx, 0, Intent(ctx, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val n = NotificationCompat.Builder(ctx, AdbuddyApp.CHANNEL_DONE)
            .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
            .setContentTitle(ctx.getString(R.string.notif_done_title))
            .setContentText(line ?: ctx.getString(R.string.notif_done_default))
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setPublicVersion(NotificationCompat.Builder(ctx, AdbuddyApp.CHANNEL_DONE)
                .setSmallIcon(android.R.drawable.stat_notify_sync_noanim)
                .setContentTitle(ctx.getString(R.string.notif_done_title))
                .build())
            .setAutoCancel(true)
            .setContentIntent(open)
            .build()
        ctx.getSystemService(NotificationManager::class.java).notify(DONE_ID, n)
    }

    private fun service(ctx: Context, code: Int, action: String, mutable: Boolean): PendingIntent =
        PendingIntent.getService(ctx, code, Intent(ctx, BridgeService::class.java).setAction(action),
            if (mutable) PendingIntent.FLAG_MUTABLE or PendingIntent.FLAG_UPDATE_CURRENT else PendingIntent.FLAG_IMMUTABLE)

    /** The conversation shortcut the bubble needs; published once per agent name, not with every notification. */
    private fun pushShortcut(ctx: Context, agentName: String, agent: Person) {
        if (pushedFor == agentName) return
        val open = Intent(ctx, MainActivity::class.java).setAction(Intent.ACTION_VIEW)
        val info = ShortcutInfoCompat.Builder(ctx, SHORTCUT_ID)
            .setShortLabel(agentName)
            .setLongLived(true)
            .setIsConversation()
            .setPerson(agent)
            .setLocusId(LocusIdCompat(SHORTCUT_ID))
            .setIntent(open)
            .build()
        ShortcutManagerCompat.pushDynamicShortcut(ctx, info)
        pushedFor = agentName
    }
}
