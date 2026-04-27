package com.avangard.mobile.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.avangard.mobile.data.Profile
import com.avangard.mobile.data.SubscriptionImporter
import com.avangard.mobile.ui.AppViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SubscriptionImportScreen(
    vm: AppViewModel,
    onDone: () -> Unit,
) {
    var url by rememberSaveable { mutableStateOf("") }
    var loading by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var result by remember { mutableStateOf<List<Profile>>(emptyList()) }
    var imported by remember { mutableStateOf<Int?>(null) }
    val scope = rememberCoroutineScope()

    Scaffold(
        topBar = { TopAppBar(title = { Text("Import subscription") }) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            OutlinedTextField(
                value = url,
                onValueChange = { url = it; error = null },
                label = { Text("Subscription URL") },
                placeholder = { Text("https://example.com/sub") },
                modifier = Modifier.fillMaxWidth(),
                isError = error != null,
                supportingText = error?.let { { Text(it) } },
                singleLine = true,
            )

            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedButton(
                    onClick = onDone,
                    modifier = Modifier.weight(1f).height(48.dp),
                ) { Text("Close") }
                Button(
                    onClick = {
                        loading = true
                        error = null
                        result = emptyList()
                        imported = null
                        scope.launch {
                            val res = withContext(Dispatchers.IO) {
                                SubscriptionImporter.fetch(url.trim())
                            }
                            loading = false
                            res.onSuccess { profiles ->
                                if (profiles.isEmpty()) {
                                    error = "No avangard:// URIs found in subscription body"
                                } else {
                                    result = profiles
                                }
                            }.onFailure { t ->
                                error = t.message ?: "Fetch failed"
                            }
                        }
                    },
                    enabled = url.isNotBlank() && !loading,
                    modifier = Modifier.weight(1f).height(48.dp),
                ) {
                    if (loading) {
                        CircularProgressIndicator(modifier = Modifier.height(20.dp))
                    } else {
                        Text("Fetch")
                    }
                }
            }

            if (result.isNotEmpty()) {
                Spacer(Modifier.height(4.dp))
                Text(
                    "Found ${result.size} profile(s)",
                    style = MaterialTheme.typography.titleMedium,
                )
                LazyColumn(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    items(result, key = { it.uri }) { p ->
                        Card(shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth()) {
                            Column(modifier = Modifier.padding(12.dp)) {
                                Text(p.displayName, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                Text(p.uri, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        }
                    }
                }
                Button(
                    onClick = {
                        vm.importBatch(result) { added ->
                            imported = added
                        }
                    },
                    modifier = Modifier.fillMaxWidth().height(48.dp),
                ) { Text("Import all") }
            }

            imported?.let { added ->
                Text(
                    if (added > 0) "Imported $added new profile(s). You can close this screen."
                    else "All profiles already exist; nothing imported.",
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
    }
}
