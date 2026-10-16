//go:build !windows

// gbr-tray is a Windows-only system-tray supervisor for gbr-agent. On other
// platforms it builds to a no-op so `go test ./...` and cross-builds stay green.
package main

func main() {}
