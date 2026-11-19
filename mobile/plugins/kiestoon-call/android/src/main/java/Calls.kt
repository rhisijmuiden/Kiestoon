package nl.rhis.kiestoon.call

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.media.AudioAttributes
import android.media.RingtoneManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.Person

/** The incoming-call screen and the other notifications of the app. */
internal object Calls {
    const val CH_CALLS = "kiestoon-calls"
    const val CH_MESSAGES = "kiestoon-messages"
    const val CH_ONGOING = "kiestoon-ongoing"
    const val EXTRA_ACTION = "nl.rhis.kiestoon.call.ACTION"
    const val EXTRA_ID = "nl.rhis.kiestoon.call.ID"
    private const val ID_INCOMING = 4101
    const val ID_ONGOING = 4102
    private const val ID_VOICEMAIL = 4103
    private const val ID_MISSED = 4104
    private const val ID_TEST = 4105

    /** The id of the call that rings on the call screen now. */
    @Volatile var ringing: String? = null

    fun channels(ctx: Context) {
        val nm = ctx.getSystemService(NotificationManager::class.java)
        val ring = NotificationChannel(CH_CALLS, ctx.getString(R.string.kt_channel_calls), NotificationManager.IMPORTANCE_HIGH)
        ring.setSound(
            RingtoneManager.getDefaultUri(RingtoneManager.TYPE_RINGTONE),
            AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_NOTIFICATION_RINGTONE)
                .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION).build()
        )
        ring.enableVibration(true)
        nm.createNotificationChannel(ring)
        nm.createNotificationChannel(NotificationChannel(CH_MESSAGES, ctx.getString(R.string.kt_channel_messages), NotificationManager.IMPORTANCE_DEFAULT))
        nm.createNotificationChannel(NotificationChannel(CH_ONGOING, ctx.getString(R.string.kt_channel_ongoing), NotificationManager.IMPORTANCE_LOW))
    }

    /** Opens the app (its launcher activity), optionally with an action of the call screen. */
    fun appIntent(ctx: Context, action: String?, id: String?, req: Int): PendingIntent {
        val i = ctx.packageManager.getLaunchIntentForPackage(ctx.packageName)!!
        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        if (action != null) {
            i.putExtra(EXTRA_ACTION, action)
            i.putExtra(EXTRA_ID, id)
        }
        return PendingIntent.getActivity(ctx, req, i, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
    }

    private fun receiverIntent(ctx: Context, action: String, id: String, req: Int): PendingIntent {
        val i = Intent(ctx, CallActionReceiver::class.java).putExtra(EXTRA_ACTION, action).putExtra(EXTRA_ID, id)
        return PendingIntent.getBroadcast(ctx, req, i, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
    }

    fun showIncoming(ctx: Context, id: String, name: String, number: String) {
        channels(ctx)
        ringing = id
        val title = name.ifEmpty { number.ifEmpty { ctx.getString(R.string.kt_unknown) } }
        val caller = Person.Builder().setName(title).setImportant(true).build()
        val answer = appIntent(ctx, "answer", id, 1)
        val decline = receiverIntent(ctx, "decline", id, 2)
        val n = NotificationCompat.Builder(ctx, CH_CALLS)
            .setSmallIcon(R.drawable.kt_call)
            .setContentTitle(title)
            .setContentText(if (name.isNotEmpty() && number.isNotEmpty()) number else ctx.getString(R.string.kt_incoming))
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setPriority(NotificationCompat.PRIORITY_MAX)
            .setOngoing(true)
            .setAutoCancel(false)
            .setTimeoutAfter(45_000)
            .setFullScreenIntent(appIntent(ctx, "show", id, 3), true)
            .setStyle(NotificationCompat.CallStyle.forIncomingCall(caller, decline, answer))
            .build()
        n.flags = n.flags or Notification.FLAG_INSISTENT
        ctx.getSystemService(NotificationManager::class.java).notify(ID_INCOMING, n)
    }

    /** Takes the call screen away; reason "missed" leaves a missed-call notification. */
    fun endIncoming(ctx: Context, id: String?, reason: String?, name: String = "", number: String = "") {
        if (id != null && ringing != null && ringing != id) return
        ringing = null
        val nm = ctx.getSystemService(NotificationManager::class.java)
        nm.cancel(ID_INCOMING)
        if (reason == "missed") {
            val who = name.ifEmpty { number }
            val n = NotificationCompat.Builder(ctx, CH_MESSAGES)
                .setSmallIcon(R.drawable.kt_call)
                .setContentTitle(ctx.getString(R.string.kt_missed))
                .setContentText(who.ifEmpty { ctx.getString(R.string.kt_unknown) })
                .setContentIntent(appIntent(ctx, null, null, 4))
                .setAutoCancel(true)
                .build()
            nm.notify(ID_MISSED, n)
        }
    }

    fun showVoicemail(ctx: Context, from: String, summary: String, count: Int) {
        channels(ctx)
        val title = if (from.isNotEmpty()) ctx.getString(R.string.kt_voicemail_from, from) else ctx.getString(R.string.kt_voicemail)
        val text = summary.ifEmpty { if (count > 0) ctx.getString(R.string.kt_voicemail_count, count) else "" }
        val n = NotificationCompat.Builder(ctx, CH_MESSAGES)
            .setSmallIcon(R.drawable.kt_call)
            .setContentTitle(title)
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setContentIntent(appIntent(ctx, "voicemail", null, 5))
            .setAutoCancel(true)
            .build()
        ctx.getSystemService(NotificationManager::class.java).notify(ID_VOICEMAIL, n)
    }

    fun showTest(ctx: Context) {
        channels(ctx)
        val n = NotificationCompat.Builder(ctx, CH_MESSAGES)
            .setSmallIcon(R.drawable.kt_call)
            .setContentTitle(ctx.getString(R.string.kt_test))
            .setContentText(ctx.getString(R.string.kt_test_body))
            .setContentIntent(appIntent(ctx, null, null, 6))
            .setAutoCancel(true)
            .build()
        ctx.getSystemService(NotificationManager::class.java).notify(ID_TEST, n)
    }

    /** The notification of a call in progress (shown by CallService). */
    fun ongoing(ctx: Context, name: String, number: String): Notification {
        channels(ctx)
        val who = name.ifEmpty { number.ifEmpty { ctx.getString(R.string.kt_ongoing) } }
        val caller = Person.Builder().setName(who).build()
        val b = NotificationCompat.Builder(ctx, CH_ONGOING)
            .setSmallIcon(R.drawable.kt_call)
            .setContentTitle(who)
            .setContentText(ctx.getString(R.string.kt_ongoing))
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setOngoing(true)
            .setUsesChronometer(true)
            .setContentIntent(appIntent(ctx, null, null, 7))
        b.setStyle(NotificationCompat.CallStyle.forOngoingCall(caller, receiverIntent(ctx, "end", "", 8)))
        return b.build()
    }

    fun sdk(v: Int) = Build.VERSION.SDK_INT >= v
}
