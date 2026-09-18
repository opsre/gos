package usecase

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"

	domain "gos/internal/domain/artifactrepo"
)

type ftpArtifactRepositoryConnectionTester struct {
	now func() time.Time
}

func newFTPArtifactRepositoryConnectionTester(now func() time.Time) *ftpArtifactRepositoryConnectionTester {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &ftpArtifactRepositoryConnectionTester{now: now}
}

// TestConnection logs in and then lists the configured directory. Listing opens
// a data connection, which is the part of FTP that actually breaks in practice:
// a server can accept the login and still fail every transfer because of an
// EPSV or passive-address problem. Probing only the control channel would report
// a false success.
//
// Transfers are always passive. The client library offers no active mode, so the
// only compatibility switch is disabling the EPSV extension in favour of PASV.
func (t *ftpArtifactRepositoryConnectionTester) TestConnection(ctx context.Context, input domain.UpdateInput) (ArtifactRepositoryConnectionTestResult, error) {
	if input.RepositoryType != domain.RepositoryTypeFTP {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: repository_type is invalid", ErrInvalidInput)
	}

	host, port, err := resolveRemoteArtifactRepositoryAddress(input)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, err
	}
	password := strings.TrimSpace(input.Password)
	if password == "" {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: password is required", ErrInvalidInput)
	}

	ctx, cancel := context.WithTimeout(ctx, artifactRepositoryRemoteConnectionTimeout)
	defer cancel()

	// The library's own dialer honours the context deadline, so there is no need
	// to hand it a pre-opened connection the way SSH requires.
	address := net.JoinHostPort(host, strconv.Itoa(port))
	options := []ftp.DialOption{
		ftp.DialWithContext(ctx),
		ftp.DialWithTimeout(artifactRepositoryRemoteConnectionTimeout),
		ftp.DialWithDisabledEPSV(input.DisableEPSV),
	}
	client, err := ftp.Dial(address, options...)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeRemoteDialFailure("FTP", err))
	}
	defer func() { _ = client.Quit() }()

	if err := client.Login(input.Username, password); err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeFTPFailure(err, ""))
	}

	directory := ftpProbeDirectory(input.Directory)
	if err := client.ChangeDir(directory); err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeFTPFailure(err, directory))
	}
	if _, err := client.List(directory); err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeFTPFailure(err, directory))
	}
	return ArtifactRepositoryConnectionTestResult{Success: true, Message: "制品库连通性检测通过"}, nil
}

// ftpProbeDirectory maps the stored directory onto a path the server
// understands. The normalizer strips surrounding slashes, so an empty value
// means the login directory.
func ftpProbeDirectory(directory string) string {
	trimmed := strings.Trim(strings.TrimSpace(directory), "/")
	if trimmed == "" {
		return "."
	}
	return trimmed
}

func describeFTPFailure(err error, directory string) string {
	message := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(message, "530"),
		strings.Contains(message, "Login incorrect"):
		return "FTP 认证失败，请检查用户名和密码"
	case strings.Contains(message, "550"):
		if directory != "" {
			return fmt.Sprintf("FTP 目录 %s 不存在或无权限，请检查远端路径", directory)
		}
		return "FTP 服务拒绝访问，请检查账号权限"
	default:
		// Timeouts and refused connections surface here too, so fall back to the
		// shared transport wording rather than echoing a raw Go error.
		return describeRemoteDialFailure("FTP", err)
	}
}
