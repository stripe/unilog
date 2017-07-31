package unilog

import (
	"io"
	"sync"
)

// UnilogReader wraps an existing io.Reader and adds the ability to
// perform a graceful shutdown by reading from the underlying Reader
// up through the next newline character, and then start returning
// EOF.
type UnilogReader struct {
	inner io.Reader
	// Was the last character read a newline?
	nl           bool
	shuttingDown bool
	mtx          sync.Mutex
}

// Creates a new UnilogReader. Once shutdown becomes readable, the
// returned Reader will start reading from the underlying Reader one
// byte at a time until newline, and then start returning EOF.
func NewUnilogReader(in io.Reader, shutdown <-chan struct{}) io.Reader {
	r := &UnilogReader{inner: in}
	go func() {
		<-shutdown
		r.mtx.Lock()
		r.shuttingDown = true
		r.mtx.Unlock()
	}()
	return r
}

func (r *UnilogReader) isShuttingDown() bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.shuttingDown
}

func (r *UnilogReader) Read(buf []byte) (int, error) {
	if r.nl && r.isShuttingDown() {
		return 0, io.EOF
	}

	n, e := r.inner.Read(buf)
	if n > 0 {
		r.nl = buf[n-1] == '\n'
	}
	return n, e
}
