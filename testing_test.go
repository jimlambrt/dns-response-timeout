package respwriter

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func runTestDnsServer(t *testing.T, pattern string, h dns.HandlerFunc) (*dns.Server, *dns.Client, string) {
	mux := dns.NewServeMux()
	mux.HandleFunc(pattern, h)
	l, err := net.Listen("tcp", ":0")
	require.NoErrorf(t, err, "failed to listen on TCP %s: %w", ":0", err)
	s, addr, _ := runServer(t, nil, l, func(srv *dns.Server) { srv.Handler = mux })
	t.Cleanup(func() { s.Shutdown() })
	c := &dns.Client{Net: "tcp"}
	return s, c, addr
}

func runServer(t testing.TB, pc net.PacketConn, l net.Listener, opts ...func(*dns.Server)) (*dns.Server, string, chan error) {
	t.Helper()
	if pc == nil && l == nil {
		t.Fatal("either pc or l must be non-nil")
	}

	server := &dns.Server{
		PacketConn: pc,
		Listener:   l,

		ReadTimeout:  time.Hour,
		WriteTimeout: time.Hour,
	}

	waitLock := sync.Mutex{}
	waitLock.Lock()
	server.NotifyStartedFunc = waitLock.Unlock

	for _, opt := range opts {
		opt(server)
	}

	var (
		addr   string
		closer io.Closer
	)
	if l != nil {
		addr = l.Addr().String()
		closer = l
	} else {
		addr = pc.LocalAddr().String()
		closer = pc
	}

	// fin must be buffered so the goroutine below won't block
	// forever if fin is never read from. This always happens
	// if the channel is discarded.
	fin := make(chan error, 1)

	go func() {
		fin <- server.ActivateAndServe()
		closer.Close()
	}()

	t.Cleanup(func() {
		server.Shutdown()
	})
	return server, addr, fin
}
