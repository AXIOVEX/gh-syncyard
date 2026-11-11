//go:build unix

package main

import (
	"bufio"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real-transport tests for socketCallUnix: a real unix socket
// server in-process, no transport fakes. socketCallUnix had
// 0.0% coverage in the audit because every other socket test
// substitutes the socketCall variable.

func startUnixServer(t *testing.T, respond func(net.Conn)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gk.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Consume the one request line before replying, like
		// the daemon does.
		_, _ = bufio.NewReader(conn).ReadString('\n')
		respond(conn)
	}()
	return path
}

func TestSocketCallUnixRoundTrip(t *testing.T) {
	path := startUnixServer(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte("{\"ok\":true,\"outcomes\":[]}\n"))
	})
	reply, err := socketCallUnix(path, []byte(`{"verb":"control_outcomes"}`))
	if err != nil {
		t.Fatalf("socketCallUnix: %v", err)
	}
	if !strings.Contains(reply, `"ok":true`) {
		t.Fatalf("reply = %q", reply)
	}
}

func TestSocketCallUnixEOFPartialLineIsError(t *testing.T) {
	path := startUnixServer(t, func(conn net.Conn) {
		// A reply line with no terminating newline, then EOF:
		// a partial reply, never a success.
		_, _ = conn.Write([]byte(`{"ok":true}`))
	})
	_, err := socketCallUnix(path, []byte(`{"verb":"ping"}`))
	if err == nil {
		t.Fatal("EOF-partial reply line must be an error, got nil")
	}
}

func TestSocketCallUnixOversizedReplyIsError(t *testing.T) {
	path := startUnixServer(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte(strings.Repeat("x", 2<<20) + "\n"))
	})
	_, err := socketCallUnix(path, []byte(`{"verb":"ping"}`))
	if err == nil {
		t.Fatal("oversized reply must be an error, got nil")
	}
}

func TestSocketCallUnixDialFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gk.sock")
	if _, err := socketCallUnix(path, []byte(`{"verb":"ping"}`)); err == nil {
		t.Fatal("dial to a missing socket must be an error")
	}
}

func TestSocketVerbRealTransportMalformedReply(t *testing.T) {
	cleanEnv(t)
	home := t.TempDir()
	t.Setenv("SYNCYARD_STATE_HOME", home)
	sockPath := filepath.Join(home, "gk.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, _ = bufio.NewReader(conn).ReadString('\n')
		_, _ = conn.Write([]byte("this is not json\n"))
	}()
	// Use the real transport (not the captureSocket fake).
	old := socketCall
	socketCall = socketCallUnix
	t.Cleanup(func() { socketCall = old })

	code, _, errOut := run(t, "control-outcomes")
	if code == 0 {
		t.Fatalf("malformed real reply: exit 0 (err=%q), want nonzero", errOut)
	}
	if !strings.Contains(errOut, "invalid daemon reply") {
		t.Fatalf("malformed real reply: err=%q, want 'invalid daemon reply'", errOut)
	}
}
