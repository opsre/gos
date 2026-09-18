package usecase

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	domain "gos/internal/domain/artifactrepo"
)

// artifactRepositoryRemoteConnectionTimeout bounds a remote file transfer probe.
// It is deliberately larger than the object storage timeout because an SSH
// handshake costs more round trips than an HTTP request, and deliberately under
// the frontend's 10s axios timeout so the browser still receives the server's
// message instead of aborting on its own.
const artifactRepositoryRemoteConnectionTimeout = 8 * time.Second

// resolveRemoteArtifactRepositoryAddress resolves the host and port to dial.
// Precedence is the explicit port field, then a port embedded in the endpoint,
// then the protocol default.
func resolveRemoteArtifactRepositoryAddress(input domain.UpdateInput) (string, int, error) {
	raw := strings.TrimSpace(input.Endpoint)
	if raw == "" {
		return "", 0, fmt.Errorf("%w: endpoint is required", ErrInvalidInput)
	}
	// Accept a bare host, host:port, or a scheme-prefixed form so operators can
	// paste the address however it happens to be written down.
	if index := strings.Index(raw, "://"); index >= 0 {
		raw = raw[index+3:]
	}
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "/")
	// Drop any path suffix; only the authority part is meaningful here.
	if index := strings.Index(raw, "/"); index >= 0 {
		raw = raw[:index]
	}
	if raw == "" {
		return "", 0, fmt.Errorf("%w: endpoint is invalid", ErrInvalidInput)
	}

	host := raw
	embeddedPort := 0
	if strings.Contains(raw, ":") {
		parsedHost, parsedPort, err := net.SplitHostPort(raw)
		switch {
		case err == nil:
			value, convErr := strconv.Atoi(strings.TrimSpace(parsedPort))
			if convErr != nil || value <= 0 || value > 65535 {
				return "", 0, fmt.Errorf("%w: endpoint port is invalid", ErrInvalidInput)
			}
			host = parsedHost
			embeddedPort = value
		case net.ParseIP(raw) != nil:
			// A bare IPv6 literal contains colons but carries no port.
			host = raw
		default:
			return "", 0, fmt.Errorf("%w: endpoint is invalid", ErrInvalidInput)
		}
	}
	if strings.TrimSpace(host) == "" {
		return "", 0, fmt.Errorf("%w: endpoint is invalid", ErrInvalidInput)
	}

	port := input.Port
	if port <= 0 {
		port = embeddedPort
	}
	if port <= 0 {
		port = input.RepositoryType.DefaultPort()
	}
	if port <= 0 {
		return "", 0, fmt.Errorf("%w: port is required", ErrInvalidInput)
	}
	return host, port, nil
}

// dialRemoteArtifactRepository opens a TCP connection that honours ctx. SSH
// needs this because ssh.NewClientConn takes a net.Conn and ssh.Dial offers no
// way to pass a context.
//
// The error is returned raw so each protocol can phrase the failure with its own
// describers instead of leaking a Go dial error to the operator.
func dialRemoteArtifactRepository(ctx context.Context, host string, port int) (net.Conn, string, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: artifactRepositoryRemoteConnectionTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, address, err
	}
	return conn, address, nil
}

// describeRemoteDialFailure explains a transport-level failure. Connection
// refused, timeouts and DNS failures all happen before any protocol handshake,
// so both file transfer types share this wording and only the protocol name
// differs.
func describeRemoteDialFailure(protocol string, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return protocol + " 连接超时，请检查主机、端口和网络可达性"
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "i/o timeout"),
		strings.Contains(message, "connection timed out"):
		return protocol + " 连接超时，请检查主机、端口和网络可达性"
	case strings.Contains(message, "connection refused"):
		return protocol + " 拒绝连接，请检查端口是否正确"
	case strings.Contains(message, "no such host"),
		strings.Contains(message, "lookup"):
		return protocol + " 主机名无法解析"
	default:
		return protocol + " 连接失败：" + message
	}
}
