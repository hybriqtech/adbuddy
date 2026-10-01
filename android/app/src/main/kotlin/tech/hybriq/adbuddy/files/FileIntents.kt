package tech.hybriq.adbuddy.files

import android.content.ActivityNotFoundException
import android.content.ClipData
import android.content.Context
import android.content.Intent
import androidx.core.content.FileProvider
import java.io.File

/** Opens or shares a downloaded file with a temporary read permission (spec M3 §6.4); the file never leaves the app's cache. */
object FileIntents {
    private fun uri(context: Context, file: File) = FileProvider.getUriForFile(context, context.packageName + ".files", file)

    /** false when no app opens that type. */
    fun view(context: Context, file: File, mime: String): Boolean {
        val u = uri(context, file)
        val i = Intent(Intent.ACTION_VIEW).setDataAndType(u, mime).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
        return try { context.startActivity(i); true } catch (e: ActivityNotFoundException) { false }
    }

    fun share(context: Context, file: File, mime: String): Boolean {
        val u = uri(context, file)
        val send = Intent(Intent.ACTION_SEND).setType(mime).putExtra(Intent.EXTRA_STREAM, u).apply { clipData = ClipData.newRawUri("", u) }
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        return try { context.startActivity(Intent.createChooser(send, null).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)); true } catch (e: ActivityNotFoundException) { false }
    }
}
