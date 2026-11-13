package main

import (
	"os"
	"strconv"
	"strings"
)

func trayDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GBR_TRAY"))) {
	case "0", "off", "false", "no":
		return true
	default:
		return false
	}
}

func trayPort(args []string) int {
	p := botPortFromEnv()
	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		if a == "-bot-port" && i+1 < len(args) {
			if n, err := strconv.Atoi(strings.TrimSpace(args[i+1])); err == nil && n >= 0 {
				return n
			}
		}
		if strings.HasPrefix(a, "-bot-port=") {
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "-bot-port=")); err == nil && n >= 0 {
				return n
			}
		}
	}
	return p
}
