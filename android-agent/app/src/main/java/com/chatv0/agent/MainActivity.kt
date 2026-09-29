package com.chatv0.agent

import android.Manifest
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
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
            CustomerListScreen(prefs, onLogout = { prefs.clearAuth(); token = "" }, onSelect = { selected = it }, onStartService = onStartService)
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
    val context = LocalContext.current

    Column(Modifier.fillMaxSize().padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("Chat V0 客服端", style = MaterialTheme.typography.headlineSmall)
        OutlinedTextField(baseUrl, { baseUrl = it }, label = { Text("服务器地址") }, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(login, { login = it }, label = { Text("用户名或邮箱") }, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(password, { password = it }, label = { Text("密码") }, modifier = Modifier.fillMaxWidth())
        Button(onClick = {
            scope.launch {
                status = "登录中..."
                runCatching {
                    withContext(Dispatchers.IO) {
                        val cleanBase = baseUrl.trimEnd('/')
                        val token = AgentApi(cleanBase).login(login, password, deviceId(context))
                        prefs.baseUrl = cleanBase
                        prefs.login = login
                        prefs.token = token
                        token
                    }
                }.onSuccess { status = "登录成功"; onLoggedIn(it) }
                    .onFailure { status = it.message ?: "登录失败" }
            }
        }, modifier = Modifier.fillMaxWidth()) { Text("登录并启动客服服务") }
        Text(status)
        Text("请在系统设置中允许通知、后台运行、自启动和电池无限制。", style = MaterialTheme.typography.bodySmall)
        PermissionButtons()
    }
}

@Composable
fun CustomerListScreen(prefs: AgentPrefs, onLogout: () -> Unit, onSelect: (CustomerItem) -> Unit, onStartService: () -> Unit) {
    val scope = rememberCoroutineScope()
    var customers by remember { mutableStateOf<List<CustomerItem>>(emptyList()) }
    var status by remember { mutableStateOf("加载中") }
    var duty by remember { mutableStateOf(prefs.acceptEmergency) }

    fun refresh() {
        scope.launch {
            runCatching { withContext(Dispatchers.IO) { AgentApi(prefs.baseUrl, prefs.token).customers() } }
                .onSuccess { customers = it; status = "共 ${it.size} 个客户" }
                .onFailure { status = it.message ?: "加载失败" }
        }
    }
    LaunchedEffect(Unit) {
        onStartService()
        while (true) {
            refresh()
            delay(3000)
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
            OutlinedButton(onClick = { openFullScreenSettings(context) }, modifier = Modifier.weight(1f)) { Text("全屏弹窗") }
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

@Composable
fun ConversationScreen(prefs: AgentPrefs, customer: CustomerItem, onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    var detail by remember { mutableStateOf<ConversationDetail?>(null) }
    var localMessages by remember { mutableStateOf<List<ChatMessage>>(emptyList()) }
    var input by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("加载中") }
    val listState = rememberLazyListState()
    BackHandler { onBack() }

    fun refresh() {
        scope.launch {
            runCatching { withContext(Dispatchers.IO) {
                val api = AgentApi(prefs.baseUrl, prefs.token)
                val json = api.conversationJson(customer.conversationId)
                prefs.setCachedConversation(customer.conversationId, json.toString())
                api.parseConversation(json)
            } }
                .onSuccess {
                    detail = it
                    localMessages = it.messages
                    status = ""
                    ContextCompat.startForegroundService(context, Intent(context, AgentForegroundService::class.java).setAction(AgentForegroundService.ACTION_STOP_RING))
                    withContext(Dispatchers.IO) { runCatching { AgentApi(prefs.baseUrl, prefs.token).markRead(customer.conversationId) } }
                }
                .onFailure { status = it.message ?: "加载失败" }
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
    val imageLauncher = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri: Uri? ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            status = "图片上传中..."
            runCatching {
                withContext(Dispatchers.IO) {
                    AgentApi(prefs.baseUrl, prefs.token).uploadImage(context, customer.conversationId, uri)
                }
            }.onSuccess { refresh() }
                .onFailure { status = it.message ?: "图片上传失败" }
        }
    }

    fun sendText(text: String, localKey: String = UUID.randomUUID().toString()) {
        val existing = localMessages.any { it.localKey == localKey }
        if (existing) {
            localMessages = localMessages.map { if (it.localKey == localKey) it.copy(localStatus = "sending") else it }
        } else {
            val pending = ChatMessage(-System.currentTimeMillis(), "agent", 0, "text", text, null, null, "", "sending", localKey)
            localMessages = localMessages + pending
        }
        scope.launch {
            runCatching {
                withContext(Dispatchers.IO) { AgentApi(prefs.baseUrl, prefs.token).sendMessage(customer.conversationId, text) }
            }.onSuccess { refresh() }
                .onFailure {
                    localMessages = localMessages.map { if (it.localKey == localKey) it.copy(localStatus = "failed") else it }
                }
        }
    }
    LaunchedEffect(localMessages.size) {
        if (localMessages.isNotEmpty()) {
            listState.animateScrollToItem(localMessages.lastIndex)
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
                MessageBubble(m, prefs, onRetry = { sendText(m.content, m.localKey) })
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
fun MessageBubble(message: ChatMessage, prefs: AgentPrefs, onRetry: () -> Unit) {
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
                                message.customerReadAt != null -> "已读"
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
        bitmap != null -> Image(bitmap!!.asImageBitmap(), contentDescription = "图片消息", modifier = Modifier.sizeIn(maxWidth = 180.dp, maxHeight = 180.dp))
        failed -> Text("[图片加载失败]")
        else -> CircularProgressIndicator(Modifier.size(24.dp))
    }
}

fun attachmentCacheFile(context: android.content.Context, attachmentId: Long): File =
    File(File(context.filesDir, "attachments"), "$attachmentId.img")

fun formatTime(value: String): String {
    if (value.isBlank()) return "刚刚"
    return runCatching { OffsetDateTime.parse(value).format(DateTimeFormatter.ofPattern("HH:mm")) }.getOrDefault(value.take(16))
}

fun deviceId(context: android.content.Context): String = "android-" + Settings.Secure.getString(context.contentResolver, Settings.Secure.ANDROID_ID)
