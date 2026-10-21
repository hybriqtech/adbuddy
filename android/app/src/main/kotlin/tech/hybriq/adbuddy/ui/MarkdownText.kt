package tech.hybriq.adbuddy.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.withLink
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import tech.hybriq.adbuddy.chat.Block
import tech.hybriq.adbuddy.chat.Inline
import tech.hybriq.adbuddy.chat.Links
import tech.hybriq.adbuddy.chat.Markdown

/**
 * The agent's message as markdown (spec M4 §7.1). Only what Markdown.parse
 * gives is drawn: no HTML, no images, and a link opens through onLink, which
 * shows its domain first (the same as LinkedText). Plain web addresses in the
 * text are links too, as in M3.
 */
@Composable
fun MarkdownText(text: String, onLink: (String) -> Unit, modifier: Modifier = Modifier) {
    val blocks = remember(text) { Markdown.parse(text) }
    val link = TextLinkStyles(SpanStyle(color = MaterialTheme.colorScheme.primary, textDecoration = TextDecoration.Underline))
    val codeBg = MaterialTheme.colorScheme.surface.copy(alpha = 0.6f)
    // Built once per (blocks, onLink, styles): a new onLink rebuilds them, so a link never calls an old callback.
    val texts = remember(blocks, onLink, link, codeBg) { blocks.map { b -> annotate(inlinesOf(b), onLink, link, codeBg) } }
    Column(modifier, verticalArrangement = Arrangement.spacedBy(6.dp)) {
        for ((n, b) in blocks.withIndex()) {
            val t = texts[n]
            when (b) {
                is Block.Paragraph -> Text(t, style = MaterialTheme.typography.bodyLarge)
                is Block.Heading -> Text(
                    t,
                    style = if (b.level <= 1) MaterialTheme.typography.titleLarge else MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.Bold,
                )
                is Block.Quote -> Row(Modifier.height(IntrinsicSize.Min)) {
                    Box(Modifier.width(3.dp).fillMaxHeight().background(MaterialTheme.colorScheme.outline))
                    Spacer(Modifier.width(8.dp))
                    Text(t, style = MaterialTheme.typography.bodyLarge, fontStyle = FontStyle.Italic)
                }
                is Block.Item -> Row(Modifier.padding(start = (b.indent * 16).dp)) {
                    Text(b.marker + " ", style = MaterialTheme.typography.bodyLarge)
                    Text(t, style = MaterialTheme.typography.bodyLarge)
                }
                is Block.CodeBlock -> Surface(color = codeBg, shape = MaterialTheme.shapes.small) {
                    Text(
                        b.text,
                        Modifier.horizontalScroll(rememberScrollState()).padding(8.dp),
                        style = MaterialTheme.typography.bodyMedium,
                        fontFamily = FontFamily.Monospace,
                        softWrap = false,
                    )
                }
            }
        }
    }
}

/** A code block has no inlines: it is drawn from its raw text. */
private fun inlinesOf(b: Block): List<Inline> = when (b) {
    is Block.Paragraph -> b.inlines
    is Block.Heading -> b.inlines
    is Block.Quote -> b.inlines
    is Block.Item -> b.inlines
    is Block.CodeBlock -> emptyList()
}

private fun annotate(inlines: List<Inline>, onLink: (String) -> Unit, link: TextLinkStyles, codeBg: Color): AnnotatedString = buildAnnotatedString {
    for (i in inlines) {
        when (i) {
            is Inline.Text -> {
                var at = 0
                for (s in Links.find(i.text)) {
                    append(i.text.substring(at, s.start))
                    withLink(LinkAnnotation.Clickable(tag = s.url, styles = link, linkInteractionListener = { onLink(s.url) })) { append(i.text.substring(s.start, s.end)) }
                    at = s.end
                }
                append(i.text.substring(at))
            }
            is Inline.Strong -> withStyle(SpanStyle(fontWeight = FontWeight.Bold)) { append(i.text) }
            is Inline.Em -> withStyle(SpanStyle(fontStyle = FontStyle.Italic)) { append(i.text) }
            is Inline.Code -> withStyle(SpanStyle(fontFamily = FontFamily.Monospace, background = codeBg)) { append(i.text) }
            is Inline.Link -> withLink(LinkAnnotation.Clickable(tag = i.url, styles = link, linkInteractionListener = { onLink(i.url) })) { append(Links.label(i.text, i.url)) }
        }
    }
}
