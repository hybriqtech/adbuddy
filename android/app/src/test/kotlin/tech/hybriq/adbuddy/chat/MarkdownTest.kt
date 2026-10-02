package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MarkdownTest {
    private fun p(vararg i: Inline) = Block.Paragraph(i.toList())

    @Test fun plainTextIsOneParagraph() {
        assertEquals(listOf(p(Inline.Text("Hola\nmundo"))), Markdown.parse("Hola\nmundo"))
        assertEquals(listOf(p(Inline.Text("a")), p(Inline.Text("b"))), Markdown.parse("a\n\nb"))
        assertEquals(emptyList<Block>(), Markdown.parse(""))
        assertEquals(listOf(p(Inline.Text("a")), p(Inline.Text("b"))), Markdown.parse("a\r\n\r\nb"))
    }

    @Test fun inlineStyles() {
        assertEquals(
            listOf(p(Inline.Text("a "), Inline.Strong("fuerte"), Inline.Text(", "), Inline.Em("cursiva"), Inline.Text(" y "), Inline.Code("código"))),
            Markdown.parse("a **fuerte**, *cursiva* y `código`"),
        )
        assertEquals(listOf(p(Inline.Em("uno"))), Markdown.parse("_uno_"))
    }

    @Test fun lonelyMarksAreText() {
        for (t in listOf("2 * 3 * 4", "snake_case_name", "a * b", "**", "``", "`", "_", "*", "**no cierra", "[x](", "[x]", "5 ** 2")) {
            val flat = Markdown.parse(t).flatMap { (it as Block.Paragraph).inlines }
            assertTrue("$t -> $flat", flat.all { it is Inline.Text })
            assertEquals(t, flat.joinToString("") { (it as Inline.Text).text })
        }
    }

    @Test fun webLinksKeepTheirDomainCheckable() {
        val b = Markdown.parse("ver [la web](https://example.com/a?b=1) ya")
        assertEquals(listOf(p(Inline.Text("ver "), Inline.Link("la web", "https://example.com/a?b=1"), Inline.Text(" ya"))), b)
        assertEquals("example.com", Links.domain((Markdown.parse("[x](http://Example.com)")[0] as Block.Paragraph).inlines[0].let { (it as Inline.Link).url }))
    }

    @Test fun onlyHttpLinksAreLinks() {
        for (url in listOf("intent://x#Intent;end", "file:///etc/passwd", "content://x", "data:text/html,x", "tel:112", "ftp://x.org", "//evil.com", "mailto:a@b.c")) {
            val flat = Markdown.parse("[pulsa]($url)").flatMap { (it as Block.Paragraph).inlines }
            assertFalse("$url became a link", flat.any { it is Inline.Link })
            assertEquals("the label stays, the url is dropped", "pulsa", flat.joinToString("") { (it as Inline.Text).text })
        }
        // A url with parentheses is not a link at all (it stays as the text it was, not clickable).
        val js = Markdown.parse("[pulsa](javascript:alert(1))").flatMap { (it as Block.Paragraph).inlines }
        assertFalse(js.any { it is Inline.Link })
    }

    @Test fun imagesAreOnlyTheirAltTextAndTheUrlIsNeverALink() {
        val b = Markdown.parse("![gráfico](https://evil.example/pixel.png?leak=secret) y ![](https://evil.example/x.png)")
        val flat = b.flatMap { (it as Block.Paragraph).inlines }
        assertTrue(flat.all { it is Inline.Text })
        val text = flat.joinToString("") { (it as Inline.Text).text }
        assertFalse(text.contains("evil.example"))
        assertTrue(text.startsWith("gráfico y "))
    }

    @Test fun htmlIsNotHtml() {
        val raw = "<img src=\"https://evil.example/p.png\"> <script>alert(1)</script> <b>x</b> <a href=\"https://x.org\">y</a>"
        val flat = Markdown.parse(raw).flatMap { (it as Block.Paragraph).inlines }
        assertEquals(listOf<Inline>(Inline.Text(raw)), flat)
    }

    @Test fun headingsListsAndQuotes() {
        val b = Markdown.parse("# Título\n## Sub\n- uno\n- dos con **negrita**\n  - anidado\n1. primero\n2) segundo\n> cita")
        assertEquals(Block.Heading(1, listOf(Inline.Text("Título"))), b[0])
        assertEquals(Block.Heading(2, listOf(Inline.Text("Sub"))), b[1])
        assertEquals(Block.Item("•", 0, listOf(Inline.Text("uno"))), b[2])
        assertEquals(Block.Item("•", 0, listOf(Inline.Text("dos con "), Inline.Strong("negrita"))), b[3])
        assertEquals(Block.Item("•", 1, listOf(Inline.Text("anidado"))), b[4])
        assertEquals(Block.Item("1.", 0, listOf(Inline.Text("primero"))), b[5])
        assertEquals(Block.Item("2)", 0, listOf(Inline.Text("segundo"))), b[6])
        assertEquals(Block.Quote(listOf(Inline.Text("cita"))), b[7])
        assertEquals(8, b.size)
    }

    @Test fun codeBlocksAreVerbatim() {
        val b = Markdown.parse("antes\n```kotlin\nval a = **no** [x](https://a.com)\n\n# no\n```\ndespués")
        assertEquals(p(Inline.Text("antes")), b[0])
        assertEquals(Block.CodeBlock("kotlin", "val a = **no** [x](https://a.com)\n\n# no"), b[1])
        assertEquals(p(Inline.Text("después")), b[2])
        assertEquals(listOf<Block>(Block.CodeBlock("", "sin cierre\nsigue")), Markdown.parse("```\nsin cierre\nsigue"))
    }

    @Test fun neverThrowsAndIsBounded() {
        val hostile = listOf(
            "*".repeat(50_000), "[".repeat(50_000), "[a](".repeat(20_000), "`".repeat(50_000), "_a_".repeat(30_000), "**a** ".repeat(20_000),
            "#".repeat(50_000), "> ".repeat(50_000), "- ".repeat(50_000), "```\n".repeat(30_000), "\u0000￿\uD800", "a\n".repeat(200_000),
        )
        for (h in hostile) {
            val b = Markdown.parse(h)
            assertTrue(b.size <= Markdown.MAX_BLOCKS)
        }
        val big = Markdown.parse("x".repeat(Markdown.MAX_INPUT * 3))
        val text = ((big[0] as Block.Paragraph).inlines[0] as Inline.Text).text
        assertEquals(Markdown.MAX_INPUT + 1, text.length)
        assertTrue(text.endsWith("…"))
    }

    @Test fun hostileBracketsTakeLinearTime() {
        val n = 2_000_000 // inlines has no cap of its own: a longer text shows the quadratic cost
        val inputs = mapOf(
            "open brackets" to "[".repeat(n),
            "open links" to "[a](".repeat(n / 4),
            "brackets and a late close" to "[".repeat(n - 1) + "]",
            "links and a late close" to "[a](".repeat(n / 4 - 1) + "[a](b)",
            "shared close, url with no end" to "[".repeat(n / 2) + "](" + "x".repeat(n / 2 - 2),
            "shared close, url of brackets" to "[".repeat(n / 2) + "](" + "]".repeat(n / 2 - 2),
            "links and a closing paren" to "[a](".repeat(n / 4 - 1) + ")",
            "images" to "![a](".repeat(n / 5),
            "linked images" to "[![a](b)](".repeat(n / 10),
            "linked images with no link" to "[![a](b)".repeat(n / 9),
        )
        for ((name, text) in inputs) {
            val t0 = System.nanoTime()
            val flat = Markdown.inlines(text)
            val ms = (System.nanoTime() - t0) / 1_000_000
            assertTrue("$name took $ms ms", ms < 2_000)
            assertTrue(name, flat.isNotEmpty())
        }
    }

    @Test fun theMemoryOfAFailedSearchDoesNotChangeWhatIsALink() {
        // a later, valid link after failed attempts is still found
        val b = Markdown.parse("[x [y](https://example.com) [z](")
        val flat = (b.single() as Block.Paragraph).inlines
        assertTrue(flat.toString(), flat.contains(Inline.Link("x [y", "https://example.com")))
        assertEquals(" [z](", (flat.last() as Inline.Text).text)
    }

    /** V43: never more than MAX_BLOCKS, also when one line flushes a paragraph and adds its own block. */
    @Test fun neverMoreThanMaxBlocks() {
        val headings = (1 until Markdown.MAX_BLOCKS).joinToString("\n") { "# h$it" } // one block short
        val b = Markdown.parse("$headings\nthe last paragraph\n# one too many")
        assertEquals(Markdown.MAX_BLOCKS, b.size)
        assertEquals(p(Inline.Text("the last paragraph")), b.last())
        val fence = Markdown.parse("$headings\nthe last paragraph\n```\ncode")
        assertEquals(Markdown.MAX_BLOCKS, fence.size)
    }

    /**
     * V43: an image inside a link (a badge) is the link, with the image's alt text as its text; the image's own
     * URL is never a link, as with an image alone.
     */
    @Test fun aLinkedImageIsTheLinkWithItsAltText() {
        val flat = (Markdown.parse("[![build](https://img.example/badge.svg)](https://example.com/repo)").single() as Block.Paragraph).inlines
        assertEquals(listOf<Inline>(Inline.Link("build", "https://example.com/repo")), flat)
        for (s in listOf(
            "[![a](https://img.example/x.png)](javascript:alert(1))", // not a web link: only text
            "[![a](https://img.example/x.png) and more", // no link around it
            "[![](https://img.example/x.png)](ftp://example.com/x)",
        )) {
            val inl = (Markdown.parse(s).single() as Block.Paragraph).inlines
            assertTrue("$s: $inl", inl.none { it is Inline.Link })
            assertFalse("$s: $inl", inl.joinToString("") { (it as Inline.Text).text }.contains("img.example"))
        }
        val empty = (Markdown.parse("[![](https://img.example/x.png)](https://example.com/)").single() as Block.Paragraph).inlines
        assertEquals("no alt: the link shows its URL", listOf<Inline>(Inline.Link("https://example.com/", "https://example.com/")), empty)
    }
}

class MarkdownRealSizeTest {
    @Test fun aSharedCloseWithNoEndOfUrlTakesLinearTimeAtTheRealLimit() {
        val k = Markdown.MAX_INPUT / 2
        for (filler in listOf("x", "]", "[")) {
            val text = "[".repeat(k - 1) + "](" + filler.repeat(k - 1)
            val t0 = System.nanoTime()
            val blocks = Markdown.parse(text)
            val ms = (System.nanoTime() - t0) / 1_000_000
            assertTrue("filler $filler took $ms ms", ms < 1_500)
            assertTrue(blocks.isNotEmpty())
        }
    }
}
