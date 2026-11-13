//go:build linux || (darwin && cgo)

package main

import _ "embed"

//go:embed assets/icon-256.png
var trayIconPNG []byte
