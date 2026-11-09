package com.chatv0.agent

import android.app.Notification
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.os.SystemClock
import android.service.notification.StatusBarNotification
import androidx.test.platform.app.InstrumentationRegistry
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.util.concurrent.atomic.AtomicBoolean

class DutyReminderTest {
    @Test fun dutyRepeatsAndTurningOffOrReadingStopsTheReminder() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val prefs = AgentPrefs(context)
        val read = AtomicBoolean(false)
        val service = Intent(context, AgentForegroundService::class.java)
        val conversationId = 71L
        val customerTag = "customer:$conversationId"
        val previousDuty = prefs.acceptEmergency

        MockWebServer().use { server ->
            server.dispatcher = object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    val url = request.requestUrl ?: return MockResponse().setResponseCode(404)
                    val hasRead = read.get()
                    val body = when (url.encodedPath) {
                        "/api/v1/agent/sync" -> {
                            val after = url.queryParameter("after_seq")?.toLongOrNull() ?: 0L
                            val events = JSONArray()
                            if (after < 1L) events.put(JSONObject().apply {
                                put("seq", 1); put("type", "MESSAGE_CREATED"); put("notify", !hasRead)
                                put("payload", JSONObject(mapOf("id" to 110L, "seq" to 110L,
                                    "conversation_id" to conversationId, "sender_type" to "customer",
                                    "type" to "text", "content" to "duty test", "created_at" to "2026-11-09T10:00:00+08:00")))
                            })
                            if (hasRead && after < 2L) events.put(JSONObject().apply {
                                put("seq", 2); put("type", "AGENT_READ_UPDATED")
                                put("payload", JSONObject(mapOf("conversation_id" to conversationId, "up_to_seq" to 110L)))
                            })
                            JSONObject().put("events", events)
                        }
                        "/api/v1/agent/customers" -> customers(conversationId, if (hasRead) 0 else 1)
                        else -> return MockResponse().setResponseCode(404)
                    }
                    return MockResponse().setHeader("Content-Type", "application/json").setBody(body.toString())
                }
            }
            server.start()
            val baseUrl = server.url("/").toString().trimEnd('/')
            try {
                prefs.beginSession(baseUrl, 7, "duty-test-token", "duty-test")
                prefs.markDevicePaired(baseUrl)
                prefs.setCachedCustomers(customers(conversationId, 0).toString())
                prefs.syncInitialized = true
                prefs.acceptEmergency = true
                context.startForegroundService(service)

                val initial = awaitNotification(manager) { it.tag == customerTag }
                val first = awaitNotification(manager) { it.id == 4 }
                val next = awaitNotification(manager) { it.id == 4 && it.postTime > first.postTime }
                assertTrue(next.postTime - first.postTime >= 9_500L)
                assertEquals(0, next.notification.flags and Notification.FLAG_ONLY_ALERT_ONCE)

                prefs.acceptEmergency = false
                await { manager.activeNotifications.none { it.id == 4 } }
                SystemClock.sleep(11_000L)
                assertFalse(manager.activeNotifications.any { it.id == 4 })
                assertEquals(initial.postTime, manager.activeNotifications.single { it.tag == customerTag }.postTime)

                prefs.acceptEmergency = true
                awaitNotification(manager) { it.id == 4 }
                read.set(true)
                await { manager.activeNotifications.none { it.id == 4 || it.tag == customerTag } }
            } finally {
                context.stopService(service)
                prefs.clearAuth()
                prefs.acceptEmergency = previousDuty
                manager.cancel(customerTag, 2)
                manager.cancel(4)
                instrumentation.waitForIdleSync()
            }
        }
    }

    private fun customers(conversationId: Long, unread: Int): JSONObject =
        JSONObject().put("customers", JSONArray().put(JSONObject(mapOf(
            "id" to 7L, "conversation_id" to conversationId, "email" to "duty@example.com",
            "site_name" to "测试站点", "unread_count" to unread
        ))))

    private fun awaitNotification(manager: NotificationManager, match: (StatusBarNotification) -> Boolean): StatusBarNotification {
        var result: StatusBarNotification? = null
        await {
            result = manager.activeNotifications.firstOrNull(match)
            result != null
        }
        return requireNotNull(result)
    }

    private fun await(condition: () -> Boolean) {
        val deadline = SystemClock.elapsedRealtime() + 17_000L
        while (!condition()) {
            assertTrue("Notification state did not update before the deadline", SystemClock.elapsedRealtime() < deadline)
            SystemClock.sleep(100L)
        }
    }
}
