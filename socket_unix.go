//go:build unix

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"time"
)

// maxDaemonReply bounds one daemon reply line. Control
// outcomes are small JSON objects; an unbounded read would
// let a wedged or hostile socket fill memory until the
// deadline.
const maxDaemonReply = 1 << 20

// socketCallUnix sends one newline-terminated JSON request to
// the daemon's Unix socket and returns the reply line, mirroring
// the tray's SocketGatekeeper channel (serve.rs / control.rs:
// one request line, one reply line per connection). A reply
// that ends at EOF without its terminating newline is a
// partial reply and is an error, never a success.
func socketCallUnix(path string, payload []byte) (string, error) {
	conn, err := net.DialTimeout("unix", path, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := conn.Write(append(append([]byte{}, payload...), '\n')); err != nil {
		return "", err
	}
	reader := bufio.NewReader(io.LimitReader(conn, maxDaemonReply+1))
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading daemon reply: %w", err)
	}
	if len(line) > maxDaemonReply {
		return "", fmt.Errorf("daemon reply exceeds %d bytes", maxDaemonReply)
	}
	return line, nil
}
