//go:build !windows && !darwin && !linux

package service

import "fmt"

func installPlatform() error   { return fmt.Errorf("service install not supported on this OS") }
func uninstallPlatform() error { return fmt.Errorf("service uninstall not supported on this OS") }
func statusPlatform() (string, error) {
	return "unsupported platform", nil
}
