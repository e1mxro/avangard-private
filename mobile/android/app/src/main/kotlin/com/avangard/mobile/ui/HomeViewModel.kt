package com.avangard.mobile.ui

import android.app.Application
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import avmobile.Avmobile
import com.avangard.mobile.AvangardForegroundService
import com.avangard.mobile.AvangardVpnService
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch

private val Application.prefs: DataStore<Preferences> by preferencesDataStore(name = "avangard-prefs")

/** Tunnel mode the user has selected. */
enum class TunnelMode { SYSTEM_VPN, SOCKS5_ONLY }

data class HomeUiState(
    val uri: String = "",
    val transport: String = "tcp",
    val mode: TunnelMode = TunnelMode.SYSTEM_VPN,
    val running: Boolean = false,
    val statusMessage: String = "Disconnected",
)

/** Side-effect requests the ViewModel asks the Activity to perform. */
sealed class HomeEvent {
    /** Activity must call VpnService.prepare() and (on consent) start the VPN service. */
    data class StartSystemVpn(val uri: String, val transport: String) : HomeEvent()
    /** Activity should start the legacy SOCKS5-only foreground service. */
    data class StartSocksOnly(val uri: String, val transport: String) : HomeEvent()
    /** Activity should stop whichever service is currently active. */
    data class Stop(val mode: TunnelMode) : HomeEvent()
}

class HomeViewModel(private val app: Application) : AndroidViewModel(app) {

    private val _state = MutableStateFlow(HomeUiState())
    val state: StateFlow<HomeUiState> = _state.asStateFlow()

    private val _events = Channel<HomeEvent>(capacity = Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    init {
        viewModelScope.launch {
            val data = app.prefs.data.first()
            _state.value = _state.value.copy(
                uri = data[KEY_URI].orEmpty(),
                mode = if (data[KEY_VPN_MODE] != false) TunnelMode.SYSTEM_VPN else TunnelMode.SOCKS5_ONLY,
            )
        }
        viewModelScope.launch {
            while (true) {
                val running = try { Avmobile.isRunning() } catch (_: Throwable) { false }
                if (running != _state.value.running) {
                    _state.value = _state.value.copy(
                        running = running,
                        statusMessage = if (running) statusForRunning() else "Disconnected",
                    )
                }
                delay(1_000)
            }
        }
    }

    private fun statusForRunning(): String = when (_state.value.mode) {
        TunnelMode.SYSTEM_VPN -> "Connected — system-wide VPN active"
        TunnelMode.SOCKS5_ONLY -> "Connected — SOCKS5 on ${AvangardForegroundService.LISTEN_ADDR}"
    }

    fun onUriChanged(s: String) { _state.value = _state.value.copy(uri = s) }

    fun onTransportChanged(t: String) { _state.value = _state.value.copy(transport = t) }

    fun onModeChanged(m: TunnelMode) {
        _state.value = _state.value.copy(mode = m)
        viewModelScope.launch {
            app.prefs.edit { it[KEY_VPN_MODE] = (m == TunnelMode.SYSTEM_VPN) }
        }
    }

    fun onUriPasted(uri: String) { _state.value = _state.value.copy(uri = uri) }

    fun connect() {
        val uri = _state.value.uri.trim()
        if (uri.isEmpty()) {
            _state.value = _state.value.copy(statusMessage = "Paste an avangard:// URI first")
            return
        }
        val transport = _state.value.transport
        val mode = _state.value.mode
        viewModelScope.launch {
            app.prefs.edit { it[KEY_URI] = uri }
            _state.value = _state.value.copy(statusMessage = "Connecting…")
            val event = when (mode) {
                TunnelMode.SYSTEM_VPN -> HomeEvent.StartSystemVpn(uri, transport)
                TunnelMode.SOCKS5_ONLY -> HomeEvent.StartSocksOnly(uri, transport)
            }
            _events.send(event)
        }
    }

    fun disconnect() {
        viewModelScope.launch {
            _state.value = _state.value.copy(statusMessage = "Disconnecting…")
            _events.send(HomeEvent.Stop(_state.value.mode))
        }
    }

    /** Activity reports an error from VpnService.prepare or service start. */
    fun onActivityError(message: String) {
        _state.value = _state.value.copy(statusMessage = message)
    }

    companion object {
        private val KEY_URI = stringPreferencesKey("uri")
        private val KEY_VPN_MODE = booleanPreferencesKey("vpn_mode")
        fun factory(app: Application) = object : ViewModelProvider.Factory {
            @Suppress("UNCHECKED_CAST")
            override fun <T : ViewModel> create(modelClass: Class<T>): T = HomeViewModel(app) as T
        }
    }
}

/** Helper used by Activity once the user has granted VpnService consent. */
fun launchVpnService(app: Application, uri: String, transport: String) {
    app.startForegroundService(AvangardVpnService.startIntent(app, uri, transport))
}
