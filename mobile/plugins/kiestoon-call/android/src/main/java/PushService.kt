package nl.rhis.kiestoon.call

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage

/**
 * Receives the notifications of the push relay (high priority data messages): k = kind, id, e =
 * encrypted content. Shows the call screen or a notification and tells the page when it runs.
 */
class PushService : FirebaseMessagingService() {
    override fun onNewToken(token: String) {
        Store.setToken(this, token)
        CallPlugin.emit("token", mapOf("token" to token))
    }

    override fun onMessageReceived(msg: RemoteMessage) {
        val kind = msg.data["k"] ?: return
        val id = msg.data["id"] ?: ""
        val data = Store.open(this, msg.data["e"])
        when (kind) {
            "call" -> {
                // Without the key (not linked any more) the call cannot be shown.
                if (data == null) return
                Calls.showIncoming(this, id, data.optString("fromName"), data.optString("from"))
            }
            "cancel" -> Calls.endIncoming(this, id, data?.optString("reason") ?: "missed")
            "voicemail" -> Calls.showVoicemail(this, data?.optString("from") ?: "", data?.optString("summary") ?: "", data?.optInt("new") ?: 0)
            "test" -> Calls.showTest(this)
        }
        CallPlugin.emit("push", mapOf("kind" to kind, "id" to id, "data" to (data?.toString() ?: "")))
    }
}
