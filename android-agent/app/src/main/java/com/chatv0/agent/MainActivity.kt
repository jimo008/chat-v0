package com.chatv0.agent

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { AgentApp { startService(Intent(this, AgentForegroundService::class.java)) } }
    }
}

@Composable
fun AgentApp(onStartService: () -> Unit) {
    var baseUrl by remember { mutableStateOf("") }
    var login by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("未登录") }

    MaterialTheme {
        Column(Modifier.fillMaxSize().padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Chat V0 客服端", style = MaterialTheme.typography.headlineSmall)
            OutlinedTextField(baseUrl, { baseUrl = it }, label = { Text("服务器地址") }, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(login, { login = it }, label = { Text("用户名或邮箱") }, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(password, { password = it }, label = { Text("密码") }, modifier = Modifier.fillMaxWidth())
            Button(onClick = {
                AgentSession.baseUrl = baseUrl.trimEnd('/')
                AgentSession.login = login
                AgentSession.password = password
                status = "已启动前台服务，正在连接"
                onStartService()
            }, modifier = Modifier.fillMaxWidth()) { Text("登录并启动客服服务") }
            Text(status)
            Text("普通消息始终接收；紧急值班只影响紧急呼叫。请在系统设置中允许通知、后台运行、自启动和电池无限制。", style = MaterialTheme.typography.bodySmall)
        }
    }
}
