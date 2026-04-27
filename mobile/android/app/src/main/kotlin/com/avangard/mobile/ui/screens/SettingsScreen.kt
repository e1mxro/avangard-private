package com.avangard.mobile.ui.screens

import androidx.compose.foundation.clickable
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
import androidx.compose.material.icons.outlined.ChevronRight
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
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
import com.avangard.mobile.data.PerAppMode
import com.avangard.mobile.data.ThemeMode
import com.avangard.mobile.ui.AppViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    vm: AppViewModel,
    onOpenAppRouting: () -> Unit,
) {
    val settings by vm.settings.collectAsState()
    val theme = settings.themeModeEnum()
    val perApp = settings.perAppModeEnum()
    val perAppCount = settings.perAppPackages.size

    Scaffold(
        topBar = { TopAppBar(title = { Text("Settings") }) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            // Theme picker.
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text("Theme", style = MaterialTheme.typography.titleMedium)
                SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                    val modes = ThemeMode.entries
                    modes.forEachIndexed { idx, m ->
                        SegmentedButton(
                            selected = theme == m,
                            onClick = { vm.setThemeMode(m) },
                            shape = SegmentedButtonDefaults.itemShape(idx, modes.size),
                        ) {
                            Text(
                                when (m) {
                                    ThemeMode.SYSTEM -> "System"
                                    ThemeMode.LIGHT -> "Light"
                                    ThemeMode.DARK -> "Dark"
                                    ThemeMode.AMOLED -> "AMOLED"
                                },
                            )
                        }
                    }
                }
                Text(
                    when (theme) {
                        ThemeMode.SYSTEM -> "Follows the device-wide light / dark setting."
                        ThemeMode.LIGHT -> "Always light, regardless of system."
                        ThemeMode.DARK -> "Always dark, regardless of system."
                        ThemeMode.AMOLED -> "Pure-black background — saves battery on OLED."
                    },
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            // Per-app routing entry.
            Card(
                shape = RoundedCornerShape(16.dp),
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable(onClick = onOpenAppRouting),
            ) {
                Row(
                    modifier = Modifier.padding(16.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text("Per-app routing", style = MaterialTheme.typography.titleMedium)
                        Text(
                            when (perApp) {
                                PerAppMode.ALL -> "All apps tunneled (default)"
                                PerAppMode.ALLOW -> "Tunnel only $perAppCount selected app(s)"
                                PerAppMode.DISALLOW -> "Bypass $perAppCount selected app(s)"
                            },
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                    Icon(Icons.Outlined.ChevronRight, contentDescription = null)
                }
            }

            // About card.
            Card(shape = RoundedCornerShape(16.dp)) {
                Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text("About", style = MaterialTheme.typography.titleMedium)
                    Text("AVANGARD Mobile · v0.3.0-alpha", style = MaterialTheme.typography.bodyMedium)
                    Spacer(Modifier.height(4.dp))
                    Text(
                        "System-wide tunneling via Android VpnService + tun2socks. " +
                            "Custom AVANGARD protocol for resilient transport.",
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
            }
        }
    }
}
