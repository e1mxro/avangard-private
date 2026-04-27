package com.avangard.mobile.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(vm: HomeViewModel) {
    val state by vm.state.collectAsState()
    Scaffold(
        topBar = {
            TopAppBar(title = { Text("AVANGARD") })
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Card(shape = RoundedCornerShape(16.dp)) {
                Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = if (state.running) "Connected" else "Disconnected",
                        style = MaterialTheme.typography.titleLarge,
                    )
                    Text(state.statusMessage, style = MaterialTheme.typography.bodyMedium)
                }
            }

            // Tunnel mode: system-wide VPN (captures all device traffic) vs SOCKS5 only.
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text("Mode", style = MaterialTheme.typography.labelLarge)
                SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                    SegmentedButton(
                        selected = state.mode == TunnelMode.SYSTEM_VPN,
                        onClick = { vm.onModeChanged(TunnelMode.SYSTEM_VPN) },
                        enabled = !state.running,
                        shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
                    ) { Text("System VPN") }
                    SegmentedButton(
                        selected = state.mode == TunnelMode.SOCKS5_ONLY,
                        onClick = { vm.onModeChanged(TunnelMode.SOCKS5_ONLY) },
                        enabled = !state.running,
                        shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
                    ) { Text("SOCKS5 only") }
                }
                Text(
                    when (state.mode) {
                        TunnelMode.SYSTEM_VPN ->
                            "Tunnels every app on the device. Android will ask for VPN consent on first connect."
                        TunnelMode.SOCKS5_ONLY ->
                            "Exposes a local SOCKS5 proxy on 127.0.0.1:18964. Apps must be configured manually."
                    },
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            OutlinedTextField(
                value = state.uri,
                onValueChange = vm::onUriChanged,
                label = { Text("avangard:// URI") },
                placeholder = { Text("avangard://uuid@host:443?...") },
                modifier = Modifier.fillMaxWidth(),
                minLines = 3,
                maxLines = 6,
                enabled = !state.running,
            )

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(
                    selected = state.transport == "tcp",
                    onClick = { vm.onTransportChanged("tcp") },
                    enabled = !state.running,
                    label = { Text("TCP / TLS") },
                )
                FilterChip(
                    selected = state.transport == "quic",
                    onClick = { vm.onTransportChanged("quic") },
                    enabled = !state.running,
                    label = { Text("QUIC") },
                )
            }

            Spacer(Modifier.height(8.dp))

            if (!state.running) {
                Button(
                    onClick = vm::connect,
                    modifier = Modifier.fillMaxWidth().height(56.dp),
                    enabled = state.uri.isNotBlank(),
                ) { Text("Connect") }
            } else {
                Button(
                    onClick = vm::disconnect,
                    modifier = Modifier.fillMaxWidth().height(56.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error),
                ) { Text("Disconnect") }
            }

            Card(shape = RoundedCornerShape(16.dp)) {
                Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text("Как пользоваться", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "System VPN — Android покажет диалог «Connection request». " +
                            "Нажми «OK», и весь трафик устройства автоматически пойдёт через AVANGARD. " +
                            "Никаких настроек в отдельных приложениях не нужно.\n\n" +
                            "SOCKS5 only — для отладки или ручной настройки. После подключения " +
                            "укажи 127.0.0.1:18964 в настройках прокси нужного приложения.",
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
            }
        }
    }
}
