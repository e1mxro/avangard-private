package com.avangard.mobile

import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.ActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.avangard.mobile.data.PerAppMode
import com.avangard.mobile.data.Profile
import com.avangard.mobile.data.TunnelMode
import com.avangard.mobile.ui.AppEvent
import com.avangard.mobile.ui.AppNav
import com.avangard.mobile.ui.AppViewModel
import com.avangard.mobile.ui.theme.AvangardTheme
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {

    private val vm: AppViewModel by viewModels { AppViewModel.factory(application) }

    /** Pending VPN start request kept while we wait for the system consent dialog. */
    private var pendingVpnStart: AppEvent.StartSystemVpn? = null

    private val vpnConsentLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result: ActivityResult ->
        val pending = pendingVpnStart
        pendingVpnStart = null
        if (result.resultCode == Activity.RESULT_OK && pending != null) {
            launchVpnService(pending.profile, pending.perAppMode, pending.perAppPackages)
        } else {
            vm.reportActivityError("VPN consent denied — system-wide tunneling disabled")
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
            val theme by vm.themeMode.collectAsState()
            AvangardTheme(mode = theme) {
                AppNav(vm = vm)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    /**
     * Accepts `avangard://...` deep-link intents. We save the URI as a new
     * profile (auto-named from the host:port) and switch to it. The user
     * still has to push Connect — surprise auto-connect from a foreign URL
     * would be hostile.
     */
    private fun handleIntent(intent: Intent?) {
        val data: Uri = intent?.data ?: return
        if (data.scheme != "avangard") return
        val uri = data.toString()
        vm.saveProfile(Profile(name = "", uri = uri), makeActive = true)
    }

    private fun handleEvent(event: AppEvent) {
        when (event) {
            is AppEvent.StartSystemVpn -> requestVpnConsent(event)
            is AppEvent.StartSocksOnly -> AvangardForegroundService.start(
                this, event.profile.uri, event.profile.transport,
            )
            is AppEvent.Stop -> when (event.tunnelMode) {
                // The STOP intent must use startService, NOT startForegroundService:
                // VpnService.stop() never calls startForeground() (it only stops the
                // foreground state), so on Android 8+ a startForegroundService here
                // races a ForegroundServiceDidNotStartInTimeException if the OS had
                // already killed the service or if onRevoke() raced our tap.
                TunnelMode.SYSTEM_VPN -> startService(AvangardVpnService.stopIntent(this))
                TunnelMode.SOCKS5_ONLY -> AvangardForegroundService.stop(this)
            }
        }
    }

    private fun requestVpnConsent(event: AppEvent.StartSystemVpn) {
        val prepareIntent = VpnService.prepare(this)
        if (prepareIntent == null) {
            launchVpnService(event.profile, event.perAppMode, event.perAppPackages)
        } else {
            pendingVpnStart = event
            vpnConsentLauncher.launch(prepareIntent)
        }
    }

    private fun launchVpnService(
        profile: Profile,
        perAppMode: PerAppMode,
        perAppPackages: List<String>,
    ) {
        val intent = AvangardVpnService.startIntent(
            this,
            uri = profile.uri,
            transport = profile.transport,
            perAppMode = perAppMode.name,
            perAppPackages = perAppPackages,
        )
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }
    }
}
