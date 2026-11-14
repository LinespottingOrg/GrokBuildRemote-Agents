//go:build darwin || linux

package main

import _ "embed"

//go:embed assets/icon-256.png
var trayIconPNG []byte
