package main

import (
	"fmt"
	"net"
	"time"

	"github.com/jimlambrt/respwriter"
	"github.com/miekg/dns"
)

func main() {
	mux := dns.NewServeMux()
	// wrap the handler with a 100ms timeout
	handlerWithTimeout, err := respwriter.NewHandlerFunc(100*time.Millisecond, new(dnsHandler).ServeDNS)
	if err != nil {
		fmt.Printf("Failed to create handler: %s\n", err.Error())
		return
	}
	mux.HandleFunc(".", handlerWithTimeout)
	pc, err := net.ListenPacket("udp", ":0")

	if err != nil {
		fmt.Printf("Failed to start listener: %s\n", err.Error())
		return
	}

	server := &dns.Server{
		PacketConn: pc,
		Net:        "udp",
		Handler:    mux,
		UDPSize:    65535,
		ReusePort:  true,
	}

	fmt.Printf("Starting DNS server on %s\n", pc.LocalAddr())
	err = server.ListenAndServe()
	if err != nil {
		fmt.Printf("Failed to start server: %s\n", err.Error())
		return
	}
}

type dnsHandler struct{}

func (h *dnsHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	_, ok := w.(*respwriter.RespWriter)
	if !ok {
		// this cannot happen given the way we're using
		// respwriter.NewHandlerFunc to wrap ServeDNS
		fmt.Println("Failed to cast to RespWriter")
	}
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true
	for _, question := range r.Question {
		fmt.Printf("Received query: %s\n", question.Name)
		answers := resolve(question.Name, question.Qtype)
		msg.Answer = append(msg.Answer, answers...)
	}
	w.WriteMsg(msg)
}

func resolve(domain string, qtype uint16) []dns.RR {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), qtype)
	m.RecursionDesired = true

	c := new(dns.Client)
	in, _, err := c.Exchange(m, "8.8.8.8:53")
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return in.Answer
}
