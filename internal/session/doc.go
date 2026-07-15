// Package session implements Grok Build Remote session discovery and naming.
//
// Import path:
//
//	github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/session
//
// Session ID resolution priority (protocol v1):
//  1. First line of .grok-session in the session cwd
//  2. Explicit rename map in %USERPROFILE%\.gbr\sessions.json
//  3. Fallback slug from folder name, or folder + git remote repo name
//
// Core integration (A1):
//
//	store, err := session.OpenStore("") // default ~/.gbr/sessions.json
//	reg := session.NewRegistry()
//	sc := session.NewScanner(store, reg, func(ctx context.Context) ([]session.Candidate, error) {
//	    // platform inject layer supplies live terminals
//	    return candidates, nil
//	})
//	sc.Interval = 5 * time.Second // default
//	go sc.Run(ctx)
//
//	// After each scan (or on demand), publish registers:
//	for _, s := range reg.List() {
//	    msg := s.ToRegister(deviceID)
//	    // send msg via Grok relay client
//	}
//
//	// Rename command handler:
//	_ = store.Rename(cwd, "global-edition")
//	// or: sess, err := sc.Rename(cwd, "global-edition")
//
// This package is pure Go (stdlib only). It does not own inject or main.
package session
