package lifecycle

import "sync"

// Closer makes a Runtime shutdown callback safe to invoke from both the Wails
// shutdown hook and a deferred fallback.
type Closer struct {
	once     sync.Once
	close    func() error
	closeErr error
}

func NewCloser(close func() error) *Closer {
	return &Closer{close: close}
}

func (c *Closer) Close() error {
	c.once.Do(func() {
		c.closeErr = c.close()
	})
	return c.closeErr
}
