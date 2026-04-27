package com.avangard.mobile.ui

import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.List
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.avangard.mobile.ui.screens.AppRoutingScreen
import com.avangard.mobile.ui.screens.HomeScreen
import com.avangard.mobile.ui.screens.ProfileEditScreen
import com.avangard.mobile.ui.screens.ProfilesScreen
import com.avangard.mobile.ui.screens.SettingsScreen
import com.avangard.mobile.ui.screens.SubscriptionImportScreen

object Routes {
    const val HOME = "home"
    const val PROFILES = "profiles"
    const val PROFILE_EDIT = "profile_edit"
    const val PROFILE_EDIT_ROUTE = "profile_edit?id={id}"
    const val SUBSCRIPTION_IMPORT = "subscription_import"
    const val SETTINGS = "settings"
    const val APP_ROUTING = "app_routing"

    fun profileEdit(id: String? = null): String =
        if (id.isNullOrBlank()) "$PROFILE_EDIT?id=" else "$PROFILE_EDIT?id=$id"
}

private data class TabItem(val route: String, val label: String, val icon: ImageVector)

private val Tabs = listOf(
    TabItem(Routes.HOME, "Home", Icons.Outlined.Home),
    TabItem(Routes.PROFILES, "Profiles", Icons.Outlined.List),
    TabItem(Routes.SETTINGS, "Settings", Icons.Outlined.Settings),
)

@Composable
fun AppNav(vm: AppViewModel) {
    val navController: NavHostController = rememberNavController()
    val backStack by navController.currentBackStackEntryAsState()
    val currentRoute = backStack?.destination?.route
    val showBottomBar = currentRoute in setOf(Routes.HOME, Routes.PROFILES, Routes.SETTINGS)

    Scaffold(
        bottomBar = {
            if (showBottomBar) {
                NavigationBar {
                    Tabs.forEach { tab ->
                        NavigationBarItem(
                            selected = currentRoute == tab.route,
                            onClick = {
                                if (currentRoute != tab.route) {
                                    navController.navigate(tab.route) {
                                        popUpTo(Routes.HOME) { inclusive = false }
                                        launchSingleTop = true
                                    }
                                }
                            },
                            icon = { Icon(tab.icon, contentDescription = tab.label) },
                            label = { Text(tab.label) },
                        )
                    }
                }
            }
        },
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = Routes.HOME,
            modifier = Modifier.padding(padding),
        ) {
            composable(Routes.HOME) {
                HomeScreen(
                    vm = vm,
                    onAddProfile = { navController.navigate(Routes.profileEdit()) },
                    onPickProfile = { navController.navigate(Routes.PROFILES) },
                )
            }
            composable(Routes.PROFILES) {
                ProfilesScreen(
                    vm = vm,
                    onEditProfile = { id -> navController.navigate(Routes.profileEdit(id)) },
                    onAddProfile = { navController.navigate(Routes.profileEdit()) },
                    onImportSubscription = { navController.navigate(Routes.SUBSCRIPTION_IMPORT) },
                )
            }
            composable(
                Routes.PROFILE_EDIT_ROUTE,
                arguments = listOf(
                    navArgument("id") {
                        type = NavType.StringType
                        nullable = true
                        defaultValue = null
                    },
                ),
            ) { entry ->
                val id = entry.arguments?.getString("id")?.takeIf { it.isNotBlank() }
                ProfileEditScreen(
                    vm = vm,
                    profileId = id,
                    onDone = { navController.popBackStack() },
                )
            }
            composable(Routes.SUBSCRIPTION_IMPORT) {
                SubscriptionImportScreen(
                    vm = vm,
                    onDone = { navController.popBackStack() },
                )
            }
            composable(Routes.SETTINGS) {
                SettingsScreen(
                    vm = vm,
                    onOpenAppRouting = { navController.navigate(Routes.APP_ROUTING) },
                )
            }
            composable(Routes.APP_ROUTING) {
                AppRoutingScreen(
                    vm = vm,
                    onDone = { navController.popBackStack() },
                )
            }
        }
    }
}
