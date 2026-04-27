package com.avangard.mobile.data

import android.app.Application
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json

private val Application.prefs: DataStore<Preferences> by preferencesDataStore(name = "avangard-prefs")

/**
 * Single source of truth for the user-visible state: the list of saved
 * [Profile]s and the global [AppSettings] (active profile, theme, per-app
 * routing, …). Backed by Preferences DataStore with JSON-serialized blobs
 * so we don't need to maintain a Room schema for v0.3.
 */
class AppRepository private constructor(private val app: Application) {

    private val json = Json {
        ignoreUnknownKeys = true
        encodeDefaults = true
    }

    val profiles: Flow<List<Profile>> = app.prefs.data.map { prefs ->
        prefs[KEY_PROFILES_JSON]?.let { decodeProfiles(it) } ?: emptyList()
    }

    val settings: Flow<AppSettings> = app.prefs.data.map { prefs ->
        prefs[KEY_SETTINGS_JSON]?.let { decodeSettings(it) } ?: AppSettings()
    }

    suspend fun snapshotProfiles(): List<Profile> = profiles.first()
    suspend fun snapshotSettings(): AppSettings = settings.first()

    suspend fun saveProfile(profile: Profile) {
        val current = snapshotProfiles().toMutableList()
        val idx = current.indexOfFirst { it.id == profile.id }
        if (idx >= 0) current[idx] = profile else current.add(profile)
        writeProfiles(current)
    }

    suspend fun saveProfilesBatch(toAdd: List<Profile>): Int {
        if (toAdd.isEmpty()) return 0
        val current = snapshotProfiles().toMutableList()
        val existingUris = current.map { it.uri }.toHashSet()
        var added = 0
        for (p in toAdd) {
            if (p.uri.isBlank() || p.uri in existingUris) continue
            current.add(p)
            existingUris += p.uri
            added++
        }
        writeProfiles(current)
        return added
    }

    suspend fun deleteProfile(id: String) {
        val current = snapshotProfiles().filterNot { it.id == id }
        writeProfiles(current)
        // If we just deleted the active profile, fall back to first or null.
        val s = snapshotSettings()
        if (s.activeProfileId == id) {
            updateSettings { it.copy(activeProfileId = current.firstOrNull()?.id) }
        }
    }

    suspend fun setActive(id: String) {
        updateSettings { it.copy(activeProfileId = id) }
    }

    suspend fun updateSettings(transform: (AppSettings) -> AppSettings) {
        val updated = transform(snapshotSettings())
        app.prefs.edit { it[KEY_SETTINGS_JSON] = json.encodeToString(AppSettings.serializer(), updated) }
    }

    private suspend fun writeProfiles(list: List<Profile>) {
        app.prefs.edit {
            it[KEY_PROFILES_JSON] = json.encodeToString(ListSerializer(Profile.serializer()), list)
        }
    }

    private fun decodeProfiles(s: String): List<Profile> = try {
        json.decodeFromString(ListSerializer(Profile.serializer()), s)
    } catch (_: Throwable) {
        emptyList()
    }

    private fun decodeSettings(s: String): AppSettings = try {
        json.decodeFromString(AppSettings.serializer(), s)
    } catch (_: Throwable) {
        AppSettings()
    }

    companion object {
        private val KEY_PROFILES_JSON = stringPreferencesKey("profiles_json_v1")
        private val KEY_SETTINGS_JSON = stringPreferencesKey("settings_json_v1")

        @Volatile
        private var instance: AppRepository? = null

        fun get(app: Application): AppRepository =
            instance ?: synchronized(this) {
                instance ?: AppRepository(app).also { instance = it }
            }
    }
}
