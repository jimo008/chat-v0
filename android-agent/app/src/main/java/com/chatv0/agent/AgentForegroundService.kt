package com.chatv0.agent

import android.app.*
import android.content.Intent
import android.media.Ringtone
import android.media.RingtoneManager
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import android.os.SystemClock
import android.net.Uri
import android.os.Bundle
import android.util.Log
import androidx.core.app.NotificationCompat
import kotlinx.coroutines.*
import okhttp3.*
import org.json.JSONObject
import org.json.JSONArray
import java.util.concurrent.TimeUnit

class AgentForegroundService : Service() {
    companion object {
        const val ACTION_STOP_RING = "com.chatv0.agent.STOP_RING"
        const val EXTRA_CONVERSATION_ID = "conversation_id"
        private const val WATCHDOG_REQUEST_CODE = 2001
        private const val WATCHDOG_INTERVAL_MS = 60_000L
        private const val NORMAL_CHANNEL_ID = "agent-popup-v3"
        private const val EMERGENCY_CHANNEL_ID = "emergency-v2"
        private const val UNREAD_REMINDER_ID = 4

        fun scheduleWatchdog(context: android.content.Context) {
            val prefs = AgentPrefs(context)
            if (prefs.token.isBlank() || !prefs.isPairedFor(prefs.baseUrl)) return
            val intent = Intent(context, ServiceWatchdogReceiver::class.java)
            val pendingIntent = PendingIntent.getBroadcast(
                context,
                WATCHDOG_REQUEST_CODE,
                intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
            val alarmManager = context.getSystemService(ALARM_SERVICE) as AlarmManager
            alarmManager.setAndAllowWhileIdle(
                AlarmManager.RTC_WAKEUP,
                System.currentTimeMillis() + WATCHDOG_INTERVAL_MS,
                pendingIntent
            )
        }
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val client = OkHttpClient.Builder().addInterceptor(AgentDeviceProofInterceptor()).pingInterval(20, TimeUnit.SECONDS).build()
    private lateinit var prefs: AgentPrefs
    private var webSocket: WebSocket? = null
    private var emergencyRingtone: Ringtone? = null
    private var syncJob: Job? = null
    private var reconnectJob: Job? = null
    private var wakeLock: PowerManager.WakeLock? = null
    private var sessionToken = ""
    private var reminderSyncToken = ""
    private val reminderClock = UnreadReminderClock()
    private var reminderConversationId = 0L
    @Volatile private var customerRefreshNeeded = true
    private var nextCustomerRefreshAt = 0L

    override fun onCreate() {
        super.onCreate()
        prefs = AgentPrefs(this)
        createChannel()
        if (prefs.token.isNotBlank() && prefs.isPairedFor(prefs.baseUrl)) {
            acquireWakeLock()
            scheduleWatchdog(this)
        }
        startForeground(1, notification(text = "客服服务正在运行", ongoing = true, alert = false))
        scope.launch { unreadReminderLoop() }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (!prefs.isPairedFor(prefs.baseUrl) || prefs.token.isBlank()) {
            stopSelf()
            return START_NOT_STICKY
        }
        if (intent?.action == ACTION_STOP_RING) {
            stopEmergencyRing()
            (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).cancel(3)
        }
        if (sessionToken != prefs.token) {
            sessionToken = prefs.token
            syncJob?.cancel()
            reconnectJob?.cancel()
            webSocket?.cancel()
            webSocket = null
            customerRefreshNeeded = true
            nextCustomerRefreshAt = 0L
        }
        if (syncJob?.isActive != true) {
            syncJob = scope.launch {
                while (isActive && !bootstrapLastSeq()) delay(3000)
                if (!isActive || prefs.token.isBlank()) return@launch
                if (webSocket == null) connectWebSocket()
                syncLoop()
            }
        }
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        scope.cancel()
        notificationManager().cancel(UNREAD_REMINDER_ID)
        webSocket?.close(1000, "service_destroy")
        stopEmergencyRing()
        if (::prefs.isInitialized && prefs.token.isNotBlank()) scheduleWatchdog(this)
        releaseWakeLock()
        super.onDestroy()
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        scheduleWatchdog(this)
        super.onTaskRemoved(rootIntent)
    }

    private fun connectWebSocket() {
        val token = prefs.token
        val baseUrl = prefs.baseUrl
        if (token.isBlank() || baseUrl.isBlank()) return
        val wsUrl = baseUrl.replaceFirst("http", "ws") + "/ws/agent?after_seq=" + prefs.lastSeq
        val req = Request.Builder().url(wsUrl).header("Authorization", "Bearer $token").build()
        webSocket = client.newWebSocket(req, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) { handleEventEnvelope(text, token) }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                if (prefs.token == token && this@AgentForegroundService.webSocket === webSocket) {
                    this@AgentForegroundService.webSocket = null
                    scheduleReconnect()
                }
            }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (prefs.token == token && this@AgentForegroundService.webSocket === webSocket) {
                    this@AgentForegroundService.webSocket = null
                    scheduleReconnect()
                }
            }
        })
    }

    private fun acquireWakeLock() {
        if (wakeLock?.isHeld == true) return
        val powerManager = getSystemService(POWER_SERVICE) as PowerManager
        wakeLock = powerManager.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "chatv0:agent-sync").apply {
            setReferenceCounted(false)
            acquire()
        }
    }

    private fun releaseWakeLock() {
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
    }

    private fun bootstrapLastSeq(): Boolean {
        if (prefs.syncInitialized) return true
        val token = prefs.token
        val baseUrl = prefs.baseUrl
        if (token.isBlank() || baseUrl.isBlank()) return false
        val req = Request.Builder()
            .url(baseUrl + "/api/v1/agent/sync?bootstrap=true")
            .header("Authorization", "Bearer " + token)
            .build()
        return runCatching {
            client.newCall(req).execute().use { resp ->
                if (invalidAgentSession(resp)) {
                    prefs.clearAuthIfToken(token)
                    return@use false
                }
                if (!resp.isSuccessful || prefs.token != token) return@use false
                val json = JSONObject(resp.body?.string().orEmpty())
                if (!json.optBoolean("snapshot") || !json.has("latest_seq")) return@use false
                val customers = AgentApi(baseUrl, token).customersJson()
                prefs.completeSnapshot(token, json.getLong("latest_seq"), customers.toString()).also { complete ->
                    if (complete) customerRefreshNeeded = false
                }
            }
        }.getOrDefault(false)
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
            runCatching {
                client.newCall(req).execute().use { resp ->
                    if (invalidAgentSession(resp)) {
                        prefs.clearAuthIfToken(token)
                        return@use
                    }
                    if (resp.isSuccessful) handleEventEnvelope(resp.body?.string().orEmpty(), token)
                }
            }
            refreshMissingCustomers(baseUrl, token)
        }
    }

    private fun refreshMissingCustomers(baseUrl: String, token: String) {
        if (!customerRefreshNeeded || android.os.SystemClock.elapsedRealtime() < nextCustomerRefreshAt) return
        customerRefreshNeeded = false
        nextCustomerRefreshAt = android.os.SystemClock.elapsedRealtime() + 3000
        val previous = prefs.cachedCustomers()
        runCatching {
            val json = AgentApi(baseUrl, token).customersJson().toString()
            // Do not overwrite a newer message/read event while filling a newly discovered customer.
            if (!prefs.replaceCustomerSnapshot(token, previous, json)) customerRefreshNeeded = true
        }.onFailure {
            customerRefreshNeeded = true
            if (it is AgentRequestException && it.invalidSession) prefs.clearAuthIfToken(it.requestToken)
        }
    }

    private fun handleEventEnvelope(text: String, token: String) {
        prefs.inSession(token) { applyEventEnvelope(text, token) }
    }

    private fun applyEventEnvelope(text: String, token: String) {
        if (!prefs.syncInitialized) return
        val json = runCatching { JSONObject(text) }.getOrNull() ?: return
        val events = json.optJSONArray("events") ?: return
        val cursor = EventCursor(prefs.lastSeq)
        val alerts = loadAlerts()
        for (i in 0 until events.length()) {
            val event = events.getJSONObject(i)
            if (!cursor.accept(event.optLong("seq"))) continue
            when (event.optString("type")) {
                "MESSAGE_CREATED" -> {
                    val payload = event.optJSONObject("payload") ?: event.optJSONObject("payload_json")
                    val conversationId = payload?.optLong("conversation_id", 0L) ?: 0L
                    if (payload != null && conversationId > 0L) {
                        prefs.appendCachedConversationMessage(conversationId, payload)
                        val preview = when (payload.optString("type")) {
                            "image" -> "[图片消息]"
                            else -> payload.optString("content")
                        }
                        val incoming = payload.optString("sender_type") == "customer"
                        val found = prefs.updateCachedCustomerPreview(conversationId, preview,
                            if (incoming && event.optBoolean("notify", false)) 1 else 0,
                            lastMessageAt = payload.optString("created_at"),
                            customerMessageSeq = if (incoming) payload.optLong("seq") else 0L,
                            messageSeq = payload.optLong("seq"))
                        if (!found) customerRefreshNeeded = true
                    }
                    prefs.bumpCustomerListVersion()
                    if (payload?.optString("sender_type") == "customer") {
                        val messageId = payload.optLong("id", 0L)
                        if (messageId > 0L && !prefs.hasNotifiedMessage(messageId) && event.optBoolean("notify", false)) {
                            val content = when (payload.optString("type")) {
                                "image" -> "[图片消息]"
                                else -> payload.optString("content").ifBlank { "新的客户消息" }
                            }
                            alerts.offer(conversationId, messageId, payload.optLong("seq"), content)
                        }
                    }
                }
                "CUSTOMER_READ_UPDATED", "MESSAGE_READ", "AGENT_READ_UPDATED" -> {
                    val payload = event.optJSONObject("payload") ?: event.optJSONObject("payload_json")
                    val conversationId = payload?.optLong("conversation_id", 0L) ?: 0L
                    val readState = payload?.optJSONObject("read_state")
                    if (conversationId > 0L && readState != null) {
                        prefs.updateCachedConversationReadState(conversationId, readState)
                        prefs.bumpCustomerListVersion()
                    }
                    if (event.optString("type") == "AGENT_READ_UPDATED" && conversationId > 0L) {
                        val upTo = payload?.optLong("up_to_seq") ?: 0L
                        prefs.markCachedCustomerRead(conversationId, upTo)
                        alerts.read(conversationId, upTo)
                        cancelReadNotification(conversationId, upTo)
                    }
                }
                "EMERGENCY_STARTED" -> {
                    if (!event.optBoolean("notify", false)) {
                        continue
                    }
                    val payload = event.optJSONObject("payload") ?: event.optJSONObject("payload_json")
                    val conversationId = payload?.optLong("conversation_id", 0L) ?: 0L
                    if (conversationId > 0L) prefs.updateCachedCustomerPreview(conversationId, "", ringingDelta = 1)
                    prefs.bumpCustomerListVersion()
                    val email = payload?.optString("customer_email").orEmpty()
                    if (event.optBoolean("notify", false)) notifyEmergency(if (email.isBlank()) "有客户正在紧急呼叫" else "$email 正在紧急呼叫")
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
        prefs.saveNotificationProgress(cursor.value, encodeAlerts(alerts))
        // Drain catch-up pages before alerting, so newer read receipts can silence old pending items.
        if (events.length() < 100) {
            flushAlerts(alerts)
            reminderSyncToken = token
        } else {
            reminderSyncToken = ""
        }
    }

    private fun scheduleReconnect() {
        if (reconnectJob?.isActive == true) return
        reconnectJob = scope.launch {
            delay(3000)
            if (webSocket == null) connectWebSocket()
        }
    }

    private fun loadAlerts(): CustomerAlerts = runCatching {
        val array = JSONArray(prefs.pendingNotifications())
        CustomerAlerts((0 until array.length()).map { i -> array.getJSONObject(i).let {
            CustomerAlert(it.getLong("conversation_id"), it.getLong("message_id"), it.getLong("message_seq"),
                it.getString("text"), it.optInt("count", 1), it.optBoolean("notified", false))
        } })
    }.getOrElse { CustomerAlerts() }

    private fun encodeAlerts(alerts: CustomerAlerts): String = JSONArray().apply {
        alerts.items().forEach { item -> put(JSONObject().apply {
            put("conversation_id", item.conversationId); put("message_id", item.messageId)
            put("message_seq", item.messageSeq); put("text", item.text); put("count", item.count)
            put("notified", item.notified)
        }) }
    }.toString()

    private fun flushAlerts(alerts: CustomerAlerts) {
        val manager = notificationManager()
        if (!messageNotificationsEnabled(manager)) return
        for (item in alerts.pending()) {
            val lastRead = runCatching { JSONObject(prefs.cachedConversation(item.conversationId)).optJSONObject("read_state")?.optLong("agent_last_seen_seq") ?: 0L }.getOrDefault(0L)
            if (item.messageSeq <= lastRead) {
                alerts.read(item.conversationId, lastRead)
                prefs.saveNotificationProgress(prefs.lastSeq, encodeAlerts(alerts))
                continue
            }
            if (prefs.hasNotifiedMessage(item.messageId)) {
                alerts.delivered(item.conversationId)
                prefs.saveNotificationProgress(prefs.lastSeq, encodeAlerts(alerts))
                continue
            }
            val customer = runCatching {
                val customers = JSONObject(prefs.cachedCustomers()).getJSONArray("customers")
                (0 until customers.length()).map { customers.getJSONObject(it) }
                    .firstOrNull { it.optLong("conversation_id") == item.conversationId }
            }.getOrNull()
            val title = customer?.optString("email").orEmpty().ifBlank { "客户 #${item.conversationId}" }
            val count = maxOf(item.count, customer?.optInt("unread_count") ?: 0)
            val text = if (count > 1) "$count 条未读消息：${item.text}" else item.text
            val posted = runCatching {
                manager.notify("customer:${item.conversationId}", 2, notification(NORMAL_CHANNEL_ID, text, false, alert = true,
                    conversationId = item.conversationId, messageSeq = item.messageSeq, title = title, count = count))
            }.onFailure { Log.w("AgentNotifications", "Unable to post message notification", it) }.isSuccess
            if (!posted) continue
            alerts.delivered(item.conversationId)
            prefs.markNotifiedMessage(item.messageId, encodeAlerts(alerts))
        }
    }

    private fun notificationManager(): NotificationManager =
        getSystemService(NOTIFICATION_SERVICE) as NotificationManager

    private fun messageNotificationsEnabled(manager: NotificationManager): Boolean =
        manager.areNotificationsEnabled() &&
            manager.getNotificationChannel(NORMAL_CHANNEL_ID)?.importance != NotificationManager.IMPORTANCE_NONE

    private suspend fun unreadReminderLoop() {
        while (currentCoroutineContext().isActive) {
            val token = prefs.token
            if (token.isBlank()) {
                updateUnreadReminder(emptySet())
            } else {
                prefs.inSession(token) {
                    if (prefs.acceptEmergency && prefs.syncInitialized && reminderSyncToken == token) {
                        val unread = loadAlerts().items().sortedByDescending { it.messageSeq }
                            .mapTo(linkedSetOf()) { it.conversationId }
                        // The snapshot includes unread messages from before this device began syncing.
                        val cached = runCatching { parseCustomerItems(JSONObject(prefs.cachedCustomers())) }.getOrDefault(emptyList())
                        cached.filter { it.unreadCount > 0 }.forEach { unread.add(it.conversationId) }
                        updateUnreadReminder(unread)
                    } else {
                        updateUnreadReminder(emptySet())
                    }
                }
            }
            delay(1000)
        }
    }

    private fun updateUnreadReminder(unread: Set<Long>) {
        val manager = notificationManager()
        val active = unread.isNotEmpty() && messageNotificationsEnabled(manager)
        if (reminderConversationId != 0L && (!active || reminderConversationId !in unread)) {
            manager.cancel(UNREAD_REMINDER_ID)
            reminderConversationId = 0L
        }
        if (!reminderClock.due(SystemClock.elapsedRealtime(), active)) return
        val conversationId = unread.first()
        val text = if (unread.size == 1) "还有客户消息未读，点击查看" else "还有 ${unread.size} 个会话未读，点击查看"
        runCatching {
            manager.notify(UNREAD_REMINDER_ID, notification(NORMAL_CHANNEL_ID, text, ongoing = false, alert = true,
                conversationId = conversationId, title = "值班未读提醒", count = unread.size))
            reminderConversationId = conversationId
        }.onFailure { Log.w("AgentNotifications", "Unable to post unread reminder", it) }
    }

    private fun cancelReadNotification(conversationId: Long, upToSeq: Long) {
        val manager = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        val tag = "customer:$conversationId"
        runCatching {
            for (active in manager.activeNotifications) {
                if (active.tag == tag && active.notification.extras.getLong("message_seq") <= upToSeq) manager.cancel(tag, 2)
            }
        }.onFailure { Log.w("AgentNotifications", "Unable to cancel read notification", it) }
    }

    private fun notifyEmergency(text: String) {
        startEmergencyRing()
        (getSystemService(NOTIFICATION_SERVICE) as NotificationManager).notify(3, notification(EMERGENCY_CHANNEL_ID, text, ongoing = false, emergency = true))
    }

    private fun notification(channelId: String = "agent", text: String, ongoing: Boolean, emergency: Boolean = false, alert: Boolean = false,
        conversationId: Long = 0, messageSeq: Long = 0, title: String = "Chat V0 客服", count: Int = 0): Notification {
        val intent = Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        if (conversationId > 0) intent.putExtra(EXTRA_CONVERSATION_ID, conversationId).setData(Uri.parse("chatv0://conversation/$conversationId"))
        val pendingIntent = PendingIntent.getActivity(
            this,
            if (conversationId > 0) conversationId.hashCode() else if (emergency) 3 else 2,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val builder = NotificationCompat.Builder(this, channelId)
            .setSmallIcon(android.R.drawable.sym_call_incoming)
            .setContentTitle(if (emergency) "紧急客服呼叫" else title)
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setNumber(count)
            .addExtras(Bundle().apply { putLong("message_seq", messageSeq) })
            .setContentIntent(pendingIntent)
            .setOngoing(ongoing)
            .setAutoCancel(!ongoing)
            .setCategory(if (emergency) NotificationCompat.CATEGORY_CALL else NotificationCompat.CATEGORY_MESSAGE)
            .setPriority(if (emergency || alert) NotificationCompat.PRIORITY_MAX else NotificationCompat.PRIORITY_HIGH)
            .setVibrate(if (emergency) longArrayOf(0, 700, 300, 700, 300, 700) else longArrayOf(0, 200, 100, 200))
            .setDefaults(Notification.DEFAULT_ALL)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
        if (emergency) builder.setTimeoutAfter(60000)
        return builder.build()
    }

    private fun createChannel() {
        val manager = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        manager.createNotificationChannel(NotificationChannel("agent", "客服服务", NotificationManager.IMPORTANCE_LOW).apply {
            description = "保持客服后台连接"
            setShowBadge(false)
        })
        manager.createNotificationChannel(NotificationChannel(NORMAL_CHANNEL_ID, "客户新消息弹窗", NotificationManager.IMPORTANCE_MAX).apply {
            description = "客户发送新消息时弹窗和亮屏提醒"
            enableVibration(true)
            vibrationPattern = longArrayOf(0, 300, 120, 300)
            lockscreenVisibility = Notification.VISIBILITY_PUBLIC
        })
        val emergencyChannel = NotificationChannel(EMERGENCY_CHANNEL_ID, "紧急呼叫", NotificationManager.IMPORTANCE_MAX).apply {
            description = "紧急客服呼叫提醒"
            enableVibration(true)
            vibrationPattern = longArrayOf(0, 700, 300, 700, 300, 700)
            lockscreenVisibility = Notification.VISIBILITY_PUBLIC
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
