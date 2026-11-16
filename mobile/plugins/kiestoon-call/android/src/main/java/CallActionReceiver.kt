package nl.rhis.kiestoon.call

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/** Decline on the call screen, and hang up on the notification of a call in progress. */
class CallActionReceiver : BroadcastReceiver() {
    override fun onReceive(ctx: Context, intent: Intent) {
        val action = intent.getStringExtra(Calls.EXTRA_ACTION) ?: return
        val id = intent.getStringExtra(Calls.EXTRA_ID) ?: ""
        if (action == "decline") Calls.endIncoming(ctx, id, null)
        if (!CallPlugin.emit("callAction", mapOf("action" to action, "id" to id))) {
            // The page is not loaded: it takes the decline when it starts (and the server stops ringing).
            Store.setPending(ctx, action, id)
        }
        if (action == "end") CallService.stop(ctx)
    }
}
