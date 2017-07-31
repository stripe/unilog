package unilog

import (
	"io"
	"sync"
)

// Reader wraps an existing io.Reader and adds the ability to
// perform a graceful shutdown by reading from the underlying Reader
// up through the next newline character, and then start returning
// EOF.
type Reader struct {
	inner io.Reader
	// Was the last character read a newline?
	nl           bool
	shuttingDown bool
	mtx          sync.Mutex
}

// NewReader creates a new Reader for use in Unilog. Once shutdown becomes readable, the
// returned Reader will start reading from the underlying Reader one
// byte at a time until newline, and then start returning EOF.
func NewReader(in io.Reader, shutdown <-chan struct{}) io.Reader {
	r := &Reader{inner: in}
	go func() {
		<-shutdown
		r.mtx.Lock()
		r.shuttingDown = true
		r.mtx.Unlock()
	}()
	return r
}

func (r *Reader) isShuttingDown() bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.shuttingDown
}

func (r *Reader) Read(buf []byte) (int, error) {
	if r.nl && r.isShuttingDown() {
		return 0, io.EOF
	}

	n, e := r.inner.Read(buf)
	if n > 0 {
		r.nl = buf[n-1] == '\n'
	}
	return n, e
}
