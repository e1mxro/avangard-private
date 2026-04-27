package com.avangard.mobile

import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.net.VpnService
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.ActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.avangard.mobile.ui.HomeEvent
import com.avangard.mobile.ui.HomeScreen
import com.avangard.mobile.ui.HomeViewModel
import com.avangard.mobile.ui.TunnelMode
import com.avangard.mobile.ui.launchVpnService
import com.avangard.mobile.ui.theme.AvangardTheme
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {

    private val vm: HomeViewModel by viewModels { HomeViewModel.factory(application) }

    /** Pending VPN start request kept while we wait for the system consent dialog. */
    private var pendingVpnStart: HomeEvent.StartSystemVpn? = null

    private val vpnConsentLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result: ActivityResult ->
        val pending = pendingVpnStart
        pendingVpnStart = null
        if (result.resultCode == Activity.RESULT_OK && pending != null) {
            launchVpnService(application, pending.uri, pending.transport)
        } else {
            vm.onActivityError("VPN consent denied — system-wide tunneling disabled")
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window, false)
        handleIntent(intent)

        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                vm.events.collect { event -> handleEvent(event) }
            }
        }

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

    private fun handleEvent(event: HomeEvent) {
        when (event) {
            is HomeEvent.StartSystemVpn -> requestVpnConsent(event)
            is HomeEvent.StartSocksOnly -> AvangardForegroundService.start(this, event.uri, event.transport)
            is HomeEvent.Stop -> when (event.mode) {
                TunnelMode.SYSTEM_VPN -> startService(AvangardVpnService.stopIntent(this))
                TunnelMode.SOCKS5_ONLY -> AvangardForegroundService.stop(this)
            }
        }
    }

    private fun requestVpnConsent(event: HomeEvent.StartSystemVpn) {
        val prepareIntent = VpnService.prepare(this)
        if (prepareIntent == null) {
            // Already approved.
            launchVpnService(application, event.uri, event.transport)
        } else {
            pendingVpnStart = event
            vpnConsentLauncher.launch(prepareIntent)
        }
    }
}
