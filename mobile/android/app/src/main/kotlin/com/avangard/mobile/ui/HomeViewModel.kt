package com.avangard.mobile.ui

import android.app.Application
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import avmobile.Avmobile
import com.avangard.mobile.AvangardForegroundService
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

private val Application.prefs: DataStore<Preferences> by preferencesDataStore(name = "avangard-prefs")

data class HomeUiState(
    val uri: String = "",
    val transport: String = "tcp",
    val running: Boolean = false,
    val statusMessage: String = "Disconnected",
)

class HomeViewModel(private val app: Application) : AndroidViewModel(app) {

    private val _state = MutableStateFlow(HomeUiState())
    val state: StateFlow<HomeUiState> = _state.asStateFlow()

    init {
        viewModelScope.launch {
            val saved = app.prefs.data.first()[KEY_URI].orEmpty()
            _state.value = _state.value.copy(uri = saved)
        }
        viewModelScope.launch {
            while (true) {
                val running = try {
                    Avmobile.isRunning()
                } catch (_: Throwable) {
                    false
                }
                if (running != _state.value.running) {
                    _state.value = _state.value.copy(
                        running = running,
                        statusMessage = if (running)
                            "Connected — SOCKS5 on ${AvangardForegroundService.LISTEN_ADDR}"
                        else
                            "Disconnected",
                    )
                }
                delay(1_000)
            }
        }
    }

    fun onUriChanged(s: String) {
        _state.value = _state.value.copy(uri = s)
    }

    fun onTransportChanged(t: String) {
        _state.value = _state.value.copy(transport = t)
    }

    fun onUriPasted(uri: String) {
        _state.value = _state.value.copy(uri = uri)
    }

    fun connect() {
        val uri = _state.value.uri.trim()
        if (uri.isEmpty()) {
            _state.value = _state.value.copy(statusMessage = "Paste an avangard:// URI first")
            return
        }
        viewModelScope.launch {
            app.prefs.edit { it[KEY_URI] = uri }
            _state.value = _state.value.copy(statusMessage = "Connecting…")
            AvangardForegroundService.start(app, uri, _state.value.transport)
        }
    }

    fun disconnect() {
        AvangardForegroundService.stop(app)
        _state.value = _state.value.copy(statusMessage = "Disconnecting…")
    }

    companion object {
        private val KEY_URI = stringPreferencesKey("uri")
        fun factory(app: Application) = object : ViewModelProvider.Factory {
            @Suppress("UNCHECKED_CAST")
            override fun <T : ViewModel> create(modelClass: Class<T>): T = HomeViewModel(app) as T
        }
    }
}
