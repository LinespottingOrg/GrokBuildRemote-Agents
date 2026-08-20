//go:build !windows

package inject

func (s *ManagedSession) startConPTYLocked() error {
	return ErrNotSupported
}

type winConPTY struct{}

func (c *winConPTY) wait()  {}
func (c *winConPTY) close() {}
