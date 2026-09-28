package com.chatv0.agent

import android.content.Context

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

    fun cachedConversation(conversationId: Long): String =
        prefs.getString("conversation_$conversationId", "") ?: ""

    fun setCachedConversation(conversationId: Long, json: String) {
        prefs.edit().putString("conversation_$conversationId", json).apply()
    }

    fun clearAuth() {
        prefs.edit().remove("token").remove("last_seq").apply()
    }
}
