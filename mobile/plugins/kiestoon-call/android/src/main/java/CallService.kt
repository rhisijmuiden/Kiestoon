package nl.rhis.kiestoon.call

import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.IBinder
import androidx.core.app.ServiceCompat

/**
 * Runs while a call is in progress, so Android keeps the microphone and the connection of the
 * app when the user switches to another app or locks the screen.
 */
class CallService : Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val n = Calls.ongoing(this, intent?.getStringExtra("name") ?: "", intent?.getStringExtra("number") ?: "")
        val type = if (Calls.sdk(30)) ServiceInfo.FOREGROUND_SERVICE_TYPE_PHONE_CALL or ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE else 0
        try {
            ServiceCompat.startForeground(this, Calls.ID_ONGOING, n, type)
        } catch (e: Exception) {
            stopSelf()
        }
        return START_NOT_STICKY
    }

    companion object {
        fun start(ctx: Context, name: String, number: String) {
            val i = Intent(ctx, CallService::class.java).putExtra("name", name).putExtra("number", number)
            try {
                ctx.startForegroundService(i)
            } catch (e: Exception) {
                // Not allowed from the background: the call goes on without the notification.
            }
        }

        fun stop(ctx: Context) {
            ctx.stopService(Intent(ctx, CallService::class.java))
        }
    }
}
