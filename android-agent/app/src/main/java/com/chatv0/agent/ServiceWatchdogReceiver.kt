package com.chatv0.agent

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.core.content.ContextCompat

class ServiceWatchdogReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val prefs = AgentPrefs(context)
        if (prefs.token.isBlank()) return
        AgentForegroundService.scheduleWatchdog(context)
        runCatching { ContextCompat.startForegroundService(context, Intent(context, AgentForegroundService::class.java)) }
    }
}
