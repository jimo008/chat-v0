package com.chatv0.agent

import android.app.*
import android.content.Intent
import android.media.Ringtone
import android.media.RingtoneManager
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.*
import okhttp3.*
import org.json.JSONObject
import java.util.concurrent.TimeUnit

class AgentForegroundService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val client = OkHttpClient.Builder().pingInterval(20, TimeUnit.SECONDS).build()
    private lateinit var prefs: AgentPrefs
    private var webSocket: WebSocket? = null
    private var emergencyRingtone: Ringtone? = null
    private var syncJob: Job? = null
    private var reconnectJob: Job? = null

    override fun onCreate() {
        super.onCreate()
        prefs = AgentPrefs(this)
        createChannel()
        startForeground(1, notification(text = "客服服务正在运行", ongoing = true))
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (webSocket == null) connectWebSocket()
        if (syncJob?.isActive != true) {
            syncJob = scope.launch { syncLoop() }
        }
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        webSocket?.close(1000, "service_destroy")
        stopEmergencyRing()
        scope.cancel()
        super.onDestroy()
    }

    private fun connectWebSocket() {
        val token = prefs.token
        val baseUrl = prefs.baseUrl
        if (token.isBlank() || baseUrl.isBlank()) return
        val wsUrl = baseUrl.replaceFirst("http", "ws") + "/ws/agent?token=" + token + "&after_seq=" + prefs.lastSeq
        val req = Request.Builder().url(wsUrl).build()
        webSocket = client.newWebSocket(req, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) { handleEventEnvelope(text) }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                this@AgentForegroundService.webSocket = null
                scheduleReconnect()
            }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                this@AgentForegroundService.webSocket = null
                scheduleReconnect()
            }
        })
    }

    private suspend fun syncLoop() {
        while (scope.isActive) {
            delay(3000)
            val token = prefs.token
            val baseUrl = prefs.baseUrl
            if (token.isBlank() || baseUrl.isBlank()) continue
            val req = Request.Builder()
                .url(baseUrl + "/api/v1/agent/sync?after_seq=" + prefs.lastSeq)
                .header("Authorization", "Bearer " + token)
                .build()
            runCatching { client.newCall(req).execute().use { resp -> if (resp.isSuccessful) handleEventEnvelope(resp.body?.string().orEmpty()) } }
        }
    }

    private fun handleEventEnvelope(text: String) {
        val json = runCatching { JSONObject(text) }.getOrNull() ?: return
        if (json.has("latest_seq")) prefs.lastSeq = maxOf(prefs.lastSeq, json.optLong("latest_seq", prefs.lastSeq))
        val events = json.optJSONArray("events") ?: return
        for (i in 0 until events.length()) {
            val event = events.getJSONObject(i)
            prefs.lastSeq = maxOf(prefs.lastSeq, event.optLong("seq", prefs.lastSeq))
            when (event.optString("type")) {
                "MESSAGE_CREATED" -> notifyNormal("新的客户消息")
                "EMERGENCY_STARTED" -> notifyEmergency("紧急客服呼叫")
            }
        }
    }

    private fun scheduleReconnect() {
        if (reconnectJob?.isActive == true) return
        reconnectJob = scope.launch {
            delay(3000)
            if (webSocket == null) connectWebSocket()
        }
    }

    private fun notifyNormal(text: String) {
        (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).notify(2, notification("agent", text, ongoing = false, emergency = false))
    }

    private fun notifyEmergency(text: String) {
        startEmergencyRing()
        (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).notify(3, notification("emergency", text, ongoing = false, emergency = true))
    }

    private fun notification(channelId: String = "agent", text: String, ongoing: Boolean, emergency: Boolean = false): Notification {
        val intent = Intent(this, MainActivity::class.java)
        val pendingIntent = PendingIntent.getActivity(
            this,
            if (emergency) 3 else 2,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        return NotificationCompat.Builder(this, channelId)
            .setSmallIcon(android.R.drawable.sym_call_incoming)
            .setContentTitle(if (emergency) "紧急客服呼叫" else "Chat V0 客服")
            .setContentText(text)
            .setContentIntent(pendingIntent)
            .setOngoing(ongoing)
            .setAutoCancel(!ongoing)
            .setCategory(if (emergency) NotificationCompat.CATEGORY_CALL else NotificationCompat.CATEGORY_MESSAGE)
            .setPriority(if (emergency) NotificationCompat.PRIORITY_MAX else NotificationCompat.PRIORITY_HIGH)
            .setVibrate(if (emergency) longArrayOf(0, 700, 300, 700, 300, 700) else longArrayOf(0, 200, 100, 200))
            .setFullScreenIntent(pendingIntent, emergency)
            .build()
    }

    private fun createChannel() {
        val manager = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        manager.createNotificationChannel(NotificationChannel("agent", "客服服务", NotificationManager.IMPORTANCE_HIGH))
        val emergencyChannel = NotificationChannel("emergency", "紧急呼叫", NotificationManager.IMPORTANCE_HIGH).apply {
            description = "紧急客服呼叫提醒"
            enableVibration(true)
            vibrationPattern = longArrayOf(0, 700, 300, 700, 300, 700)
        }
        manager.createNotificationChannel(emergencyChannel)
    }

    private fun startEmergencyRing() {
        stopEmergencyRing()
        emergencyRingtone = RingtoneManager.getRingtone(this, RingtoneManager.getDefaultUri(RingtoneManager.TYPE_RINGTONE))?.also {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) it.isLooping = true
            it.play()
        }
        scope.launch {
            delay(60000)
            stopEmergencyRing()
        }
    }

    private fun stopEmergencyRing() {
        emergencyRingtone?.stop()
        emergencyRingtone = null
    }
}
