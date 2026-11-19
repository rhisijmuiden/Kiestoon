package nl.rhis.kiestoon.call

import android.content.Context
import android.util.Base64
import org.json.JSONObject
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/** What the app keeps between starts: its id, the push token and the key of the notifications. */
internal object Store {
    private fun prefs(ctx: Context) = ctx.getSharedPreferences("kiestoon-call", Context.MODE_PRIVATE)

    fun appId(ctx: Context): String {
        val p = prefs(ctx)
        p.getString("appId", null)?.let { return it }
        val id = UUID.randomUUID().toString()
        p.edit().putString("appId", id).apply()
        return id
    }

    fun token(ctx: Context): String = prefs(ctx).getString("token", "") ?: ""
    fun setToken(ctx: Context, t: String) = prefs(ctx).edit().putString("token", t).apply()

    fun secret(ctx: Context): String = prefs(ctx).getString("secret", "") ?: ""
    fun setSecret(ctx: Context, s: String) = prefs(ctx).edit().putString("secret", s).apply()

    /** An answer or decline pressed before the page could take it: {action, id, at}. */
    fun pending(ctx: Context): JSONObject? {
        val raw = prefs(ctx).getString("pending", null) ?: return null
        val o = try { JSONObject(raw) } catch (e: Exception) { return null }
        // Older than a minute: the call is long gone.
        if (System.currentTimeMillis() - o.optLong("at") > 60_000) return null
        return o
    }

    fun setPending(ctx: Context, action: String, id: String) {
        val o = JSONObject().put("action", action).put("id", id).put("at", System.currentTimeMillis())
        prefs(ctx).edit().putString("pending", o.toString()).apply()
    }

    fun clearPending(ctx: Context) = prefs(ctx).edit().remove("pending").apply()

    /**
     * Decrypts the content of a notification: AES-256-GCM with the key from the server, nonce in
     * front, "kiestoon-push-data" as additional data (see server/internal/push).
     */
    fun open(ctx: Context, data: String?): JSONObject? {
        if (data.isNullOrEmpty()) return null
        val secret = secret(ctx)
        if (secret.isEmpty()) return null
        return try {
            val key = Base64.decode(secret, Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP)
            val raw = Base64.decode(data, Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP)
            val c = Cipher.getInstance("AES/GCM/NoPadding")
            c.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, raw, 0, 12))
            c.updateAAD("kiestoon-push-data".toByteArray())
            JSONObject(String(c.doFinal(raw, 12, raw.size - 12), Charsets.UTF_8))
        } catch (e: Exception) {
            null
        }
    }
}
