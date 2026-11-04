package com.chatv0.agent

import androidx.compose.runtime.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject

/** Owned by the signed-in session, so opening a conversation cannot discard the list. */
@Stable
class CustomerListState(initialCache: String) {
    var customers by mutableStateOf<List<CustomerItem>>(emptyList())
        private set
    var status by mutableStateOf("正在同步客户列表")
        private set
    var deviceStatus by mutableStateOf("设备状态检查中")
        private set
    var refreshing by mutableStateOf(false)
        private set

    init { applyCache(initialCache) }

    fun applyCache(raw: String) {
        if (raw.isBlank()) return
        runCatching { parseCustomerItems(JSONObject(raw)) }.onSuccess { next ->
            val previous = customers.associateBy { it.conversationId }
            // Keep unchanged rows, including their identity, when another customer changes.
            customers = next.map { item -> previous[item.conversationId]?.takeIf { it == item } ?: item }
            status = "共 ${next.size} 个客户"
        }
    }

    suspend fun refresh(prefs: AgentPrefs, url: String, token: String, onAuthExpired: (String) -> Unit) {
        if (refreshing) return
        refreshing = true
        try {
            withContext(Dispatchers.IO) {
                val previous = prefs.cachedCustomers()
                val json = AgentApi(url, token).customersJson().toString()
                // A message/read event arriving during the request must win over an older snapshot.
                prefs.replaceCustomerSnapshot(token, previous, json)
            }
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            if (error is AgentRequestException && error.invalidSession) onAuthExpired(error.requestToken)
            status = "${if (customers.isEmpty()) "" else "共 ${customers.size} 个客户 · "}更新失败，保留本地记录"
        } finally {
            refreshing = false
        }
    }

    suspend fun refreshDevices(prefs: AgentPrefs, url: String, token: String, onAuthExpired: (String) -> Unit) {
        try {
            val root = withContext(Dispatchers.IO) { AgentApi(url, token).devicesJson() }
            val arr = root.optJSONArray("devices")
            if (arr == null || arr.length() == 0) {
                deviceStatus = "设备：无在线记录"
                return
            }
            var active = 0
            var current = ""
            val now = System.currentTimeMillis()
            for (i in 0 until arr.length()) {
                val item = arr.getJSONObject(i)
                val age = syncAgeSeconds(item.optString("last_sync_at"), now)
                if (age in 0..10 || item.optBoolean("ws_connected")) active++
                if (item.optBoolean("is_current")) {
                    current = "本机 ${item.optString("device_id").takeLast(12)} ${if (age >= 0) "${age}秒前" else "未同步"}"
                }
            }
            deviceStatus = "设备：${active}/${arr.length()} 活跃" + if (current.isBlank()) "" else " · $current"
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            if (error is AgentRequestException && error.invalidSession) onAuthExpired(error.requestToken)
            deviceStatus = connectionErrorMessage(error)
        }
    }
}

@Composable
fun rememberCustomerListState(prefs: AgentPrefs, token: String, onStartService: () -> Unit, onAuthExpired: (String) -> Unit): CustomerListState {
    val url = prefs.baseUrl
    val state = remember(url, token) { CustomerListState(prefs.cachedCustomers()) }
    val expire by rememberUpdatedState(onAuthExpired)
    LaunchedEffect(state) {
        onStartService()
        launch { prefs.customerCacheChanges().collect { state.applyCache(it) } }
        // Device diagnostics are separate from the list and never block cached content.
        while (true) {
            state.refreshDevices(prefs, url, token) { expire(it) }
            delay(60000)
        }
    }
    return state
}
