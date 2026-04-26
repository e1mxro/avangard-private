package com.avangard.mobile

import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.core.view.WindowCompat
import com.avangard.mobile.ui.HomeScreen
import com.avangard.mobile.ui.HomeViewModel
import com.avangard.mobile.ui.theme.AvangardTheme

class MainActivity : ComponentActivity() {

    private val vm: HomeViewModel by viewModels { HomeViewModel.factory(application) }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window, false)
        handleIntent(intent)
        setContent {
            AvangardTheme {
                HomeScreen(vm)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        val data: Uri = intent?.data ?: return
        if (data.scheme == "avangard") {
            vm.onUriPasted(data.toString())
        }
    }
}
