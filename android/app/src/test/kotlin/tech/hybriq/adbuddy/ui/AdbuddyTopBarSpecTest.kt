package tech.hybriq.adbuddy.ui

import androidx.compose.ui.unit.dp
import org.junit.Assert.assertEquals
import org.junit.Test

class AdbuddyTopBarSpecTest {
    @Test fun theBarIs64dpUntilTwoLinesOfTitleNeedMore() {
        assertEquals(64.dp, AdbuddyTopBarSpec.height(twoLines = 56.dp))
        assertEquals(64.dp, AdbuddyTopBarSpec.height(twoLines = 40.dp))
        assertEquals(98.dp, AdbuddyTopBarSpec.height(twoLines = 90.dp))
    }
}
