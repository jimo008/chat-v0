package com.chatv0.agent

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.util.UUID

class AgentPrefs(context: Context) {
    private val prefs = context.getSharedPreferences("agent", Context.MODE_PRIVATE)

    var baseUrl: String
        get() = prefs.getString("base_url", "") ?: ""
        set(value) = prefs.edit().putString("base_url", value).apply()

    var login: String
        get() = prefs.getString("login", "") ?: ""
        set(value) = prefs.edit().putString("login", value).apply()

    var token: String
        get() = prefs.getString("token", "") ?: ""
        set(value) = prefs.edit().putString("token", value).apply()

    var lastSeq: Long
        get() = prefs.getLong("last_seq", 0L)
        set(value) = prefs.edit().putLong("last_seq", value).apply()

    var acceptEmergency: Boolean
        get() = prefs.getBoolean("accept_emergency", false)
        set(value) = prefs.edit().putBoolean("accept_emergency", value).apply()

    var customerListVersion: Long
        get() = prefs.getLong("customer_list_version", 0L)
        set(value) = prefs.edit().putLong("customer_list_version", value).apply()

    val deviceId: String
        get() {
            val existing = prefs.getString("device_id", "") ?: ""
            if (existing.isNotBlank()) return existing
            val generated = "android-" + UUID.randomUUID().toString()
            prefs.edit().putString("device_id", generated).apply()
            return generated
        }

    fun bumpCustomerListVersion() {
        prefs.edit().putLong("customer_list_version", customerListVersion + 1).apply()
    }

    fun cachedCustomers(): String =
        prefs.getString("customers_json", "") ?: ""

    fun setCachedCustomers(json: String) {
        prefs.edit().putString("customers_json", json).apply()
    }

    fun cachedConversation(conversationId: Long): String =
        prefs.getString("conversation_$conversationId", "") ?: ""

    fun setCachedConversation(conversationId: Long, json: String) {
        prefs.edit().putString("conversation_$conversationId", json).apply()
    }

    fun appendCachedConversationMessage(conversationId: Long, message: JSONObject) {
        val raw = cachedConversation(conversationId)
        if (raw.isBlank()) return
        runCatching {
            val root = JSONObject(raw)
            val arr = root.optJSONArray("messages") ?: JSONArray()
            val messageId = message.optLong("id", 0L)
            val clientMsgId = message.optString("client_msg_id").takeIf { it.isNotBlank() && it != "null" }
            for (i in 0 until arr.length()) {
                val existing = arr.getJSONObject(i)
                if (messageId > 0L && existing.optLong("id") == messageId) return
                if (clientMsgId != null && existing.optString("client_msg_id") == clientMsgId) {
                    arr.put(i, message)
                    root.put("messages", arr)
                    setCachedConversation(conversationId, root.toString())
                    return
                }
            }
            arr.put(message)
            root.put("messages", arr)
            setCachedConversation(conversationId, root.toString())
        }
    }

    fun updateCachedCustomerPreview(conversationId: Long, lastMessage: String, unreadDelta: Int = 0, ringingDelta: Int = 0, lastMessageAt: String = nowIso()) {
        val raw = cachedCustomers()
        if (raw.isBlank()) return
        runCatching {
            val root = JSONObject(raw)
            val arr = root.optJSONArray("customers") ?: return
            var updated: JSONObject? = null
            val next = JSONArray()
            for (i in 0 until arr.length()) {
                val item = arr.getJSONObject(i)
                if (item.optLong("conversation_id") == conversationId) {
                    if (lastMessage.isNotBlank()) item.put("last_message", lastMessage)
                    if (lastMessageAt.isNotBlank()) item.put("last_message_at", lastMessageAt)
                    if (unreadDelta != 0) item.put("unread_count", (item.optInt("unread_count") + unreadDelta).coerceAtLeast(0))
                    if (ringingDelta != 0) item.put("ringing_count", (item.optInt("ringing_count") + ringingDelta).coerceAtLeast(0))
                    updated = item
                } else {
                    next.put(item)
                }
            }
            if (updated != null) {
                val sorted = JSONArray()
                sorted.put(updated)
                for (i in 0 until next.length()) sorted.put(next.getJSONObject(i))
                root.put("customers", sorted)
            }
            setCachedCustomers(root.toString())
        }
    }

    fun setCachedCustomerRinging(conversationId: Long, ringingCount: Int) {
        val raw = cachedCustomers()
        if (raw.isBlank()) return
        runCatching {
            val root = JSONObject(raw)
            val arr = root.optJSONArray("customers") ?: return
            for (i in 0 until arr.length()) {
                val item = arr.getJSONObject(i)
                if (item.optLong("conversation_id") == conversationId) {
                    item.put("ringing_count", ringingCount.coerceAtLeast(0))
                    break
                }
            }
            setCachedCustomers(root.toString())
        }
    }

    fun markCachedCustomerRead(conversationId: Long) {
        val raw = cachedCustomers()
        if (raw.isBlank()) return
        runCatching {
            val root = JSONObject(raw)
            val arr = root.optJSONArray("customers") ?: return
            for (i in 0 until arr.length()) {
                val item = arr.getJSONObject(i)
                if (item.optLong("conversation_id") == conversationId) {
                    item.put("unread_count", 0)
                    break
                }
            }
            setCachedCustomers(root.toString())
        }
    }

    fun updateCachedConversationReadState(conversationId: Long, readState: JSONObject) {
        val raw = cachedConversation(conversationId)
        if (raw.isBlank()) return
        runCatching {
            val root = JSONObject(raw)
            root.put("read_state", readState)
            setCachedConversation(conversationId, root.toString())
        }
    }

    fun clearAuth() {
        prefs.edit().remove("token").remove("last_seq").remove("customers_json").apply()
    }
}

private fun nowIso(): String = java.time.OffsetDateTime.now(java.time.ZoneOffset.ofHours(8)).toString()
