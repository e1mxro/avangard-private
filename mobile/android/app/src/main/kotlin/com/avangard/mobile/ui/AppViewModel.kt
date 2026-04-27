package com.avangard.mobile.ui

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import avmobile.Avmobile
import com.avangard.mobile.data.AppRepository
import com.avangard.mobile.data.AppSettings
import com.avangard.mobile.data.PerAppMode
import com.avangard.mobile.data.Profile
import com.avangard.mobile.data.ThemeMode
import com.avangard.mobile.data.TunnelMode
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/**
 * Side-effect requests dispatched to MainActivity for activity-scoped work
 * (VPN consent, startService, …).
 */
sealed class AppEvent {
    data class StartSystemVpn(
        val profile: Profile,
        val perAppMode: PerAppMode,
        val perAppPackages: List<String>,
    ) : AppEvent()

    data class StartSocksOnly(val profile: Profile) : AppEvent()
    data class Stop(val tunnelMode: TunnelMode) : AppEvent()
}

/**
 * Connection-state surface for the Home screen.
 */
data class ConnectionUi(
    val running: Boolean = false,
    val statusMessage: String = "Disconnected",
)

class AppViewModel(private val app: Application) : AndroidViewModel(app) {

    private val repo = AppRepository.get(app)

    val profiles: StateFlow<List<Profile>> =
        repo.profiles.stateIn(viewModelScope, SharingStarted.Eagerly, emptyList())

    val settings: StateFlow<AppSettings> =
        repo.settings.stateIn(viewModelScope, SharingStarted.Eagerly, AppSettings())

    /** Currently selected profile; null if empty list or none chosen. */
    val activeProfile: StateFlow<Profile?> =
        combine(profiles, settings) { list, s ->
            list.firstOrNull { it.id == s.activeProfileId } ?: list.firstOrNull()
        }.stateIn(viewModelScope, SharingStarted.Eagerly, null)

    val themeMode: StateFlow<ThemeMode> = settings.map { it.themeModeEnum() }
        .stateIn(viewModelScope, SharingStarted.Eagerly, ThemeMode.SYSTEM)

    private val _connection = MutableStateFlow(ConnectionUi())
    val connection: StateFlow<ConnectionUi> = _connection.asStateFlow()

    private val _events = Channel<AppEvent>(capacity = Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    init {
        // Long-lived poll of the Go-side running flag.
        viewModelScope.launch {
            while (true) {
                val running = try { Avmobile.isRunning() } catch (_: Throwable) { false }
                if (running != _connection.value.running) {
                    val s = settings.value
                    val profile = activeProfile.value
                    val msg = when {
                        running && s.tunnelModeEnum() == TunnelMode.SYSTEM_VPN ->
                            "Connected — system-wide VPN" + (profile?.let { " (${it.displayName})" } ?: "")
                        running ->
                            "Connected — SOCKS5 on 127.0.0.1:18964"
                        else -> "Disconnected"
                    }
                    _connection.value = ConnectionUi(running = running, statusMessage = msg)
                }
                delay(1_000)
            }
        }
    }

    // ---- Mutations ----

    fun saveProfile(p: Profile, makeActive: Boolean = true) {
        viewModelScope.launch {
            repo.saveProfile(p)
            if (makeActive) repo.setActive(p.id)
        }
    }

    fun deleteProfile(id: String) {
        viewModelScope.launch { repo.deleteProfile(id) }
    }

    fun setActive(id: String) {
        viewModelScope.launch { repo.setActive(id) }
    }

    fun setTunnelMode(mode: TunnelMode) {
        viewModelScope.launch { repo.updateSettings { it.copy(tunnelMode = mode.name) } }
    }

    fun setThemeMode(mode: ThemeMode) {
        viewModelScope.launch { repo.updateSettings { it.copy(themeMode = mode.name) } }
    }

    fun setPerApp(mode: PerAppMode, packages: List<String>) {
        viewModelScope.launch {
            repo.updateSettings {
                it.copy(perAppMode = mode.name, perAppPackages = packages.distinct().sorted())
            }
        }
    }

    fun importBatch(profiles: List<Profile>, onResult: (Int) -> Unit) {
        viewModelScope.launch {
            val added = repo.saveProfilesBatch(profiles)
            onResult(added)
        }
    }

    // ---- Connection actions ----

    fun connect() {
        val p = activeProfile.value
        if (p == null) {
            _connection.value = _connection.value.copy(statusMessage = "Add a profile first")
            return
        }
        val s = settings.value
        viewModelScope.launch {
            _connection.value = _connection.value.copy(statusMessage = "Connecting…")
            val ev = when (s.tunnelModeEnum()) {
                TunnelMode.SYSTEM_VPN -> AppEvent.StartSystemVpn(
                    profile = p,
                    perAppMode = s.perAppModeEnum(),
                    perAppPackages = s.perAppPackages,
                )
                TunnelMode.SOCKS5_ONLY -> AppEvent.StartSocksOnly(p)
            }
            _events.send(ev)
        }
    }

    fun disconnect() {
        viewModelScope.launch {
            _connection.value = _connection.value.copy(statusMessage = "Disconnecting…")
            _events.send(AppEvent.Stop(settings.value.tunnelModeEnum()))
        }
    }

    fun reportActivityError(msg: String) {
        _connection.value = _connection.value.copy(statusMessage = msg)
    }

    companion object {
        fun factory(app: Application) = object : ViewModelProvider.Factory {
            @Suppress("UNCHECKED_CAST")
            override fun <T : ViewModel> create(modelClass: Class<T>): T = AppViewModel(app) as T
        }
    }
}
