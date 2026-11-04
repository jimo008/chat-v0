package com.chatv0.agent

import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.json.JSONArray
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import java.time.OffsetDateTime
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

class CustomerListCacheTest {
    @get:Rule val compose = createComposeRule()
    private val responseGate = CountDownLatch(1)
    private val server = MockWebServer()
    private lateinit var prefs: AgentPrefs

    @Before fun setup() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                responseGate.await(30, TimeUnit.SECONDS)
                return MockResponse().setHeader("Content-Type", "application/json").setBody("{\"devices\":[]}")
            }
        }
        server.start()
        prefs = AgentPrefs(InstrumentationRegistry.getInstrumentation().targetContext)
        prefs.baseUrl = server.url("/").toString().trimEnd('/')
        prefs.token = "customer-cache-ui-test"
        prefs.setCachedCustomers(fixture())
    }

    @After fun teardown() {
        responseGate.countDown()
        prefs.clearAuth()
        server.shutdown()
    }

    @Test fun returningShowsCachedRowsAndScrollPositionEvenWhileNetworkIsBlocked() {
        lateinit var state: CustomerListState
        lateinit var scroll: LazyListState
        val serviceStarts = AtomicInteger()
        val refreshes = AtomicInteger()
        compose.setContent {
            MaterialTheme {
                state = rememberCustomerListState(prefs, prefs.token, { serviceStarts.incrementAndGet() }) { error("Unexpected auth failure") }
                scroll = rememberLazyListState()
                var opened by remember { mutableStateOf(false) }
                if (opened) {
                    Button(onClick = { opened = false }) { Text("Back to customers") }
                } else {
                    CustomerListScreen(prefs, state, scroll, {}, { opened = true }, { refreshes.incrementAndGet() })
                }
            }
        }
        compose.onNodeWithText("[测试站点] customer1@example.com").assertIsDisplayed()
        compose.onNodeWithTag("customer-list").performScrollToIndex(10)
        var oldIndex = 0
        var oldOffset = 0
        compose.runOnIdle { oldIndex = scroll.firstVisibleItemIndex; oldOffset = scroll.firstVisibleItemScrollOffset }
        repeat(3) {
            compose.onNodeWithTag("customer-11").performClick()
            compose.onNodeWithText("Back to customers").performClick()
            compose.onNodeWithTag("customer-11").assertIsDisplayed()
            compose.onNodeWithText("正在同步客户列表").assertDoesNotExist()
            compose.runOnIdle {
                assertEquals(oldIndex, scroll.firstVisibleItemIndex)
                assertEquals(oldOffset, scroll.firstVisibleItemScrollOffset)
            }
        }
        compose.waitUntil(5000) { server.requestCount == 1 }
        assertEquals("/api/v1/agent/devices", server.takeRequest(1, TimeUnit.SECONDS)?.path)
        assertEquals(1, serviceStarts.get())
        assertEquals(0, refreshes.get())
        assertEquals(1L, responseGate.count) // No network response was needed to display or return.

        compose.onNodeWithTag("customer-list").performScrollToIndex(0)
        lateinit var unchanged: CustomerItem
        compose.runOnIdle { unchanged = state.customers.first { it.conversationId == 2L } }
        prefs.updateCachedCustomerPreview(1, "最新客户消息", unreadDelta = 1, lastMessageAt = "2026-11-04T11:00:00+08:00", customerMessageSeq = 41)
        compose.waitUntil(5000) { state.customers.first().lastMessage == "最新客户消息" }
        compose.onNodeWithText("最新客户消息").assertIsDisplayed()
        compose.runOnIdle {
            assertEquals(1, state.customers.first().unreadCount)
            assertSame(unchanged, state.customers.first { it.conversationId == 2L })
        }
        prefs.setCachedCustomerRinging(1, 1)
        compose.waitUntil(5000) { state.customers.first().ringingCount == 1 }
        compose.onNodeWithText("呼叫 1").assertIsDisplayed()
        prefs.setCachedCustomerRinging(1, 0)
        prefs.markCachedCustomerRead(1, 41)
        compose.waitUntil(5000) { state.customers.first().unreadCount == 0 && state.customers.first().ringingCount == 0 }
        compose.onNodeWithText("呼叫 1").assertDoesNotExist()
        assertEquals(1, server.requestCount)
    }

    @Test fun staleSnapshotsAndReadReceiptsCannotOverwriteANewerCustomerMessage() {
        val previous = prefs.cachedCustomers()
        assertTrue(prefs.updateCachedCustomerPreview(1, "新消息", unreadDelta = 1, customerMessageSeq = 41))
        assertFalse(prefs.replaceCustomerSnapshot(prefs.token, previous, fixture()))
        assertFalse(prefs.replaceCustomerSnapshot("expired-token", prefs.cachedCustomers(), fixture()))
        prefs.markCachedCustomerRead(1, 40)
        assertEquals(1, CustomerListState(prefs.cachedCustomers()).customers.first { it.conversationId == 1L }.unreadCount)
        prefs.markCachedCustomerRead(1, 41)
        assertEquals(0, CustomerListState(prefs.cachedCustomers()).customers.first { it.conversationId == 1L }.unreadCount)
        assertFalse(prefs.updateCachedCustomerPreview(9999, "新客户"))
        val restarted = AgentPrefs(InstrumentationRegistry.getInstrumentation().targetContext)
        assertEquals("新消息", CustomerListState(restarted.cachedCustomers()).customers.first { it.conversationId == 1L }.lastMessage)
    }

    @Test fun catchingUpEventsAlreadyIncludedInASnapshotDoesNotDoubleCountUnreadMessages() {
        val original = CustomerListState(prefs.cachedCustomers()).customers.first()
        assertTrue(prefs.updateCachedCustomerPreview(1, "older event", unreadDelta = 1,
            lastMessageAt = "2026-11-04T09:00:00+08:00", customerMessageSeq = 10, messageSeq = 10))
        assertEquals(original, CustomerListState(prefs.cachedCustomers()).customers.first())
        assertTrue(prefs.updateCachedCustomerPreview(1, "new event", unreadDelta = 1,
            lastMessageAt = "2026-11-04T11:00:00+08:00", customerMessageSeq = 41, messageSeq = 41))
        assertTrue(prefs.updateCachedCustomerPreview(1, "new event", unreadDelta = 1,
            lastMessageAt = "2026-11-04T11:00:00+08:00", customerMessageSeq = 41, messageSeq = 41))
        assertEquals(1, CustomerListState(prefs.cachedCustomers()).customers.first().unreadCount)
    }

    private fun fixture(): String = JSONObject().put("customers", JSONArray().apply {
        for (id in 1..30) put(JSONObject().apply {
            put("id", id); put("conversation_id", id); put("site_name", "测试站点")
            put("email", "customer$id@example.com"); put("last_message", "cached message $id")
            put("last_message_at", OffsetDateTime.parse("2026-11-04T10:00:00+08:00").minusSeconds(id.toLong()).toString())
            put("unread_count", 0); put("ringing_count", 0)
        })
    }).toString()
}
