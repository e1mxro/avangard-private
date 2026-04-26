pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
        flatDir {
            // gomobile bind drops avmobile.aar here. Resolved as
            // implementation(":avmobile@aar") in app/build.gradle.kts.
            dirs("${rootDir}/app/libs")
        }
    }
}
rootProject.name = "AvangardMobile"
include(":app")
