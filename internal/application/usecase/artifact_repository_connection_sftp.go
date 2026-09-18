package usecase

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	domain "gos/internal/domain/artifactrepo"
)

type sftpArtifactRepositoryConnectionTester struct {
	now func() time.Time
}

func newSFTPArtifactRepositoryConnectionTester(now func() time.Time) *sftpArtifactRepositoryConnectionTester {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &sftpArtifactRepositoryConnectionTester{now: now}
}

// TestConnection opens an SSH session, authenticates, then lists the configured
// directory. Listing rather than merely connecting proves the credential grants
// access to the path the platform will actually publish into.
func (t *sftpArtifactRepositoryConnectionTester) TestConnection(ctx context.Context, input domain.UpdateInput) (ArtifactRepositoryConnectionTestResult, error) {
	if input.RepositoryType != domain.RepositoryTypeSFTP {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: repository_type is invalid", ErrInvalidInput)
	}

	host, port, err := resolveRemoteArtifactRepositoryAddress(input)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, err
	}
	authMethods, err := sftpAuthMethods(input)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, artifactRepositoryRemoteConnectionTimeout)
	defer cancel()

	conn, address, err := dialRemoteArtifactRepository(ctx, host, port)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeRemoteDialFailure("SFTP", err))
	}

	clientConfig := &ssh.ClientConfig{
		User:            input.Username,
		Auth:            authMethods,
		HostKeyCallback: sftpHostKeyCallback(input.HostKeyFingerprint),
		Timeout:         artifactRepositoryRemoteConnectionTimeout,
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, address, clientConfig)
	if err != nil {
		_ = conn.Close()
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeSFTPHandshakeFailure(err))
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)
	defer func() { _ = sshClient.Close() }()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeSFTPHandshakeFailure(err))
	}
	defer func() { _ = sftpClient.Close() }()

	directory := sftpProbeDirectory(input.Directory)
	if _, err := sftpClient.ReadDirContext(ctx, directory); err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeSFTPDirectoryFailure(err, directory))
	}
	return ArtifactRepositoryConnectionTestResult{Success: true, Message: "制品库连通性检测通过"}, nil
}

func sftpAuthMethods(input domain.UpdateInput) ([]ssh.AuthMethod, error) {
	methods := make([]ssh.AuthMethod, 0, 2)
	if password := strings.TrimSpace(input.Password); password != "" {
		methods = append(methods, ssh.Password(password))
	}
	if rawKey := strings.TrimSpace(input.PrivateKey); rawKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(rawKey))
		if err != nil {
			var passphraseMissing *ssh.PassphraseMissingError
			if errors.As(err, &passphraseMissing) {
				return nil, fmt.Errorf("%w: 私钥受口令保护，请改用未加密私钥或密码认证", ErrInvalidInput)
			}
			return nil, fmt.Errorf("%w: private_key is invalid", ErrInvalidInput)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("%w: password or private_key is required", ErrInvalidInput)
	}
	return methods, nil
}

// errSFTPHostKeyMismatch marks a rejected server key so the failure can be
// recognised without matching on a translated message.
var errSFTPHostKeyMismatch = errors.New("sftp host key fingerprint mismatch")

// sftpHostKeyCallback verifies the server key only when the operator pinned a
// fingerprint. Without one the platform has no known_hosts source, so the key
// cannot be checked at all; the UI states that rather than hiding it here.
func sftpHostKeyCallback(fingerprint string) ssh.HostKeyCallback {
	expected := normalizeSSHFingerprint(fingerprint)
	if expected == "" {
		return ssh.InsecureIgnoreHostKey() //nolint:gosec // the fingerprint is optional by design
	}
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		actual := normalizeSSHFingerprint(ssh.FingerprintSHA256(key))
		if actual != expected {
			return fmt.Errorf("%w: 实际为 %s", errSFTPHostKeyMismatch, ssh.FingerprintSHA256(key))
		}
		return nil
	}
}

// normalizeSSHFingerprint tolerates a missing "SHA256:" prefix, which is easy to
// drop when copying from ssh-keyscan output.
func normalizeSSHFingerprint(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "SHA256:")
	return strings.TrimSpace(trimmed)
}

// sftpProbeDirectory maps the stored directory onto a path the SFTP server
// understands. The normalizer strips surrounding slashes, so an empty value
// means the login directory rather than the filesystem root.
func sftpProbeDirectory(directory string) string {
	trimmed := strings.Trim(strings.TrimSpace(directory), "/")
	if trimmed == "" {
		return "."
	}
	return trimmed
}

func describeSFTPHandshakeFailure(err error) string {
	if errors.Is(err, errSFTPHostKeyMismatch) {
		return "SFTP 主机密钥指纹不匹配，请核对服务器指纹"
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "unable to authenticate"),
		strings.Contains(message, "no supported methods remain"):
		return "SFTP 认证失败，请检查用户名、密码或私钥"
	default:
		// Transport failures such as a timeout during the handshake still read
		// better with the shared wording.
		return describeRemoteDialFailure("SFTP", err)
	}
}

func describeSFTPDirectoryFailure(err error, directory string) string {
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "does not exist"),
		strings.Contains(message, "no such file"):
		return fmt.Sprintf("SFTP 目录 %s 不存在，请检查远端路径", directory)
	case strings.Contains(message, "permission denied"):
		return fmt.Sprintf("SFTP 目录 %s 无访问权限，请检查账号权限", directory)
	default:
		return fmt.Sprintf("SFTP 目录 %s 探测失败：%s", directory, message)
	}
}
