package unilog

import "io"

// UnilogReader wraps an existing io.Reader and adds the ability to
// perform a graceful shutdown by reading from the underlying Reader
// up through the next newline character, and then start returning
// EOF.
type UnilogReader struct {
	inner io.Reader
	// Was the last character read a newline?
	nl           bool
	shuttingDown bool
}

// Creates a new UnilogReader. Once shutdown becomes readable, the
// returned Reader will start reading from the underlying Reader one
// byte at a time until newline, and then start returning EOF.
func NewUnilogReader(in io.Reader, shutdown <-chan struct{}) io.Reader {
	r := &UnilogReader{inner: in}
	go func() {
		<-shutdown
		r.shuttingDown = true
	}()
	return r
}

func (u *UnilogReader) Read(buf []byte) (int, error) {
	if u.nl && u.shuttingDown {
		return 0, io.EOF
	}

	if u.shuttingDown {
		buf = buf[:1]
	}

	n, e := u.inner.Read(buf)
	if n > 0 {
		u.nl = buf[n-1] == '\n'
	}
	return n, e
}
