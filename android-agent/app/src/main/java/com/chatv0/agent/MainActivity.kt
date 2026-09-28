package com.chatv0.agent

import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class MainActivity : ComponentActivity() {
    private lateinit var prefs: AgentPrefs

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        prefs = AgentPrefs(this)
        setContent { AgentApp(prefs) { startService(Intent(this, AgentForegroundService::class.java)) } }
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
    LaunchedEffect(Unit) { onStartService(); refresh() }

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
        Text(status, style = MaterialTheme.typography.bodySmall)
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(customers) { c ->
                Card(Modifier.fillMaxWidth().clickable { onSelect(c) }) {
                    Column(Modifier.padding(12.dp)) {
                        Text("[${c.siteName}] ${c.email}", fontWeight = FontWeight.SemiBold)
                        Text("conversation #${c.conversationId}" + if (c.blocked) " · 已限制" else "", style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }
}

@Composable
fun ConversationScreen(prefs: AgentPrefs, customer: CustomerItem, onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    var detail by remember { mutableStateOf<ConversationDetail?>(null) }
    var input by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("加载中") }

    fun refresh() {
        scope.launch {
            runCatching { withContext(Dispatchers.IO) { AgentApi(prefs.baseUrl, prefs.token).conversation(customer.conversationId) } }
                .onSuccess { detail = it; status = ""; withContext(Dispatchers.IO) { runCatching { AgentApi(prefs.baseUrl, prefs.token).markRead(customer.conversationId) } } }
                .onFailure { status = it.message ?: "加载失败" }
        }
    }
    LaunchedEffect(customer.conversationId) { refresh() }

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
        LazyColumn(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(detail?.messages ?: emptyList()) { m ->
                val who = if (m.senderType == "agent") "客服" else "客户"
                Card(Modifier.fillMaxWidth()) { Text("$who：${if (m.type == "image") "[图片消息]" else m.content}", Modifier.padding(10.dp)) }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(input, { input = it }, modifier = Modifier.weight(1f), placeholder = { Text("输入回复") })
            Button(onClick = {
                val text = input.trim(); if (text.isBlank()) return@Button
                input = ""
                scope.launch { withContext(Dispatchers.IO) { runCatching { AgentApi(prefs.baseUrl, prefs.token).sendMessage(customer.conversationId, text) } }; refresh() }
            }) { Text("发送") }
        }
    }
}

fun deviceId(context: android.content.Context): String = "android-" + Settings.Secure.getString(context.contentResolver, Settings.Secure.ANDROID_ID)
