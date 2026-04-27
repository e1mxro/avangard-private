package com.avangard.mobile.ui.screens

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.avangard.mobile.data.Profile
import com.avangard.mobile.qr.QrScanContract
import com.avangard.mobile.ui.AppViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileEditScreen(
    vm: AppViewModel,
    profileId: String?,
    onDone: () -> Unit,
) {
    val profiles by vm.profiles.collectAsState()
    val existing = profiles.firstOrNull { it.id == profileId }

    var name by rememberSaveable(profileId) { mutableStateOf(existing?.name.orEmpty()) }
    var uri by rememberSaveable(profileId) { mutableStateOf(existing?.uri.orEmpty()) }
    var transport by rememberSaveable(profileId) { mutableStateOf(existing?.transport ?: "tcp") }
    var error by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(profileId, existing) {
        if (existing != null) {
            name = existing.name
            uri = existing.uri
            transport = existing.transport
        }
    }

    val qrLauncher = rememberLauncherForActivityResult(QrScanContract()) { result ->
        if (!result.isNullOrBlank()) {
            uri = result
            // Auto-detect transport from QR-decoded URI.
            transport = if (result.contains("transport=quic") || result.contains("type=quic")) "quic" else transport
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(if (existing == null) "New profile" else "Edit profile") },
                actions = {
                    IconButton(onClick = { qrLauncher.launch(Unit) }) {
                        Icon(Icons.Outlined.QrCodeScanner, contentDescription = "Scan QR")
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text("Name (optional)") },
                placeholder = { Text("e.g. Frankfurt #1") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
            )
            OutlinedTextField(
                value = uri,
                onValueChange = { uri = it; error = null },
                label = { Text("avangard:// URI") },
                placeholder = { Text("avangard://uuid@host:443?...") },
                modifier = Modifier.fillMaxWidth(),
                minLines = 3,
                maxLines = 6,
                isError = error != null,
                supportingText = error?.let { { Text(it) } },
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(
                    selected = transport == "tcp",
                    onClick = { transport = "tcp" },
                    label = { Text("TCP / TLS") },
                )
                FilterChip(
                    selected = transport == "quic",
                    onClick = { transport = "quic" },
                    label = { Text("QUIC") },
                )
            }

            Spacer(Modifier.height(8.dp))

            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedButton(
                    onClick = onDone,
                    modifier = Modifier.weight(1f).height(48.dp),
                ) { Text("Cancel") }
                Button(
                    onClick = {
                        val trimmed = uri.trim()
                        if (!trimmed.startsWith("avangard://")) {
                            error = "Must start with avangard://"
                            return@Button
                        }
                        val toSave = (existing?.copy(
                            name = name.trim(),
                            uri = trimmed,
                            transport = transport,
                        )) ?: Profile(
                            name = name.trim(),
                            uri = trimmed,
                            transport = transport,
                        )
                        vm.saveProfile(toSave, makeActive = (existing == null))
                        onDone()
                    },
                    modifier = Modifier.weight(1f).height(48.dp),
                    enabled = uri.isNotBlank(),
                ) { Text(if (existing == null) "Save" else "Update") }
            }
        }
    }
}
