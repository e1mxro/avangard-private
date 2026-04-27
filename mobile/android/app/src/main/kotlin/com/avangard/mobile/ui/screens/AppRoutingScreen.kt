package com.avangard.mobile.ui.screens

import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import com.avangard.mobile.data.PerAppMode
import com.avangard.mobile.ui.AppViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private data class InstalledApp(
    val packageName: String,
    val label: String,
    val icon: android.graphics.drawable.Drawable?,
    val isSystem: Boolean,
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AppRoutingScreen(
    vm: AppViewModel,
    onDone: () -> Unit,
) {
    val ctx = LocalContext.current
    val pm = remember { ctx.packageManager }
    val settings by vm.settings.collectAsState()
    val mode = settings.perAppModeEnum()

    var apps by remember { mutableStateOf<List<InstalledApp>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var query by remember { mutableStateOf("") }
    var selected by remember { mutableStateOf<Set<String>>(settings.perAppPackages.toSet()) }
    var includeSystem by remember { mutableStateOf(false) }

    LaunchedEffect(settings.perAppPackages) {
        selected = settings.perAppPackages.toSet()
    }

    LaunchedEffect(Unit) {
        loading = true
        val list = withContext(Dispatchers.IO) { loadApps(pm) }
        apps = list
        loading = false
    }

    val filtered = remember(apps, query, includeSystem) {
        apps.asSequence()
            .filter { includeSystem || !it.isSystem }
            .filter { query.isBlank() || it.label.contains(query, ignoreCase = true) || it.packageName.contains(query, ignoreCase = true) }
            .sortedBy { it.label.lowercase() }
            .toList()
    }

    Scaffold(
        topBar = { TopAppBar(title = { Text("Per-app routing") }) },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            Column(
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                    val modes = PerAppMode.entries
                    modes.forEachIndexed { idx, m ->
                        SegmentedButton(
                            selected = mode == m,
                            onClick = { vm.setPerApp(m, selected.toList()) },
                            shape = SegmentedButtonDefaults.itemShape(idx, modes.size),
                        ) {
                            Text(when (m) {
                                PerAppMode.ALL -> "All apps"
                                PerAppMode.ALLOW -> "Only selected"
                                PerAppMode.DISALLOW -> "Bypass selected"
                            })
                        }
                    }
                }
                Text(
                    when (mode) {
                        PerAppMode.ALL -> "Every app on the device is routed through AVANGARD."
                        PerAppMode.ALLOW -> "Only the apps you check below go through AVANGARD; everything else uses the direct connection."
                        PerAppMode.DISALLOW -> "Every app uses AVANGARD except the ones you check below (e.g. banking apps that you want on the cellular network)."
                    },
                    style = MaterialTheme.typography.bodySmall,
                )
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    label = { Text("Search") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(checked = includeSystem, onCheckedChange = { includeSystem = it })
                    Text("Show system apps", style = MaterialTheme.typography.bodyMedium)
                }
            }

            HorizontalDivider()

            if (loading) {
                Column(
                    modifier = Modifier.fillMaxSize(),
                    verticalArrangement = Arrangement.Center,
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    CircularProgressIndicator()
                    Spacer(Modifier.height(8.dp))
                    Text("Loading apps…")
                }
            } else {
                LazyColumn(modifier = Modifier.weight(1f)) {
                    items(filtered, key = { it.packageName }) { app ->
                        AppRow(
                            app = app,
                            checked = app.packageName in selected,
                            enabled = mode != PerAppMode.ALL,
                            onToggle = { isOn ->
                                val newSet = if (isOn) selected + app.packageName
                                else selected - app.packageName
                                selected = newSet
                                vm.setPerApp(mode, newSet.toList())
                            },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun AppRow(
    app: InstalledApp,
    checked: Boolean,
    enabled: Boolean,
    onToggle: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = enabled) { onToggle(!checked) }
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (app.icon != null) {
            AndroidView(
                modifier = Modifier.size(36.dp),
                factory = { ctx ->
                    android.widget.ImageView(ctx).apply {
                        setImageDrawable(app.icon)
                    }
                },
                update = { it.setImageDrawable(app.icon) },
            )
        } else {
            Spacer(Modifier.size(36.dp))
        }
        Spacer(Modifier.size(12.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(app.label, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(app.packageName, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        Checkbox(checked = checked, onCheckedChange = onToggle, enabled = enabled)
    }
}

private fun loadApps(pm: PackageManager): List<InstalledApp> {
    @Suppress("DEPRECATION")
    val list = pm.getInstalledApplications(0)
    return list.map { info ->
        val isSystem = (info.flags and ApplicationInfo.FLAG_SYSTEM) != 0
        InstalledApp(
            packageName = info.packageName,
            label = pm.getApplicationLabel(info).toString(),
            icon = runCatching { pm.getApplicationIcon(info) }.getOrNull(),
            isSystem = isSystem,
        )
    }
}
