//go:build linux

package linux

// typeWindowArgs is the helper argv that types into an already-known X11 window.
//
// Jeff's working hand command is:
//
//	xdotool type --window $WID "text"
//
// Keep this exact shape. Do not insert a bare "--" before the payload — older
// xdotool treats "--" as the typed string, exits 0, and never sends the command.
// --delay is omitted: 1ms is xdotool's default and changes the argv Jeff used.
func typeWindowArgs(windowID, text string) []string {
	return []string{"type", "--window", windowID, text}
}
