package respwriter

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/miekg/dns"
)

// NewHandlerFunc returns a new dns.HandlerFunc that wraps the given
// handler with a RespWriter. The returned handler will use the given logger
// and requestTimeout to create the RespWriter. Options supported: WithLogger
func NewHandlerFunc(requestTimeout time.Duration, h dns.HandlerFunc, opt ...Option) (dns.HandlerFunc, error) {
	const op = "handlers.NewRespWriterHandler"
	switch {
	case requestTimeout <= 0:
		return nil, fmt.Errorf("%s: invalid request timeout: %w", op, ErrInvalidParameter)
	case isNil(h):
		return nil, fmt.Errorf("%s: nil handler: %w", op, ErrInvalidParameter)
	}
	return func(w dns.ResponseWriter, r *dns.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		wrappedWriter := NewRespWriter(ctx, w, opt...)
		h(wrappedWriter, r)
	}, nil
}

// RespWriter is a wrapper around dns.ResponseWriter that provides "base"
// capabilities for the wrapped writer. Among other things, this is useful for
// ensuring that the wrapped writer is not used after the context is canceled.
type RespWriter struct {
	// underlying is the wrapped dns.ResponseWriter.  We need an explicit field
	// here for the underlying wrapped writer so we can perform type assertions
	// on the underlying writer to access the underlying connections via the
	// dns.ExposesUnderlyingConns interface.
	underlying dns.ResponseWriter

	// requestCtx is the context for the request and will have a timeout set.
	// It's important to only use this context for the duration of the request
	// and not for things which may outlive the request.
	requestCtx context.Context

	// logger is the logger to use for logging during the request.
	logger *slog.Logger
}

// NewRespWriter returns a new RespWriter that wraps the given dns.ResponseWriter.
func NewRespWriter(ctx context.Context, w dns.ResponseWriter, opt ...Option) *RespWriter {
	switch {
	case isNil(ctx):
		panic("nil context")
	case isNil(w):
		panic("nil dns.ResponseWriter")
	}
	opts := getGeneralOpts(opt...)
	return &RespWriter{
		requestCtx: ctx,
		logger:     opts.withLogger,
		underlying: w,
	}
}

// WriteMsg writes a DNS message to the client. If the ctx is done, it returns
// the ctx error.
func (rw *RespWriter) WriteMsg(msg *dns.Msg) error {
	select {
	case <-rw.requestCtx.Done():
		return rw.requestCtx.Err()
	default:
		return rw.underlying.WriteMsg(msg)
	}
}

// Write writes a raw buffer to the client. If the ctx is done, it returns
// the ctx error.
func (rw *RespWriter) Write([]byte) (int, error) {
	select {
	case <-rw.requestCtx.Done():
		return 0, rw.requestCtx.Err()
	default:
		return rw.underlying.Write([]byte{})
	}
}

// RemoteAddr returns the remote address of the client.
func (rw *RespWriter) RemoteAddr() net.Addr {
	return rw.underlying.RemoteAddr()
}

// LocalAddr returns the local address of the server.
func (rw *RespWriter) LocalAddr() net.Addr {
	return rw.underlying.LocalAddr()
}

// TsigStatus returns the Tsig status of the message.
func (rw *RespWriter) TsigStatus() error {
	select {
	case <-rw.requestCtx.Done():
		return rw.requestCtx.Err()
	default:
		return rw.underlying.TsigStatus()
	}
}

// TsigTimersOnly sets the Tsig timers only flag on the message.
func (rw *RespWriter) TsigTimersOnly(b bool) {
	rw.underlying.TsigTimersOnly(b)
}

// Hijack hijacks the underlying connection.
func (rw *RespWriter) Hijack() {
	rw.underlying.Hijack()
}

// Close closes the underlying connection.
func (rw *RespWriter) Close() error {
	return rw.underlying.Close()
}

// Underlying returns the underlying dns.ResponseWriter
func (rw *RespWriter) Underlying() dns.ResponseWriter {
	return rw.underlying
}

// RequestContext returns the context for the request.
func (rw *RespWriter) RequestContext() context.Context {
	return rw.requestCtx
}

// Logger returns the logger to use for logging during the request.
func (rw *RespWriter) Logger() *slog.Logger {
	return rw.logger
}

// SetLogger sets the logger to use for logging during the request which allows
// you to override the logger passed to NewRespWriter(...)
func (rw *RespWriter) SetLogger(logger *slog.Logger) {
	rw.logger = logger
}
