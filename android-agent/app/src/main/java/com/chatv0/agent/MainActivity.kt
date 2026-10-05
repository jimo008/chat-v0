package com.chatv0.agent

import android.Manifest
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.compose.BackHandler
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.core.content.ContextCompat
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.File
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.util.UUID

class MainActivity : ComponentActivity() {
    private lateinit var prefs: AgentPrefs

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        prefs = AgentPrefs(this)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1001)
        }
        setContent { AgentApp(prefs) { ContextCompat.startForegroundService(this, Intent(this, AgentForegroundService::class.java)) } }
    }

    override fun onResume() {
        super.onResume()
        ContextCompat.startForegroundService(this, Intent(this, AgentForegroundService::class.java).setAction(AgentForegroundService.ACTION_STOP_RING))
        runCatching { AgentApi(prefs.baseUrl, prefs.token).setForeground(true) }
    }

    override fun onPause() {
        runCatching { AgentApi(prefs.baseUrl, prefs.token).setForeground(false) }
        super.onPause()
    }
}

@Composable
fun AgentApp(prefs: AgentPrefs, onStartService: () -> Unit) {
    var token by remember { mutableStateOf(prefs.token) }
    var selected by remember { mutableStateOf<CustomerItem?>(null) }
    MaterialTheme {
        if (token.isBlank()) {
            LoginScreen(prefs) { newToken -> token = newToken; onStartService() }
        } else if (selected == null) {
            CustomerListScreen(prefs, onLogout = { prefs.clearAuth(); token = "" }, onSelect = { selected = it }, onStartService = onStartService, onAuthExpired = { prefs.clearAuth(); token = "" })
        } else {
            ConversationScreen(prefs, selected!!, onBack = { selected = null })
        }
    }
}

@Composable
fun LoginScreen(prefs: AgentPrefs, onLoggedIn: (String) -> Unit) {
    val scope = rememberCoroutineScope()
    var baseUrl by remember { mutableStateOf(prefs.baseUrl) }
    var login by remember { mutableStateOf(prefs.login) }
    var password by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("未登录") }

    Column(Modifier.fillMaxSize().padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("Chat V0 客服端", style = MaterialTheme.typography.headlineSmall)
        OutlinedTextField(baseUrl, { baseUrl = it }, label = { Text("服务器地址") }, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(login, { login = it }, label = { Text("用户名或邮箱") }, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(password, { password = it }, label = { Text("密码") }, modifier = Modifier.fillMaxWidth())
        Text("本机设备：${prefs.deviceId.takeLast(12)}", style = MaterialTheme.typography.bodySmall, color = Color(0xFF64748B))
        Button(onClick = {
            scope.launch {
                status = "登录中..."
                runCatching {
                    withContext(Dispatchers.IO) {
                        val cleanBase = baseUrl.trimEnd('/')
                        val token = AgentApi(cleanBase).login(login, password, prefs.deviceId)
                        val me = AgentApi(cleanBase, token).me()
                        prefs.baseUrl = cleanBase
                        prefs.login = login
                        prefs.lastSeq = 0L
                        prefs.token = token
                        token to (me.optJSONObject("device")?.optString("device_id").orEmpty())
                    }
                }.onSuccess { (newToken, serverDevice) ->
                    status = if (serverDevice.isBlank()) "登录成功" else "登录成功，设备 ${serverDevice.takeLast(12)}"
                    onLoggedIn(newToken)
                }
                    .onFailure { status = it.message ?: "登录失败" }
            }
        }, modifier = Modifier.fillMaxWidth()) { Text("登录并启动客服服务") }
        Text(status)
        Text("请在系统设置中允许通知、后台运行、自启动和电池无限制。", style = MaterialTheme.typography.bodySmall)
        PermissionButtons()
    }
}

@Composable
fun CustomerListScreen(prefs: AgentPrefs, onLogout: () -> Unit, onSelect: (CustomerItem) -> Unit, onStartService: () -> Unit, onAuthExpired: () -> Unit) {
    val scope = rememberCoroutineScope()
    var customers by remember { mutableStateOf<List<CustomerItem>>(emptyList()) }
    var status by remember { mutableStateOf("加载中") }
    var deviceStatus by remember { mutableStateOf("设备状态检查中") }
    var duty by remember { mutableStateOf(prefs.acceptEmergency) }
    var seenListVersion by remember { mutableStateOf(prefs.customerListVersion) }

    fun refreshDevices() {
        scope.launch {
            runCatching {
                withContext(Dispatchers.IO) { AgentApi(prefs.baseUrl, prefs.token).devicesJson() }
            }.onSuccess { root ->
                val arr = root.optJSONArray("devices")
                if (arr == null || arr.length() == 0) {
                    deviceStatus = "设备：无在线记录"
                    return@onSuccess
                }
                var active = 0
                var current = ""
                val now = System.currentTimeMillis()
                for (i in 0 until arr.length()) {
                    val item = arr.getJSONObject(i)
                    val lastSync = item.optString("last_sync_at")
                    val age = syncAgeSeconds(lastSync, now)
                    if (age in 0..10 || item.optBoolean("ws_connected")) active++
                    if (item.optBoolean("is_current")) {
                        current = "本机 ${item.optString("device_id").takeLast(12)} ${if (age >= 0) "${age}秒前" else "未同步"}"
                    }
                }
                deviceStatus = "设备：${active}/${arr.length()} 活跃" + if (current.isBlank()) "" else " · $current"
            }.onFailure {
                if (AgentApi(prefs.baseUrl, prefs.token).isUnauthorized(it)) onAuthExpired()
                deviceStatus = "设备状态获取失败"
            }
        }
    }

    fun refresh() {
        scope.launch {
            runCatching {
                withContext(Dispatchers.IO) {
                    val api = AgentApi(prefs.baseUrl, prefs.token)
                    val json = api.customersJson()
                    prefs.setCachedCustomers(json.toString())
                    api.parseCustomers(json)
                }
            }
                .onSuccess { customers = it; seenListVersion = prefs.customerListVersion; status = "共 ${it.size} 个客户" }
                .onFailure { status = it.message ?: "加载失败" }
        }
    }
    LaunchedEffect(Unit) {
        onStartService()
        val authOk = withContext(Dispatchers.IO) {
            runCatching { AgentApi(prefs.baseUrl, prefs.token).me() }.isSuccess
        }
        if (!authOk) {
            status = "登录已失效，请重新登录"
            onAuthExpired()
            return@LaunchedEffect
        }
        val cached = prefs.cachedCustomers()
        if (cached.isNotBlank()) {
            runCatching { AgentApi(prefs.baseUrl, prefs.token).parseCustomers(JSONObject(cached)) }
                .onSuccess { customers = it; status = "共 ${it.size} 个客户" }
        }
        while (true) {
            val currentVersion = prefs.customerListVersion
            if (currentVersion != seenListVersion || customers.isEmpty()) {
                val latestCached = prefs.cachedCustomers()
                if (latestCached.isNotBlank()) {
                    runCatching { AgentApi(prefs.baseUrl, prefs.token).parseCustomers(JSONObject(latestCached)) }
                        .onSuccess { customers = it; status = "共 ${it.size} 个客户"; seenListVersion = currentVersion }
                }
                refresh()
                refreshDevices()
            }
            delay(1000)
        }
    }
    LaunchedEffect(Unit) {
        while (true) {
            delay(60000)
            refresh()
            refreshDevices()
        }
    }

    Column(Modifier.fillMaxSize().padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("客户列表", style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
            Button(onClick = { refresh() }) { Text("刷新") }
            OutlinedButton(onClick = onLogout) { Text("退出") }
        }
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("紧急值班", modifier = Modifier.weight(1f))
            Switch(checked = duty, onCheckedChange = {
                duty = it; prefs.acceptEmergency = it
                scope.launch { withContext(Dispatchers.IO) { runCatching { AgentApi(prefs.baseUrl, prefs.token).setEmergencyDuty(it) } } }
            })
        }
        PermissionButtons()
        Text(status, style = MaterialTheme.typography.bodySmall)
        Text(deviceStatus, style = MaterialTheme.typography.bodySmall, color = Color(0xFF475569))
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(customers) { c ->
                Card(Modifier.fillMaxWidth().clickable { onSelect(c) }) {
                    Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text("[${c.siteName}] ${c.email}", fontWeight = FontWeight.SemiBold)
                            Text(lastPreview(c), style = MaterialTheme.typography.bodySmall, color = Color(0xFF475569))
                            Text("conversation #${c.conversationId}" + if (c.blocked) " · 已限制" else "", style = MaterialTheme.typography.labelSmall, color = Color(0xFF64748B))
                        }
                        if (c.ringingCount > 0) {
                            RedBadge("呼叫 ${c.ringingCount}")
                        } else if (c.unreadCount > 0) {
                            RedBadge(c.unreadCount.toString())
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun PermissionButtons() {
    val context = LocalContext.current
    val notificationPermissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}
    Column(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
        Button(onClick = {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            } else {
                openAppSettings(context)
            }
        }, modifier = Modifier.fillMaxWidth()) { Text("允许通知") }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
            OutlinedButton(onClick = { openAppSettings(context) }, modifier = Modifier.weight(1f)) { Text("权限设置") }
            OutlinedButton(onClick = { openBatterySettings(context) }, modifier = Modifier.weight(1f)) { Text("后台保活") }
        }
    }
}

fun openAppSettings(context: android.content.Context) {
    val intent = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS)
        .setData(Uri.parse("package:${context.packageName}"))
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    context.startActivity(intent)
}

fun openFullScreenSettings(context: android.content.Context) {
    if (Build.VERSION.SDK_INT >= 34) {
        val intent = Intent(Settings.ACTION_MANAGE_APP_USE_FULL_SCREEN_INTENT)
            .setData(Uri.parse("package:${context.packageName}"))
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        runCatching { context.startActivity(intent) }.onFailure { openAppSettings(context) }
        return
    }
    openAppSettings(context)
}

fun openBatterySettings(context: android.content.Context) {
    val powerManager = context.getSystemService(android.content.Context.POWER_SERVICE) as PowerManager
    if (!powerManager.isIgnoringBatteryOptimizations(context.packageName)) {
        val intent = Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS)
            .setData(Uri.parse("package:${context.packageName}"))
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        runCatching { context.startActivity(intent) }.onSuccess { return }
    }
    val fallback = Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    runCatching { context.startActivity(fallback) }.onFailure { openAppSettings(context) }
}

@Composable
fun ConversationScreen(prefs: AgentPrefs, customer: CustomerItem, onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    var detail by remember { mutableStateOf<ConversationDetail?>(null) }
    var localMessages by remember { mutableStateOf<List<ChatMessage>>(emptyList()) }
    var input by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("加载中") }
    var loadingOlder by remember { mutableStateOf(false) }
    var didInitialScroll by remember(customer.conversationId) { mutableStateOf(false) }
    var seenCacheVersion by remember(customer.conversationId) { mutableStateOf(prefs.customerListVersion) }
    val listState = rememberLazyListState()
    BackHandler { onBack() }

    fun scrollToLatest() {
        scope.launch {
            delay(80)
            if (localMessages.isNotEmpty()) {
                listState.animateScrollToItem(localMessages.lastIndex)
            }
        }
    }

    fun refresh() {
        scope.launch {
            runCatching { withContext(Dispatchers.IO) {
                val api = AgentApi(prefs.baseUrl, prefs.token)
                val afterSeq = localMessages.filter { it.seq > 0 }.maxOfOrNull { it.seq } ?: 0L
                val json = if (afterSeq > 0L) {
                    api.conversationJson(customer.conversationId, afterSeq = afterSeq)
                } else {
                    api.conversationJson(customer.conversationId)
                }
                api.parseConversation(json)
            } }
                .onSuccess { latest ->
                    val merged = if (localMessages.isEmpty()) latest.messages else mergeMessages(localMessages + latest.messages)
                    detail = latest.copy(messages = merged)
                    localMessages = merged
                    prefs.setCachedConversation(customer.conversationId, conversationCacheJson(latest, merged))
                    status = ""
                    withContext(Dispatchers.IO) {
                        runCatching { AgentApi(prefs.baseUrl, prefs.token).markRead(customer.conversationId) }
                        prefs.markCachedCustomerRead(customer.conversationId)
                    }
                }
                .onFailure { status = it.message ?: "加载失败" }
        }
    }
    fun loadOlder() {
        val firstSeq = localMessages.firstOrNull { it.seq > 0 }?.seq ?: return
        if (loadingOlder) return
        scope.launch {
            loadingOlder = true
            runCatching {
                withContext(Dispatchers.IO) {
                    AgentApi(prefs.baseUrl, prefs.token).parseConversation(
                        AgentApi(prefs.baseUrl, prefs.token).conversationJson(customer.conversationId, firstSeq)
                    )
                }
            }.onSuccess { older ->
                val beforeCount = localMessages.size
                val merged = mergeMessages(older.messages + localMessages)
                val incoming = merged.take((merged.size - beforeCount).coerceAtLeast(0))
                if (incoming.isNotEmpty()) {
                    localMessages = merged
                    listState.scrollToItem(incoming.size)
                }
            }.onFailure { status = it.message ?: "加载更早消息失败" }
            loadingOlder = false
        }
    }
    LaunchedEffect(customer.conversationId) {
        val cached = prefs.cachedConversation(customer.conversationId)
        if (cached.isNotBlank()) {
            runCatching { AgentApi(prefs.baseUrl, prefs.token).parseConversation(JSONObject(cached)) }
                .onSuccess { detail = it; localMessages = it.messages; status = "" }
        }
        refresh()
    }
    LaunchedEffect(customer.conversationId) {
        while (true) {
            delay(1000)
            val currentVersion = prefs.customerListVersion
            if (currentVersion != seenCacheVersion) {
                seenCacheVersion = currentVersion
                val cached = prefs.cachedConversation(customer.conversationId)
                if (cached.isNotBlank()) {
                    runCatching { AgentApi(prefs.baseUrl, prefs.token).parseConversation(JSONObject(cached)) }
                        .onSuccess { cachedDetail ->
                            detail = cachedDetail
                            localMessages = mergeMessages(localMessages + cachedDetail.messages)
                        }
                }
            }
        }
    }
    LaunchedEffect(listState) {
        snapshotFlow { listState.firstVisibleItemIndex }
            .map { it == 0 && didInitialScroll && localMessages.isNotEmpty() }
            .distinctUntilChanged()
            .filter { it }
            .collect { loadOlder() }
    }
    val imageLauncher = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri: Uri? ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            status = "图片上传中..."
            runCatching {
                withContext(Dispatchers.IO) {
                    AgentApi(prefs.baseUrl, prefs.token).uploadImage(context, customer.conversationId, uri)
                }
            }.onSuccess { refresh(); scrollToLatest() }
                .onFailure { status = it.message ?: "图片上传失败" }
        }
    }

    fun sendText(text: String, localKey: String = UUID.randomUUID().toString()) {
        val existing = localMessages.any { it.localKey == localKey }
        if (existing) {
            localMessages = localMessages.map { if (it.localKey == localKey) it.copy(localStatus = "sending") else it }
        } else {
            val pending = ChatMessage(-System.currentTimeMillis(), "agent", 0, "text", text, null, null, "", localKey, "sending", localKey)
            localMessages = localMessages + pending
            scrollToLatest()
        }
        scope.launch {
            runCatching {
                withContext(Dispatchers.IO) { AgentApi(prefs.baseUrl, prefs.token).sendMessage(customer.conversationId, text, localKey) }
            }.onSuccess { sent ->
                localMessages = mergeMessages(localMessages.map { if (it.localKey == localKey || it.clientMsgId == localKey) sent else it })
                val mergedDetail = detail ?: ConversationDetail(customer.conversationId, customer.email, customer.siteName, customer.blocked, localMessages)
                detail = mergedDetail.copy(messages = localMessages)
                prefs.setCachedConversation(customer.conversationId, conversationCacheJson(mergedDetail, localMessages))
                prefs.updateCachedCustomerPreview(customer.conversationId, text, lastMessageAt = sent.createdAt)
                prefs.bumpCustomerListVersion()
                scrollToLatest()
            }
                .onFailure {
                    localMessages = localMessages.map { if (it.localKey == localKey) it.copy(localStatus = "failed") else it }
                }
        }
    }
    LaunchedEffect(localMessages.size, didInitialScroll) {
        if (localMessages.isNotEmpty() && !didInitialScroll) {
            listState.scrollToItem(localMessages.lastIndex)
            didInitialScroll = true
        }
    }

    Column(Modifier.fillMaxSize().padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = onBack) { Text("返回") }
            Column(Modifier.weight(1f)) {
                Text(customer.email, fontWeight = FontWeight.SemiBold)
                Text(customer.siteName, style = MaterialTheme.typography.bodySmall)
            }
            Button(onClick = { refresh() }) { Text("刷新") }
        }
        if (status.isNotBlank()) Text(status)
        LazyColumn(Modifier.weight(1f), state = listState, verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(localMessages, key = { it.localKey }) { m ->
                MessageBubble(m, prefs, customerLastSeenSeq = detail?.customerLastSeenSeq ?: 0L, onRetry = { sendText(m.content, m.localKey) })
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(input, { input = it }, modifier = Modifier.weight(1f), placeholder = { Text("输入回复") })
            OutlinedButton(onClick = { imageLauncher.launch("image/*") }) { Text("图片") }
            Button(onClick = {
                val text = input.trim(); if (text.isBlank()) return@Button
                input = ""
                sendText(text)
            }) { Text("发送") }
        }
    }
}

@Composable
fun RedBadge(text: String) {
    Surface(color = Color(0xFFDC2626), contentColor = Color.White, shape = MaterialTheme.shapes.small) {
        Text(text, modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp), style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.Bold)
    }
}

fun lastPreview(customer: CustomerItem): String {
    if (customer.ringingCount > 0) return "正在紧急呼叫"
    if (customer.lastMessage.isBlank()) return "暂无消息"
    return customer.lastMessage
}

@Composable
fun MessageBubble(message: ChatMessage, prefs: AgentPrefs, customerLastSeenSeq: Long, onRetry: () -> Unit) {
    val isMine = message.senderType == "agent"
    Row(Modifier.fillMaxWidth(), horizontalArrangement = if (isMine) Arrangement.End else Arrangement.Start) {
        Column(horizontalAlignment = if (isMine) Alignment.End else Alignment.Start, modifier = Modifier.fillMaxWidth(0.82f)) {
            Card(colors = CardDefaults.cardColors(containerColor = if (isMine) Color(0xFF2563EB) else Color.White)) {
                Column(Modifier.padding(10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (message.type == "image" && message.attachmentId != null) {
                        AttachmentImage(prefs, message.attachmentId)
                    } else {
                        Text(message.content, color = if (isMine) Color.White else Color(0xFF111827))
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                        Text(formatTime(message.createdAt), style = MaterialTheme.typography.labelSmall, color = if (isMine) Color(0xFFE0E7FF) else Color(0xFF64748B))
                        if (isMine) {
                            val state = when {
                                message.localStatus == "sending" -> "发送中..."
                                message.localStatus == "failed" -> "发送失败"
                                message.seq > 0 && message.seq <= customerLastSeenSeq -> "已读"
                                else -> "已发送"
                            }
                            Text(state, style = MaterialTheme.typography.labelSmall, color = if (message.localStatus == "failed") Color(0xFFFFCDD2) else Color(0xFFE0E7FF))
                        }
                    }
                }
            }
            if (message.localStatus == "failed") {
                TextButton(onClick = onRetry) { Text("重发") }
            }
        }
    }
}

@Composable
fun AttachmentImage(prefs: AgentPrefs, attachmentId: Long) {
    val context = LocalContext.current
    var bitmap by remember(attachmentId) { mutableStateOf<Bitmap?>(null) }
    var failed by remember(attachmentId) { mutableStateOf(false) }
    var preview by remember(attachmentId) { mutableStateOf(false) }
    LaunchedEffect(attachmentId) {
        runCatching {
            withContext(Dispatchers.IO) {
                val cacheFile = attachmentCacheFile(context, attachmentId)
                val bytes = if (cacheFile.exists() && cacheFile.length() > 0) {
                    cacheFile.readBytes()
                } else {
                    val downloaded = AgentApi(prefs.baseUrl, prefs.token).attachmentBytes(attachmentId)
                    cacheFile.parentFile?.mkdirs()
                    cacheFile.writeBytes(downloaded)
                    downloaded
                }
                BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
            }
        }.onSuccess { bitmap = it }.onFailure { failed = true }
    }
    when {
        bitmap != null -> {
            Image(
                bitmap!!.asImageBitmap(),
                contentDescription = "图片消息",
                modifier = Modifier
                    .sizeIn(maxWidth = 180.dp, maxHeight = 180.dp)
                    .clickable { preview = true }
            )
            if (preview) {
                Dialog(onDismissRequest = { preview = false }) {
                    Surface(color = Color.Black.copy(alpha = 0.92f), shape = MaterialTheme.shapes.medium) {
                        Image(
                            bitmap!!.asImageBitmap(),
                            contentDescription = "图片原图",
                            contentScale = ContentScale.Fit,
                            modifier = Modifier
                                .fillMaxWidth()
                                .fillMaxHeight(0.86f)
                                .padding(10.dp)
                                .clickable { preview = false }
                        )
                    }
                }
            }
        }
        failed -> Text("[图片加载失败]")
        else -> CircularProgressIndicator(Modifier.size(24.dp))
    }
}

fun attachmentCacheFile(context: android.content.Context, attachmentId: Long): File =
    File(File(context.filesDir, "attachments"), "$attachmentId.img")

fun conversationCacheJson(detail: ConversationDetail, messages: List<ChatMessage>): String {
    val root = JSONObject()
    root.put("conversation", JSONObject().apply {
        put("id", detail.conversationId)
        put("customer", JSONObject().apply {
            put("email", detail.customerEmail)
            put("blocked", detail.blocked)
        })
        put("site", JSONObject().apply {
            put("name", detail.siteName)
        })
    })
    root.put("read_state", JSONObject().apply {
        put("customer_last_seen_seq", detail.customerLastSeenSeq)
        if (detail.customerLastSeenAt == null) put("customer_last_seen_at", JSONObject.NULL) else put("customer_last_seen_at", detail.customerLastSeenAt)
        put("agent_last_seen_seq", detail.agentLastSeenSeq)
        if (detail.agentLastSeenAt == null) put("agent_last_seen_at", JSONObject.NULL) else put("agent_last_seen_at", detail.agentLastSeenAt)
    })
    root.put("messages", org.json.JSONArray().apply {
        messages.forEach { m ->
            put(JSONObject().apply {
                put("id", m.id)
                put("sender_type", m.senderType)
                put("seq", m.seq)
                put("type", m.type)
                put("content", m.content)
                if (m.customerReadAt == null) put("customer_read_at", JSONObject.NULL) else put("customer_read_at", m.customerReadAt)
                if (m.attachmentId == null) put("attachment_id", JSONObject.NULL) else put("attachment_id", m.attachmentId)
                put("created_at", m.createdAt)
                if (m.clientMsgId == null) put("client_msg_id", JSONObject.NULL) else put("client_msg_id", m.clientMsgId)
            })
        }
    })
    return root.toString()
}

fun formatTime(value: String): String {
    if (value.isBlank()) return "刚刚"
    return runCatching { OffsetDateTime.parse(value).format(DateTimeFormatter.ofPattern("HH:mm")) }.getOrDefault(value.take(16))
}

fun syncAgeSeconds(value: String, nowMillis: Long = System.currentTimeMillis()): Long {
    if (value.isBlank() || value == "null") return -1
    return runCatching {
        ((nowMillis - OffsetDateTime.parse(value).toInstant().toEpochMilli()) / 1000).coerceAtLeast(0)
    }.getOrDefault(-1)
}

fun mergeMessages(messages: List<ChatMessage>): List<ChatMessage> {
    val byServerId = LinkedHashMap<Long, ChatMessage>()
    val byClientId = LinkedHashMap<String, ChatMessage>()
    val result = mutableListOf<ChatMessage>()
    messages.sortedWith(compareBy<ChatMessage> { if (it.seq > 0) it.seq else Long.MAX_VALUE }.thenBy { it.id }).forEach { message ->
        val existingIndex = when {
            message.id > 0 && byServerId.containsKey(message.id) -> result.indexOf(byServerId[message.id])
            message.clientMsgId != null && byClientId.containsKey(message.clientMsgId) -> result.indexOf(byClientId[message.clientMsgId])
            else -> -1
        }
        if (existingIndex >= 0) {
            val existing = result[existingIndex]
            result[existingIndex] = if (message.id > 0 || message.seq > 0 || existing.id <= 0) message else existing
        } else {
            result.add(message)
        }
        if (message.id > 0) byServerId[message.id] = message
        message.clientMsgId?.let { byClientId[it] = message }
    }
    return result
}
