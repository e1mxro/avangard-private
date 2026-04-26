pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    // PREFER_PROJECT lets the :app subproject add its own flatDir for the
    // gomobile-built AAR while keeping google()/mavenCentral() the default.
    repositoriesMode.set(RepositoriesMode.PREFER_PROJECT)
    repositories {
        google()
        mavenCentral()
    }
}
rootProject.name = "AvangardMobile"
include(":app")
