package com.chatv0.agent

object AgentSession {
    var baseUrl: String = ""
    var login: String = ""
    var password: String = ""
    var token: String = ""
    var lastSeq: Long = 0
    val deviceId: String = "android-" + android.os.Build.SERIAL.hashCode().toString()
}
