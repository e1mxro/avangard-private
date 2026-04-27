package com.avangard.mobile.qr

import android.app.Activity
import android.content.Context
import android.content.Intent
import androidx.activity.result.contract.ActivityResultContract
import com.journeyapps.barcodescanner.CaptureActivity
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanIntentResult
import com.journeyapps.barcodescanner.ScanOptions

/**
 * Wraps ZXing's [ScanContract] in an Avangard-specific [ActivityResultContract]
 * so callers can simply do:
 *
 *     val launcher = rememberLauncherForActivityResult(QrScanContract()) { uri ->
 *         if (uri != null) ...
 *     }
 *     launcher.launch(Unit)
 *
 * The contract returns the raw scanned text (or null on cancel / no match).
 */
class QrScanContract : ActivityResultContract<Unit, String?>() {

    private val delegate = ScanContract()

    override fun createIntent(context: Context, input: Unit): Intent {
        val opts = ScanOptions().apply {
            setDesiredBarcodeFormats(ScanOptions.QR_CODE)
            setPrompt("Scan AVANGARD QR")
            setBeepEnabled(false)
            setBarcodeImageEnabled(false)
            setOrientationLocked(false)
            setCaptureActivity(CaptureActivity::class.java)
        }
        return delegate.createIntent(context, opts)
    }

    override fun parseResult(resultCode: Int, intent: Intent?): String? {
        if (resultCode != Activity.RESULT_OK) return null
        val r: ScanIntentResult = delegate.parseResult(resultCode, intent)
        return r.contents
    }
}
