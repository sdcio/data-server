// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scrapligo

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"regexp"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

const ncDelimiter = "]]>]]>"

var messageIDRe = regexp.MustCompile(`message-id="([^"]+)"`)

// fakeNetconfServer is an in-process NETCONF 1.0 server over SSH that answers
// every RPC with an empty <data/> reply. It can drop all connections (kill)
// or stop answering without closing them (blackhole).
type fakeNetconfServer struct {
	t        *testing.T
	listener net.Listener
	cfg      *ssh.ServerConfig

	mu        sync.Mutex
	conns     []net.Conn
	blackhole bool
}

func newFakeNetconfServer(t *testing.T) *fakeNetconfServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("building host key signer: %v", err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	s := &fakeNetconfServer{t: t, listener: l, cfg: cfg}
	go s.acceptLoop()
	t.Cleanup(s.kill)
	return s
}

func (s *fakeNetconfServer) addr() *net.TCPAddr { return s.listener.Addr().(*net.TCPAddr) }

// kill closes the listener and every open connection.
func (s *fakeNetconfServer) kill() {
	_ = s.listener.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.conns = nil
}

// setBlackhole makes the server read but never answer RPCs.
func (s *fakeNetconfServer) setBlackhole(v bool) {
	s.mu.Lock()
	s.blackhole = v
	s.mu.Unlock()
}

func (s *fakeNetconfServer) isBlackhole() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blackhole
}

func (s *fakeNetconfServer) acceptLoop() {
	for {
		c, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, c)
		s.mu.Unlock()
		go s.serve(c)
	}
}

func (s *fakeNetconfServer) serve(c net.Conn) {
	_, chans, reqs, err := ssh.NewServerConn(c, s.cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range chReqs {
				_ = r.Reply(r.Type == "subsystem" || r.Type == "pty-req", nil)
			}
		}()
		go s.serveNetconf(ch)
	}
}

func (s *fakeNetconfServer) serveNetconf(ch ssh.Channel) {
	hello := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<hello xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><capabilities>` +
		`<capability>urn:ietf:params:netconf:base:1.0</capability>` +
		`</capabilities><session-id>1</session-id></hello>` + ncDelimiter
	if _, err := ch.Write([]byte(hello)); err != nil {
		return
	}

	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := ch.Read(tmp)
		if err != nil {
			return
		}
		buf = append(buf, tmp[:n]...)
		for {
			i := bytes.Index(buf, []byte(ncDelimiter))
			if i < 0 {
				break
			}
			msg := buf[:i]
			buf = buf[i+len(ncDelimiter):]

			m := messageIDRe.FindSubmatch(msg)
			if m == nil || s.isBlackhole() {
				continue
			}
			reply := fmt.Sprintf(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="%s"><data/></rpc-reply>%s`, m[1], ncDelimiter)
			if _, err := ch.Write([]byte(reply)); err != nil {
				return
			}
		}
	}
}
