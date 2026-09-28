package com.chatv0.agent

import android.app.*
import android.content.Intent
import android.os.IBinder
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.*
import okhttp3.*
import org.json.JSONObject
import java.util.concurrent.TimeUnit

class AgentForegroundService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val client = OkHttpClient.Builder().pingInterval(20, TimeUnit.SECONDS).build()
    private var webSocket: WebSocket? = null

    override fun onCreate() {
        super.onCreate()
        createChannel()
        startForeground(1, notification("客服服务正在运行"))
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        scope.launch {
            loginIfNeeded()
            connectWebSocket()
            syncLoop()
        }
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        webSocket?.close(1000, "service_destroy")
        scope.cancel()
        super.onDestroy()
    }

    private suspend fun loginIfNeeded() {
        if (AgentSession.token.isNotBlank() || AgentSession.baseUrl.isBlank()) return
        val body = RequestBody.create(MediaType.get("application/json"), JSONObject(mapOf(
            "login" to AgentSession.login,
            "password" to AgentSession.password,
            "device_id" to AgentSession.deviceId
        )).toString())
        val req = Request.Builder().url(AgentSession.baseUrl + "/api/v1/agent/login").post(body).build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) return
            val json = JSONObject(resp.body()?.string().orEmpty())
            AgentSession.token = json.getString("token")
        }
    }

    private fun connectWebSocket() {
        if (AgentSession.token.isBlank()) return
        val wsUrl = AgentSession.baseUrl.replaceFirst("http", "ws") + "/ws/agent?token=" + AgentSession.token + "&after_seq=" + AgentSession.lastSeq
        val req = Request.Builder().url(wsUrl).build()
        webSocket = client.newWebSocket(req, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) { handleEventEnvelope(text) }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) { scheduleReconnect() }
        })
    }

    private suspend fun syncLoop() {
        while (scope.isActive) {
            delay(3000)
            if (AgentSession.token.isBlank()) continue
            val req = Request.Builder()
                .url(AgentSession.baseUrl + "/api/v1/agent/sync?after_seq=" + AgentSession.lastSeq)
                .header("Authorization", "Bearer " + AgentSession.token)
                .build()
            runCatching {
                client.newCall(req).execute().use { resp -> if (resp.isSuccessful) handleEventEnvelope(resp.body()?.string().orEmpty()) }
            }
        }
    }

    private fun handleEventEnvelope(text: String) {
        val json = runCatching { JSONObject(text) }.getOrNull() ?: return
        if (json.has("latest_seq")) AgentSession.lastSeq = maxOf(AgentSession.lastSeq, json.optLong("latest_seq", AgentSession.lastSeq))
        val events = json.optJSONArray("events") ?: return
        for (i in 0 until events.length()) {
            val event = events.getJSONObject(i)
            AgentSession.lastSeq = maxOf(AgentSession.lastSeq, event.optLong("seq", AgentSession.lastSeq))
            when (event.optString("type")) {
                "MESSAGE_CREATED" -> notifyNormal("新的客户消息")
                "EMERGENCY_STARTED" -> notifyEmergency("紧急客服呼叫")
            }
        }
    }

    private fun scheduleReconnect() { scope.launch { delay(3000); connectWebSocket() } }
    private fun notifyNormal(text: String) { (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).notify(2, notification(text)) }
    private fun notifyEmergency(text: String) { (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).notify(3, notification(text)) }
    private fun notification(text: String): Notification = NotificationCompat.Builder(this, "agent")
        .setSmallIcon(android.R.drawable.sym_call_incoming).setContentTitle("Chat V0 客服").setContentText(text).setOngoing(true).build()
    private fun createChannel() { (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).createNotificationChannel(NotificationChannel("agent", "客服服务", NotificationManager.IMPORTANCE_HIGH)) }
}
