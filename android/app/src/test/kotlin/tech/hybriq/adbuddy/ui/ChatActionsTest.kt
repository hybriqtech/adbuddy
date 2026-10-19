package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertEquals
import org.junit.Test

class ChatActionsTest {
    // Mutation: leave openYolo out of the constructor call in copyWith (it falls back to {}).
    @Test fun copyWithKeepsTheYoloEntry() {
        var opened = 0
        val base = ChatActions({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, openYolo = { opened++ })
        base.copyWith(cut = {}).openYolo()
        assertEquals(1, opened)
    }
}
