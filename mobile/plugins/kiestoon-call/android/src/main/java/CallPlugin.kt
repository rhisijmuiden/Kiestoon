package nl.rhis.kiestoon.call

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.webkit.WebView
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging

@InvokeArg
class SecretArgs {
    var secret: String = ""
}

@InvokeArg
class CallUpdateArgs {
    var state: String = ""
    var name: String = ""
    var number: String = ""
}

@TauriPlugin
class CallPlugin(private val activity: Activity) : Plugin(activity) {
    companion object {
        @Volatile private var instance: CallPlugin? = null

        /** Sends an event to the page; false when the app (and so the page) does not run. */
        fun emit(event: String, data: Map<String, String>): Boolean {
            val p = instance ?: return false
            val o = JSObject()
            data.forEach { (k, v) -> o.put(k, v) }
            Handler(Looper.getMainLooper()).post { p.trigger(event, o) }
            return true
        }
    }

    /** True when the app was built with a Firebase configuration, so it can receive push. */
    private var firebaseReady = false

    override fun load(webView: WebView) {
        super.load(webView)
        instance = this
        Calls.channels(activity)
        firebaseReady = initFirebase()
        if (firebaseReady) {
            FirebaseMessaging.getInstance().token.addOnSuccessListener { t ->
                if (t != null && t != Store.token(activity)) {
                    Store.setToken(activity, t)
                    emit("token", mapOf("token" to t))
                }
            }
        }
        askPermissions()
        handle(activity.intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handle(intent)
    }

    private fun initFirebase(): Boolean {
        if (FirebaseApp.getApps(activity).isNotEmpty()) return true
        // The google-services Gradle plugin turns google-services.json into these resources.
        val opts = FirebaseOptions.fromResource(activity) ?: return false
        FirebaseApp.initializeApp(activity, opts)
        return true
    }

    private fun askPermissions() {
        val want = mutableListOf(Manifest.permission.RECORD_AUDIO)
        if (Build.VERSION.SDK_INT >= 33) want.add(Manifest.permission.POST_NOTIFICATIONS)
        val missing = want.filter { ContextCompat.checkSelfPermission(activity, it) != PackageManager.PERMISSION_GRANTED }
        if (missing.isNotEmpty()) ActivityCompat.requestPermissions(activity, missing.toTypedArray(), 4100)
    }

    /** The app was opened from the call screen: show it over the lock screen and pass the action on. */
    private fun handle(intent: Intent?) {
        val action = intent?.getStringExtra(Calls.EXTRA_ACTION) ?: return
        val id = intent.getStringExtra(Calls.EXTRA_ID) ?: ""
        intent.removeExtra(Calls.EXTRA_ACTION)
        if (action == "answer" || action == "show") {
            if (Build.VERSION.SDK_INT >= 27) {
                activity.setShowWhenLocked(true)
                activity.setTurnScreenOn(true)
            }
        }
        if (action == "answer") {
            Calls.endIncoming(activity, id, null)
            // Kept until the page takes it: the call may only arrive once the page has connected.
            Store.setPending(activity, "answer", id)
        }
        emit("callAction", mapOf("action" to action, "id" to id))
    }

    @Command
    fun registration(invoke: Invoke) {
        val o = JSObject()
        o.put("platform", "fcm")
        o.put("token", Store.token(activity))
        o.put("sandbox", false)
        o.put("appId", Store.appId(activity))
        o.put("name", (Build.MANUFACTURER.replaceFirstChar { it.uppercase() } + " " + Build.MODEL).trim())
        o.put("pushReady", firebaseReady)
        invoke.resolve(o)
    }

    @Command
    fun setSecret(invoke: Invoke) {
        val a = invoke.parseArgs(SecretArgs::class.java)
        Store.setSecret(activity, a.secret)
        invoke.resolve()
    }

    /** What the softphone in the page does, for the notifications and the call service. */
    @Command
    fun callUpdate(invoke: Invoke) {
        val a = invoke.parseArgs(CallUpdateArgs::class.java)
        when (a.state) {
            "active" -> {
                Calls.endIncoming(activity, null, null)
                CallService.start(activity, a.name, a.number)
                if (Build.VERSION.SDK_INT >= 27) activity.setShowWhenLocked(true)
            }
            "ended" -> {
                Calls.endIncoming(activity, null, null)
                CallService.stop(activity)
                if (Build.VERSION.SDK_INT >= 27) activity.setShowWhenLocked(false)
            }
        }
        invoke.resolve()
    }

    @Command
    fun takePending(invoke: Invoke) {
        val p = Store.pending(activity)
        Store.clearPending(activity)
        val o = JSObject()
        if (p != null) {
            o.put("action", p.optString("action"))
            o.put("id", p.optString("id"))
        }
        invoke.resolve(o)
    }
}
