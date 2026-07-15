//go:build !windows && !darwin && !linux

package inject

// platformInjector returns a no-op injector on unsupported GOOS.
func platformInjector() Injector {
	return &noopInjector{}
}

type noopInjector struct{}

func (n *noopInjector) Discover() ([]TerminalWindow, error) { return nil, ErrNotSupported }

func (n *noopInjector) Inject(sessionID string, req InjectRequest) error {
	if err := ValidateRequest(sessionID, req); err != nil {
		return err
	}
	return ErrNotSupported
}

func (n *noopInjector) Capture(sessionID string) (CaptureResult, error) {
	return CaptureResult{Partial: true, Method: "none", Note: "unsupported platform"}, ErrNotSupported
}

func (n *noopInjector) Bind(sessionID string, win TerminalWindow) error {
	if sessionID == "" {
		return ErrEmptySession
	}
	return ErrNotSupported
}

func (n *noopInjector) Unbind(sessionID string) {}

func (n *noopInjector) Close() error { return nil }

var _ Injector = (*noopInjector)(nil)
