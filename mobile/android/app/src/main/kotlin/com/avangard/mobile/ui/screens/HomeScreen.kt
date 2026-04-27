package com.avangard.mobile.ui.screens

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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.SwapHoriz
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.avangard.mobile.data.TunnelMode
import com.avangard.mobile.ui.AppViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    vm: AppViewModel,
    onAddProfile: () -> Unit,
    onPickProfile: () -> Unit,
) {
    val connection by vm.connection.collectAsState()
    val active by vm.activeProfile.collectAsState()
    val settings by vm.settings.collectAsState()
    val mode = settings.tunnelModeEnum()

    androidx.compose.material3.Scaffold(
        topBar = { TopAppBar(title = { Text("AVANGARD") }) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            // Status card.
            Card(shape = RoundedCornerShape(16.dp)) {
                Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = if (connection.running) "Connected" else "Disconnected",
                        style = MaterialTheme.typography.headlineSmall,
                    )
                    Text(connection.statusMessage, style = MaterialTheme.typography.bodyMedium)
                }
            }

            // Active-profile card.
            Card(shape = RoundedCornerShape(16.dp)) {
                Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("Active profile", style = MaterialTheme.typography.labelLarge)
                    if (active == null) {
                        Text(
                            "No profile saved yet. Add an avangard:// URI to get started.",
                            style = MaterialTheme.typography.bodyMedium,
                        )
                        Spacer(Modifier.height(4.dp))
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Button(onClick = onAddProfile) {
                                Icon(Icons.Outlined.Add, contentDescription = null)
                                Spacer(Modifier.height(0.dp))
                                Text("  Add profile")
                            }
                        }
                    } else {
                        Text(active!!.displayName, style = MaterialTheme.typography.titleMedium)
                        Text(
                            "Transport: ${active!!.transport.uppercase()}",
                            style = MaterialTheme.typography.bodySmall,
                        )
                        Spacer(Modifier.height(4.dp))
                        Row(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            OutlinedButton(onClick = onPickProfile, enabled = !connection.running) {
                                Icon(Icons.Outlined.SwapHoriz, contentDescription = null)
                                Text("  Switch")
                            }
                        }
                    }
                }
            }

            // Tunnel mode toggle.
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text("Mode", style = MaterialTheme.typography.labelLarge)
                SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                    SegmentedButton(
                        selected = mode == TunnelMode.SYSTEM_VPN,
                        onClick = { vm.setTunnelMode(TunnelMode.SYSTEM_VPN) },
                        enabled = !connection.running,
                        shape = SegmentedButtonDefaults.itemShape(0, 2),
                    ) { Text("System VPN") }
                    SegmentedButton(
                        selected = mode == TunnelMode.SOCKS5_ONLY,
                        onClick = { vm.setTunnelMode(TunnelMode.SOCKS5_ONLY) },
                        enabled = !connection.running,
                        shape = SegmentedButtonDefaults.itemShape(1, 2),
                    ) { Text("SOCKS5 only") }
                }
                Text(
                    when (mode) {
                        TunnelMode.SYSTEM_VPN ->
                            "Tunnels every app on the device. Per-app routing is configured in Settings."
                        TunnelMode.SOCKS5_ONLY ->
                            "Local SOCKS5 proxy on 127.0.0.1:18964. Apps must be configured manually."
                    },
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            Spacer(Modifier.height(8.dp))

            // Connect / Disconnect.
            if (!connection.running) {
                Button(
                    onClick = vm::connect,
                    enabled = active != null,
                    modifier = Modifier.fillMaxWidth().height(56.dp),
                ) { Text("Connect") }
            } else {
                Button(
                    onClick = vm::disconnect,
                    modifier = Modifier.fillMaxWidth().height(56.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error),
                ) { Text("Disconnect") }
            }
        }
    }
}
