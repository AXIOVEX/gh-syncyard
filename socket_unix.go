//go:build unix

package main

import (
	"bufio"
	"net"
	"time"
)

// socketCallUnix sends one newline-terminated JSON request to
// the daemon's Unix socket and returns the reply line, mirroring
// the tray's SocketGatekeeper channel (serve.rs / control.rs:
// one request line, one reply line per connection).
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
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}
