package logging

import (
	"fmt"
	"io"
	"sync/atomic"

	"github.com/redforks/life"
	"github.com/redforks/testing/reset"
)

// Log writer accept write request, resend to internal writer run in its own
// goroutine. Because of async, the log writer can do heavy IO work and don't
// affect caller performance.
//
// If caller generate too many log request in a very short time period that
// internal writer can not handle, AsyncLogWriter drop the log requests,
// generate a log like: `Too many logs, xxx logs lost'. So the memory won't
// fill up with log messages.
type asyncLogWriter struct {
	w      io.Writer
	ch     chan []byte
	failed int32 // -1: disabled because internal writer error, > 0 how many writes lost.
	closed int32 // 1 if `ch' chan closed

	exitCh chan struct{} // closed when write goroutine exit
}

// NewAsyncLogWriter create a new instance of AsyncLogWriter, wrap an internal log writer.
func NewAsyncLogWriter(w io.Writer) io.WriteCloser {
	// queue at most 500 write request, more will dropped
	r := &asyncLogWriter{w: w, ch: make(chan []byte, 500), exitCh: make(chan struct{})}
	go r.run()
	if !reset.TestMode() {
		life.Register("asyncLogWriter", nil, func() {
			_ = r.Close()
		})
		life.RegisterHook("CloseAsyncLogWriter", 0, life.OnAbort, func() {
			_ = r.Close()
		})
	}
	return r
}

// Implement io.Writer interface, relay the write request to internal log
// writer. The internal log writer run in its own goroutine. If internal writer
// reports an error, AsyncLogWriter write the error message to stderr, but
// never report through .Write() method, and drop all fowling log write requests.
func (w *asyncLogWriter) Write(p []byte) (n int, err error) {
	if atomic.LoadInt32(&w.closed) == 1 {
		return w.w.Write(p)
	}

	n = len(p)
	buf := make([]byte, len(p))
	copy(buf, p)
	if failed := atomic.LoadInt32(&w.failed); failed != -1 {
		select {
		case w.ch <- buf:
			if failed > 0 {
				for !atomic.CompareAndSwapInt32(&w.failed, failed, 0) {
					failed := atomic.LoadInt32(&w.failed)
					if failed == -1 {
						return
					}
				}
				if _, err := w.Write([]byte(fmt.Sprintf(`Too many logs, %d logs lost`, failed))); err != nil {
					w.handleInnerWriteError(err)
				}
			}
		default:
			atomic.AddInt32(&w.failed, 1)
		}
	}
	return
}

func (w *asyncLogWriter) handleInnerWriteError(err error) {
	atomic.StoreInt32(&w.failed, -1)
	w.drain()
}

// Close and flush asyncLogWriter buffer, latter .Write() request deliver to
// inner writer directly.
func (w *asyncLogWriter) Close() error {
	if atomic.CompareAndSwapInt32(&w.closed, 0, 1) {
		close(w.ch)
		<-w.exitCh
	}
	return nil
}

// Goroutine function for internal writer.
func (w *asyncLogWriter) run() {
	for buf := range w.ch {
		if _, err := w.w.Write(buf); err != nil {
			w.handleInnerWriteError(err)
			break
		}
	}

	close(w.exitCh)
}

// Drain all messages in channel
func (w *asyncLogWriter) drain() {
	for {
		select {
		case <-w.ch:
		default:
			return
		}
	}
}
