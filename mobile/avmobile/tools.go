//go:build tools
// +build tools

// This file forces `golang.org/x/mobile/bind` (and the bound JNI helpers it
// pulls in) into the module graph. The package is required at build time by
// `gomobile bind` but its source is only compiled with a specific GOOS/GOARCH
// combination, so a plain `go get` from a Linux host without explicit imports
// silently filters it out. The `tools` build tag prevents the imports from
// affecting normal builds.
package avmobile

import (
	_ "golang.org/x/mobile/bind"
	_ "golang.org/x/mobile/bind/java"
)
