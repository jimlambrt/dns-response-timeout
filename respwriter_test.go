package respwriter

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRespWriter(t *testing.T) {
	t.Parallel()
	testCtx := context.Background()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))

	type mockDNSResponseWriter struct {
		dns.ResponseWriter
	}
	w := new(mockDNSResponseWriter)

	// Test when dns.ResponseWriter is nil
	assert.PanicsWithValue(t, "nil dns.ResponseWriter", func() {
		NewRespWriter(testCtx, nil, WithLogger(testLogger))
	})

	// Test when both logger and dns.ResponseWriter are not nil
	respWriter := NewRespWriter(testCtx, w, WithLogger(testLogger))
	assert.NotNil(t, respWriter)
}

func TestNewRespWriterHandler(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	requestTimeout := 100 * time.Millisecond

	tests := []struct {
		name                    string
		logger                  *slog.Logger
		timeout                 time.Duration
		handler                 dns.HandlerFunc
		wantErrContains         string
		wantErrIs               error
		wantErrExchangeContains string
	}{
		{
			name:    "success",
			logger:  testLogger,
			timeout: requestTimeout,
			handler: func(w dns.ResponseWriter, req *dns.Msg) {
			},
		},
		{
			name:    "success-req-timeout",
			logger:  testLogger,
			timeout: requestTimeout,
			handler: func(w dns.ResponseWriter, req *dns.Msg) {
				time.Sleep(200 * time.Millisecond)
			},
			wantErrExchangeContains: "i/o timeout",
		},
		{
			name:    "err-nil-logger",
			logger:  nil,
			timeout: requestTimeout,
			handler: func(w dns.ResponseWriter, req *dns.Msg) {},
		},
		{
			name:            "err-invalid-request-timeout",
			logger:          testLogger,
			timeout:         -1 * time.Second,
			handler:         func(w dns.ResponseWriter, req *dns.Msg) {},
			wantErrIs:       ErrInvalidParameter,
			wantErrContains: "invalid request timeout",
		},
		{
			name:            "err-nil-handler",
			logger:          testLogger,
			timeout:         requestTimeout,
			wantErrIs:       ErrInvalidParameter,
			wantErrContains: "nil handler",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert, require := assert.New(t), require.New(t)
			var executedHandler bool

			var got dns.HandlerFunc
			var err error
			switch {
			case tc.handler == nil:
				got, err = NewHandlerFunc(tc.timeout, tc.handler, WithLogger(tc.logger))
			default:
				testMockHandler := func(w dns.ResponseWriter, req *dns.Msg) {
					t.Helper()
					executedHandler = true
					tc.handler(w, req)
					m := new(dns.Msg)
					m.SetReply(req)

					m.Extra = make([]dns.RR, 1)
					m.Extra[0] = &dns.TXT{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0}, Txt: []string{"Hello world"}}
					_ = w.WriteMsg(m)
				}
				got, err = NewHandlerFunc(tc.timeout, testMockHandler, WithLogger(tc.logger))
			}
			if tc.wantErrContains != "" {
				require.Error(err)
				assert.Contains(err.Error(), tc.wantErrContains)
				if tc.wantErrIs != nil {
					assert.ErrorIs(err, tc.wantErrIs)
				}
				return
			}
			require.NoError(err)
			assert.NotNil(got)

			_, c, addr := runTestDnsServer(t, "go.dev", got)

			m := new(dns.Msg)
			m.SetQuestion("go.dev.", dns.TypeTXT)
			r, _, err := c.Exchange(m, addr)
			if tc.wantErrExchangeContains != "" {
				require.Error(err)
				assert.Contains(err.Error(), tc.wantErrExchangeContains)
				return
			}
			require.NoErrorf(err, "failed to exchange go.dev")

			require.NotZerof(len(r.Extra), "failed to exchange go.dev")
			txt := r.Extra[0].(*dns.TXT).Txt[0]
			assert.Equalf("Hello world", txt, "unexpected result for go.dev %s != Hello word", txt)
			assert.True(executedHandler)
		})
	}
}

func TestRespWriter_WriteMsg(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	t.Run("context-done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		cancel()
		msg := new(dns.Msg)
		err := respWriter.WriteMsg(msg)
		assert.Equal(t, context.Canceled, err)
	})
	t.Run("context-timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		time.Sleep(200 * time.Millisecond)
		msg := new(dns.Msg)
		err := respWriter.WriteMsg(msg)
		assert.Equal(t, context.DeadlineExceeded, err)
	})
	t.Run("success", func(t *testing.T) {
		respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
		msg := new(dns.Msg)
		err := respWriter.WriteMsg(msg)
		assert.NoError(t, err)
	})
}

func TestRespWriter_Write(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	t.Run("context-done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		cancel()
		n, err := respWriter.Write([]byte{})
		assert.Equal(t, 0, n)
		assert.Equal(t, context.Canceled, err)
	})
	t.Run("context-timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		time.Sleep(200 * time.Millisecond)
		n, err := respWriter.Write([]byte{})
		assert.Equal(t, 0, n)
		assert.Equal(t, context.DeadlineExceeded, err)
	})
	t.Run("success", func(t *testing.T) {
		respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
		n, err := respWriter.Write([]byte{})
		assert.Equal(t, 0, n)
		assert.NoError(t, err)
	})
}

func TestRespWriter_RemoteAddr(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	t.Run("context-done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		cancel()
		addr := respWriter.RemoteAddr()
		assert.NotNil(t, addr)
	})
	t.Run("context-timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		time.Sleep(200 * time.Millisecond)
		addr := respWriter.RemoteAddr()
		assert.NotNil(t, addr)
	})
	t.Run("success", func(t *testing.T) {
		respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
		addr := respWriter.RemoteAddr()
		assert.NotNil(t, addr)
	})
}

func TestRespWriter_LocalAdrr(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	t.Run("context-done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		cancel()
		addr := respWriter.LocalAddr()
		assert.NotNil(t, addr)
	})
	t.Run("context-timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		time.Sleep(200 * time.Millisecond)
		addr := respWriter.LocalAddr()
		assert.NotNil(t, addr)
	})
	t.Run("success", func(t *testing.T) {
		respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
		addr := respWriter.LocalAddr()
		assert.NotNil(t, addr)
	})
}

func TestRespWriter_TigStatus(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	t.Run("context-done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		err := respWriter.TsigStatus()
		assert.Equal(t, context.Canceled, err)
	})
	t.Run("context-timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		time.Sleep(200 * time.Millisecond)
		err := respWriter.TsigStatus()
		assert.Equal(t, context.DeadlineExceeded, err)
	})
	t.Run("success", func(t *testing.T) {
		respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
		err := respWriter.TsigStatus()
		assert.NoError(t, err)
	})
}

func TestRespWriter_TsigTimersOnly(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	t.Run("context-done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		cancel()
		respWriter.TsigTimersOnly(true)
	})
	t.Run("context-timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		time.Sleep(200 * time.Millisecond)
		respWriter.TsigTimersOnly(true)
	})
	t.Run("success", func(t *testing.T) {
		respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
		respWriter.TsigTimersOnly(true)
	})
}

func TestRespWriter_Hijack(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	t.Run("context-done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		cancel()
		respWriter.Hijack()
	})
	t.Run("context-timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		time.Sleep(200 * time.Millisecond)
		respWriter.Hijack()
	})
	t.Run("success", func(t *testing.T) {
		respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
		respWriter.Hijack()
	})
}

func TestRespWriter_Close(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	t.Run("context-done", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		cancel()
		assert.NoError(t, respWriter.Close())
	})
	t.Run("context-timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		respWriter := NewRespWriter(ctx, w, WithLogger(testLogger))
		time.Sleep(200 * time.Millisecond)
		assert.NoError(t, respWriter.Close())
	})
	t.Run("success", func(t *testing.T) {
		respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
		assert.NoError(t, respWriter.Close())
	})
}

func TestRespWriter_RequestCtx(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
	assert.NotNil(t, respWriter.RequestContext())
}

func TestRespWriter_Logger(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
	assert.NotNil(t, respWriter.Logger())
	assert.Equal(t, testLogger, respWriter.Logger())
}

func TestRespWriter_SetLogger(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
	newLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	respWriter.SetLogger(newLogger)
	assert.Equal(t, newLogger, respWriter.Logger())
}

func TestRespWriter_Underlying(t *testing.T) {
	t.Parallel()
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	w := new(mockDNSResponseWriter)
	respWriter := NewRespWriter(context.Background(), w, WithLogger(testLogger))
	assert.NotNil(t, respWriter.Underlying())
	assert.Equal(t, w, respWriter.Underlying())
}

type mockDNSResponseWriter struct {
	dns.ResponseWriter
}

func (w *mockDNSResponseWriter) WriteMsg(msg *dns.Msg) error {
	return nil
}

func (w *mockDNSResponseWriter) Write(b []byte) (int, error) {
	return 0, nil
}

func (w *mockDNSResponseWriter) RemoteAddr() net.Addr {
	return &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

func (w *mockDNSResponseWriter) LocalAddr() net.Addr {
	return &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

func (w *mockDNSResponseWriter) TsigStatus() error {
	return nil
}

func (w *mockDNSResponseWriter) TsigTimersOnly(b bool) {
}

func (w *mockDNSResponseWriter) Hijack() {
}

func (w *mockDNSResponseWriter) Close() error {
	return nil
}

func (w *mockDNSResponseWriter) IncomingPacketConn() net.PacketConn {
	return &net.UDPConn{}
}

func (w *mockDNSResponseWriter) IncomingConn() net.Conn {
	return &net.TCPConn{}
}
