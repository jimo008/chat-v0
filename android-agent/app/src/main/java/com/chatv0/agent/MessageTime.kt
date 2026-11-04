package com.chatv0.agent

import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

fun formatTime(value: String): String {
    if (value.isBlank()) return "刚刚"
    return runCatching {
        OffsetDateTime.parse(value).atZoneSameInstant(ZoneId.of("Asia/Shanghai"))
            .format(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm"))
    }.getOrDefault(value.take(16).replace('T', ' '))
}
