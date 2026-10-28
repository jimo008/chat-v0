package com.chatv0.agent

import org.junit.Assert.*
import org.junit.Test

class EventCursorTest {
    @Test fun snapshotStartsAtHeadNotTheFirstTwoHundredEvents() {
        val cursor = EventCursor(5000)
        for (seq in 1L..5000L) assertFalse(cursor.accept(seq))
        assertTrue(cursor.accept(5001))
        assertEquals(5001L, cursor.value)
    }

    @Test fun websocketAndPollCopiesAreAcceptedOnlyOnce() {
        val cursor = EventCursor(10)
        assertTrue(cursor.accept(11))
        assertTrue(cursor.accept(12))
        assertFalse(cursor.accept(11))
        assertFalse(cursor.accept(12))
        assertFalse(cursor.accept(0))
        assertTrue(cursor.accept(13))
    }

    @Test fun onlyExplicitInvalidSessionsRequireLogin() {
        assertTrue(AgentRequestException(401, "invalid_agent_token", "old").invalidSession)
        for (code in listOf("device_clock_out_of_sync", "device_request_replayed", "", "upstream_error")) {
            assertFalse(AgentRequestException(401, code, "current").invalidSession)
        }
        assertFalse(AgentRequestException(503, "invalid_agent_token", "current").invalidSession)
    }
}
