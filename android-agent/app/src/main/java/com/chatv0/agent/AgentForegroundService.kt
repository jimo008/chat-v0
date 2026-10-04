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
    companion object {
        const val ACTION_STOP_RING = "com.chatv0.agent.STOP_RING"
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val client = OkHttpClient.Builder().pingInterval(20, TimeUnit.SECONDS).build()
    private lateinit var prefs: AgentPrefs
    private var webSocket: WebSocket? = null
    private var emergencyRingtone: Ringtone? = null
    private var normalRingtone: Ringtone? = null
    private var syncJob: Job? = null
    private var reconnectJob: Job? = null

    override fun onCreate() {
        super.onCreate()
        prefs = AgentPrefs(this)
        createChannel()
        startForeground(1, notification(text = "客服服务正在运行", ongoing = true, alert = false))
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP_RING) {
            stopEmergencyRing()
            (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).cancel(3)
        }
        if (syncJob?.isActive != true) {
            syncJob = scope.launch {
                bootstrapLastSeq()
                if (webSocket == null) connectWebSocket()
                syncLoop()
            }
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

    override fun onTaskRemoved(rootIntent: Intent?) {
        val restart = PendingIntent.getService(
            this,
            100,
            Intent(this, AgentForegroundService::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        (getSystemService(ALARM_SERVICE) as AlarmManager).set(
            AlarmManager.RTC_WAKEUP,
            System.currentTimeMillis() + 1000,
            restart
        )
        super.onTaskRemoved(rootIntent)
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

    private fun bootstrapLastSeq() {
        if (prefs.lastSeq > 0L) return
        val token = prefs.token
        val baseUrl = prefs.baseUrl
        if (token.isBlank() || baseUrl.isBlank()) return
        val req = Request.Builder()
            .url(baseUrl + "/api/v1/agent/sync?after_seq=0")
            .header("Authorization", "Bearer " + token)
            .build()
        runCatching {
            client.newCall(req).execute().use { resp ->
                if (!resp.isSuccessful) return
                val json = JSONObject(resp.body?.string().orEmpty())
                if (json.has("latest_seq")) prefs.lastSeq = maxOf(prefs.lastSeq, json.optLong("latest_seq", 0L))
            }
        }
    }

    private suspend fun syncLoop() {
        while (scope.isActive) {
            delay(1000)
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
                "MESSAGE_CREATED" -> {
                    val payload = event.optJSONObject("payload") ?: event.optJSONObject("payload_json")
                    prefs.bumpCustomerListVersion()
                    val conversationId = payload?.optLong("conversation_id", 0L) ?: 0L
                    if (payload != null && conversationId > 0L) {
                        prefs.appendCachedConversationMessage(conversationId, payload)
                        val preview = when (payload.optString("type")) {
                            "image" -> "[图片消息]"
                            else -> payload.optString("content")
                        }
                        prefs.updateCachedCustomerPreview(conversationId, preview, if (payload.optString("sender_type") == "customer") 1 else 0, lastMessageAt = payload.optString("created_at"))
                    }
                    if (payload?.optString("sender_type") == "customer") {
                        val content = when (payload.optString("type")) {
                            "image" -> "[图片消息]"
                            else -> payload.optString("content").ifBlank { "新的客户消息" }
                        }
                        notifyNormal(if (conversationId > 0) "#$conversationId $content" else content)
                    }
                }
                "CUSTOMER_READ_UPDATED", "MESSAGE_READ" -> {
                    val payload = event.optJSONObject("payload") ?: event.optJSONObject("payload_json")
                    val conversationId = payload?.optLong("conversation_id", 0L) ?: 0L
                    val readState = payload?.optJSONObject("read_state")
                    if (conversationId > 0L && readState != null) {
                        prefs.updateCachedConversationReadState(conversationId, readState)
                        prefs.bumpCustomerListVersion()
                    }
                }
                "EMERGENCY_STARTED" -> {
                    val payload = event.optJSONObject("payload") ?: event.optJSONObject("payload_json")
                    val conversationId = payload?.optLong("conversation_id", 0L) ?: 0L
                    if (conversationId > 0L) prefs.updateCachedCustomerPreview(conversationId, "", ringingDelta = 1)
                    prefs.bumpCustomerListVersion()
                    val email = payload?.optString("customer_email").orEmpty()
                    notifyEmergency(if (email.isBlank()) "有客户正在紧急呼叫" else "$email 正在紧急呼叫")
                }
                "EMERGENCY_ACCEPTED", "EMERGENCY_CANCELLED", "EMERGENCY_EXPIRED" -> {
                    val payload = event.optJSONObject("payload") ?: event.optJSONObject("payload_json")
                    val conversationId = payload?.optLong("conversation_id", 0L) ?: 0L
                    if (conversationId > 0L) prefs.setCachedCustomerRinging(conversationId, 0)
                    prefs.bumpCustomerListVersion()
                    stopEmergencyRing()
                    (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).cancel(3)
                }
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
        playNormalRing()
        (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).notify(2, notification("agent-popup-v2", text, ongoing = false, emergency = false, alert = true))
    }

    private fun notifyEmergency(text: String) {
        startEmergencyRing()
        (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).notify(3, notification("emergency", text, ongoing = false, emergency = true))
    }

    private fun notification(channelId: String = "agent", text: String, ongoing: Boolean, emergency: Boolean = false, alert: Boolean = false): Notification {
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
            .setCategory(if (emergency || alert) NotificationCompat.CATEGORY_CALL else NotificationCompat.CATEGORY_MESSAGE)
            .setPriority(if (emergency || alert) NotificationCompat.PRIORITY_MAX else NotificationCompat.PRIORITY_HIGH)
            .setVibrate(if (emergency) longArrayOf(0, 700, 300, 700, 300, 700) else longArrayOf(0, 200, 100, 200))
            .setDefaults(Notification.DEFAULT_ALL)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .build()
    }

    private fun createChannel() {
        val manager = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        manager.createNotificationChannel(NotificationChannel("agent", "客服服务", NotificationManager.IMPORTANCE_HIGH))
        manager.createNotificationChannel(NotificationChannel("agent-popup-v2", "客户新消息弹窗", NotificationManager.IMPORTANCE_HIGH).apply {
            description = "客户发送新消息时弹窗和亮屏提醒"
            enableVibration(true)
            vibrationPattern = longArrayOf(0, 300, 120, 300)
            lockscreenVisibility = Notification.VISIBILITY_PUBLIC
        })
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

    private fun playNormalRing() {
        normalRingtone?.stop()
        normalRingtone = RingtoneManager.getRingtone(this, RingtoneManager.getDefaultUri(RingtoneManager.TYPE_NOTIFICATION))?.also { it.play() }
        scope.launch {
            delay(3000)
            normalRingtone?.stop()
            normalRingtone = null
        }
    }
}
