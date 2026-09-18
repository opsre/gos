package usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	domain "gos/internal/domain/artifactrepo"
)

func TestArtifactRepositoryManagerCreateValidatesAndDefaultsOSSConfig(t *testing.T) {
	repo := newArtifactRepositoryFake()
	manager := NewArtifactRepositoryManager(repo)
	manager.now = func() time.Time { return time.Unix(100, 0).UTC() }

	item, err := manager.Create(context.Background(), ArtifactRepositoryInput{
		Name:            "  oa 制品库  ",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "  https://oss.example.com  ",
		Bucket:          "  oa  ",
		Directory:       " /release/jar/ ",
		AccessKeyID:     " ak ",
		AccessKeySecret: " secret ",
		ACL:             domain.ACLPublicRead,
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	if item.Name != "oa 制品库" {
		t.Fatalf("Name = %q", item.Name)
	}
	if item.RepositoryType != domain.RepositoryTypeOSS {
		t.Fatalf("RepositoryType = %q", item.RepositoryType)
	}
	if item.Directory != "release/jar" {
		t.Fatalf("Directory = %q", item.Directory)
	}
	if item.Status != domain.StatusEnabled {
		t.Fatalf("Status = %q", item.Status)
	}
	if item.ACL != domain.ACLPublicRead {
		t.Fatalf("ACL = %q", item.ACL)
	}
}

func TestArtifactRepositoryManagerRejectsInvalidACL(t *testing.T) {
	manager := NewArtifactRepositoryManager(newArtifactRepositoryFake())

	_, err := manager.Create(context.Background(), ArtifactRepositoryInput{
		Name:            "oa",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss.example.com",
		Bucket:          "oa",
		AccessKeyID:     "ak",
		AccessKeySecret: "secret",
		ACL:             domain.ACL("public-write"),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create err = %v, want ErrInvalidInput", err)
	}
}

func TestArtifactRepositoryManagerUpdateKeepsExistingSecretWhenBlank(t *testing.T) {
	repo := newArtifactRepositoryFake()
	manager := NewArtifactRepositoryManager(repo)
	manager.now = func() time.Time { return time.Unix(200, 0).UTC() }

	created, err := manager.Create(context.Background(), ArtifactRepositoryInput{
		Name:            "oa",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss.example.com",
		Bucket:          "oa",
		AccessKeyID:     "ak",
		AccessKeySecret: "secret-1",
		ACL:             domain.ACLPrivate,
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	updated, err := manager.Update(context.Background(), created.ID, ArtifactRepositoryInput{
		Name:            "oa-prod",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss-prod.example.com",
		Bucket:          "oa-prod",
		Directory:       "",
		AccessKeyID:     "ak-prod",
		AccessKeySecret: "",
		ACL:             domain.ACLPublicRead,
		Status:          domain.StatusDisabled,
	})
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.AccessKeySecret != "secret-1" {
		t.Fatalf("AccessKeySecret = %q, want original secret", updated.AccessKeySecret)
	}
	if updated.Status != domain.StatusDisabled {
		t.Fatalf("Status = %q", updated.Status)
	}
}

func TestArtifactRepositoryManagerTestConnectionNormalizesInputAndCallsTester(t *testing.T) {
	manager := NewArtifactRepositoryManager(newArtifactRepositoryFake())
	tester := &artifactRepositoryConnectionTesterFake{}
	manager.connectionTester[domain.RepositoryTypeOSS] = tester

	result, err := manager.TestConnection(context.Background(), ArtifactRepositoryInput{
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "  https://oss.example.com/  ",
		Bucket:          "  oa  ",
		Directory:       " /release/jar/ ",
		AccessKeyID:     " ak ",
		AccessKeySecret: " secret ",
	})
	if err != nil {
		t.Fatalf("TestConnection err = %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, want true")
	}
	if tester.input.Endpoint != "https://oss.example.com/" {
		t.Fatalf("Endpoint = %q", tester.input.Endpoint)
	}
	if tester.input.Bucket != "oa" {
		t.Fatalf("Bucket = %q", tester.input.Bucket)
	}
	if tester.input.Directory != "release/jar" {
		t.Fatalf("Directory = %q", tester.input.Directory)
	}
	if tester.input.AccessKeyID != "ak" {
		t.Fatalf("AccessKeyID = %q", tester.input.AccessKeyID)
	}
	if tester.input.AccessKeySecret != "secret" {
		t.Fatalf("AccessKeySecret = %q", tester.input.AccessKeySecret)
	}
}

func TestArtifactRepositoryManagerTestConnectionRequiresSecret(t *testing.T) {
	manager := NewArtifactRepositoryManager(newArtifactRepositoryFake())

	_, err := manager.TestConnection(context.Background(), ArtifactRepositoryInput{
		RepositoryType: domain.RepositoryTypeOSS,
		Endpoint:       "https://oss.example.com",
		Bucket:         "oa",
		AccessKeyID:    "ak",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("TestConnection err = %v, want ErrInvalidInput", err)
	}
}

func TestArtifactRepositoryManagerValidatesPerType(t *testing.T) {
	cases := []struct {
		name    string
		input   ArtifactRepositoryInput
		wantErr bool
	}{
		{
			name:    "ftp without username",
			input:   ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeFTP, Endpoint: "10.8.0.14", Password: "pw"},
			wantErr: true,
		},
		{
			name:    "ftp without password",
			input:   ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeFTP, Endpoint: "10.8.0.14", Username: "ftpuser"},
			wantErr: true,
		},
		{
			name:  "ftp valid",
			input: ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeFTP, Endpoint: "10.8.0.14", Username: "ftpuser", Password: "pw"},
		},
		{
			name:    "sftp without username",
			input:   ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "sftp.example.com", Password: "pw"},
			wantErr: true,
		},
		{
			name:    "sftp without any credential",
			input:   ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "sftp.example.com", Username: "deploy"},
			wantErr: true,
		},
		{
			name:  "sftp with password only",
			input: ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "sftp.example.com", Username: "deploy", Password: "pw"},
		},
		{
			name:  "sftp with private key only",
			input: ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "sftp.example.com", Username: "deploy", PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nbody\n-----END OPENSSH PRIVATE KEY-----"},
		},
		{
			name:    "oss without bucket",
			input:   ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeOSS, Endpoint: "https://oss.example.com", AccessKeyID: "ak", AccessKeySecret: "sk"},
			wantErr: true,
		},
		{
			name:    "port out of range",
			input:   ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryTypeFTP, Endpoint: "10.8.0.14", Port: 70000, Username: "ftpuser", Password: "pw"},
			wantErr: true,
		},
		{
			name:    "unknown type",
			input:   ArtifactRepositoryInput{Name: "a", RepositoryType: domain.RepositoryType("webdav"), Endpoint: "h"},
			wantErr: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			manager := NewArtifactRepositoryManager(newArtifactRepositoryFake())
			_, err := manager.Create(context.Background(), testCase.input)
			if testCase.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("Create err = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create err = %v, want success", err)
			}
		})
	}
}

func TestArtifactRepositoryManagerKeepsObjectStorageFieldsOutOfRemoteTypes(t *testing.T) {
	repo := newArtifactRepositoryFake()
	manager := NewArtifactRepositoryManager(repo)
	manager.now = func() time.Time { return time.Unix(300, 0).UTC() }

	item, err := manager.Create(context.Background(), ArtifactRepositoryInput{
		Name:           "oa-remote",
		RepositoryType: domain.RepositoryTypeSFTP,
		Endpoint:       "  sftp.example.com:2222  ",
		Port:           2222,
		Directory:      "/release/",
		Username:       " deploy ",
		PrivateKey:     "-----BEGIN OPENSSH PRIVATE KEY-----\nbody\n-----END OPENSSH PRIVATE KEY-----",
		// These belong to object storage and must be dropped rather than stored.
		Bucket:          "leftover-bucket",
		AccessKeyID:     "leftover-ak",
		AccessKeySecret: "leftover-sk",
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if item.RepositoryType != domain.RepositoryTypeSFTP {
		t.Fatalf("RepositoryType = %q", item.RepositoryType)
	}
	if item.Username != "deploy" || item.Port != 2222 || item.Endpoint != "sftp.example.com:2222" {
		t.Fatalf("connection fields = %+v", item)
	}
	if item.Directory != "release" {
		t.Fatalf("Directory = %q", item.Directory)
	}
	if item.Bucket != "" || item.AccessKeyID != "" || item.AccessKeySecret != "" {
		t.Fatalf("object storage fields must stay empty on a remote repository: %+v", item)
	}
}

func TestArtifactRepositoryManagerSwitchingTypeDropsPreviousCredentials(t *testing.T) {
	repo := newArtifactRepositoryFake()
	manager := NewArtifactRepositoryManager(repo)
	manager.now = func() time.Time { return time.Unix(400, 0).UTC() }

	created, err := manager.Create(context.Background(), ArtifactRepositoryInput{
		Name:            "oa",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss.example.com",
		Bucket:          "oa",
		AccessKeyID:     "ak",
		AccessKeySecret: "oss-secret",
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	// Re-point the same record at an SFTP server. The object storage secret was
	// never re-sent, so it must not be carried across by the blank-means-keep
	// rule that applies within a single type.
	switched, err := manager.Update(context.Background(), created.ID, ArtifactRepositoryInput{
		Name:           "oa",
		RepositoryType: domain.RepositoryTypeSFTP,
		Endpoint:       "sftp.example.com",
		Username:       "deploy",
		Password:       "sftp-secret",
	})
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if switched.AccessKeySecret != "" || switched.Bucket != "" || switched.AccessKeyID != "" {
		t.Fatalf("object storage fields survived the type switch: %+v", switched)
	}
	if switched.Password != "sftp-secret" {
		t.Fatalf("Password = %q", switched.Password)
	}
}

func TestArtifactRepositoryManagerKeepsRemoteSecretWhenBlank(t *testing.T) {
	repo := newArtifactRepositoryFake()
	manager := NewArtifactRepositoryManager(repo)
	manager.now = func() time.Time { return time.Unix(500, 0).UTC() }

	created, err := manager.Create(context.Background(), ArtifactRepositoryInput{
		Name:           "oa-sftp",
		RepositoryType: domain.RepositoryTypeSFTP,
		Endpoint:       "sftp.example.com",
		Username:       "deploy",
		Password:       "sftp-secret",
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	updated, err := manager.Update(context.Background(), created.ID, ArtifactRepositoryInput{
		Name:           "oa-sftp",
		RepositoryType: domain.RepositoryTypeSFTP,
		Endpoint:       "sftp-prod.example.com",
		Username:       "deploy",
		// Password intentionally blank: the stored one must survive.
	})
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.Password != "sftp-secret" {
		t.Fatalf("Password = %q, want the stored secret", updated.Password)
	}
	if updated.Endpoint != "sftp-prod.example.com" {
		t.Fatalf("Endpoint = %q", updated.Endpoint)
	}
}

func TestArtifactRepositoryManagerTestConnectionDispatchesByType(t *testing.T) {
	manager := NewArtifactRepositoryManager(newArtifactRepositoryFake())
	ftpTester := &artifactRepositoryConnectionTesterFake{}
	manager.connectionTester[domain.RepositoryTypeFTP] = ftpTester

	result, err := manager.TestConnection(context.Background(), ArtifactRepositoryInput{
		RepositoryType: domain.RepositoryTypeFTP,
		Endpoint:       "10.8.0.14",
		Username:       "ftpuser",
		Password:       "pw",
		DisableEPSV:    true,
	})
	if err != nil {
		t.Fatalf("TestConnection err = %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, want true")
	}
	if ftpTester.input.RepositoryType != domain.RepositoryTypeFTP {
		t.Fatalf("dispatched to the wrong tester: %+v", ftpTester.input)
	}
	if !ftpTester.input.DisableEPSV {
		t.Fatalf("DisableEPSV = false, want it forwarded")
	}
	// The protocol default port is resolved when dialing, not when normalizing.
	if ftpTester.input.Port != 0 {
		t.Fatalf("Port = %d, want 0", ftpTester.input.Port)
	}
}

func TestDescribeRemoteConnectionFailures(t *testing.T) {
	refused := errors.New("dial tcp 127.0.0.1:1: connect: connection refused")

	// A raw dial error must never reach the operator untranslated.
	if got := describeRemoteDialFailure("SFTP", refused); got != "SFTP 拒绝连接，请检查端口是否正确" {
		t.Fatalf("describeRemoteDialFailure = %q", got)
	}
	if got := describeRemoteDialFailure("FTP", context.DeadlineExceeded); got != "FTP 连接超时，请检查主机、端口和网络可达性" {
		t.Fatalf("describeRemoteDialFailure deadline = %q", got)
	}
	if got := describeRemoteDialFailure("SFTP", errors.New("lookup nope: no such host")); got != "SFTP 主机名无法解析" {
		t.Fatalf("describeRemoteDialFailure dns = %q", got)
	}

	// The SFTP describer keeps its own cases and delegates the transport ones,
	// which is what makes a refused dial readable instead of a Go error string.
	if got := describeSFTPHandshakeFailure(errors.New("ssh: unable to authenticate, attempted methods [none]")); got != "SFTP 认证失败，请检查用户名、密码或私钥" {
		t.Fatalf("sftp auth = %q", got)
	}
	if got := describeSFTPHandshakeFailure(fmt.Errorf("%w: 实际为 SHA256:x", errSFTPHostKeyMismatch)); got != "SFTP 主机密钥指纹不匹配，请核对服务器指纹" {
		t.Fatalf("sftp host key = %q", got)
	}
	if got := describeSFTPHandshakeFailure(refused); strings.Contains(got, "dial tcp") {
		t.Fatalf("sftp describer leaked a raw dial error: %q", got)
	}

	// FTP keeps protocol codes ahead of the shared transport wording.
	if got := describeFTPFailure(errors.New("530 Login incorrect"), ""); got != "FTP 认证失败，请检查用户名和密码" {
		t.Fatalf("ftp auth = %q", got)
	}
	if got := describeFTPFailure(errors.New("550 no such directory"), "releases"); !strings.Contains(got, "releases") {
		t.Fatalf("ftp directory = %q", got)
	}
	if got := describeFTPFailure(refused, ""); strings.Contains(got, "dial tcp") {
		t.Fatalf("ftp describer leaked a raw dial error: %q", got)
	}
}

func TestSFTPAuthMethods(t *testing.T) {
	if _, err := sftpAuthMethods(domain.UpdateInput{Username: "deploy"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput for a missing credential", err)
	}
	if _, err := sftpAuthMethods(domain.UpdateInput{Password: "pw"}); err != nil {
		t.Fatalf("password auth err = %v", err)
	}
	if _, err := sftpAuthMethods(domain.UpdateInput{PrivateKey: "not-a-key"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput for a malformed key", err)
	}

	methods, err := sftpAuthMethods(domain.UpdateInput{PrivateKey: testSFTPPrivateKeyPEM(t)})
	if err != nil {
		t.Fatalf("private key auth err = %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("len(methods) = %d, want 1", len(methods))
	}
}

func TestSFTPHostKeyCallbackVerifiesOnlyWhenPinned(t *testing.T) {
	// No fingerprint configured means the platform has no known_hosts source and
	// cannot verify the key at all; the callback must accept whatever it is given.
	unpinned := sftpHostKeyCallback("")
	key := mustGenerateSSHPublicKey(t)
	if err := unpinned("sftp.example.com", nil, key); err != nil {
		t.Fatalf("unpinned callback err = %v, want nil", err)
	}

	pinned := sftpHostKeyCallback(ssh.FingerprintSHA256(key))
	if err := pinned("sftp.example.com", nil, key); err != nil {
		t.Fatalf("pinned callback err = %v, want nil for a matching key", err)
	}
	otherKey := mustGenerateSSHPublicKey(t)
	if err := pinned("sftp.example.com", nil, otherKey); err == nil {
		t.Fatalf("pinned callback accepted a mismatched key")
	}
	// Operators often paste the fingerprint without the algorithm prefix.
	if err := sftpHostKeyCallback(strings.TrimPrefix(ssh.FingerprintSHA256(key), "SHA256:"))("sftp.example.com", nil, key); err != nil {
		t.Fatalf("callback rejected a prefix-less fingerprint: %v", err)
	}
}

func TestResolveRemoteArtifactRepositoryAddress(t *testing.T) {
	cases := []struct {
		name     string
		input    domain.UpdateInput
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{
			name:     "ftp default port",
			input:    domain.UpdateInput{RepositoryType: domain.RepositoryTypeFTP, Endpoint: "10.8.0.14"},
			wantHost: "10.8.0.14",
			wantPort: 21,
		},
		{
			name:     "sftp default port",
			input:    domain.UpdateInput{RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "sftp.example.com"},
			wantHost: "sftp.example.com",
			wantPort: 22,
		},
		{
			name:     "port embedded in endpoint",
			input:    domain.UpdateInput{RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "sftp.example.com:2222"},
			wantHost: "sftp.example.com",
			wantPort: 2222,
		},
		{
			name:     "explicit port field wins",
			input:    domain.UpdateInput{RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "sftp.example.com:2222", Port: 2200},
			wantHost: "sftp.example.com",
			wantPort: 2200,
		},
		{
			name:     "scheme prefix and path are stripped",
			input:    domain.UpdateInput{RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "sftp://sftp.example.com/releases/"},
			wantHost: "sftp.example.com",
			wantPort: 22,
		},
		{
			name:     "bare ipv6 literal",
			input:    domain.UpdateInput{RepositoryType: domain.RepositoryTypeSFTP, Endpoint: "::1"},
			wantHost: "::1",
			wantPort: 22,
		},
		{
			name:    "missing endpoint",
			input:   domain.UpdateInput{RepositoryType: domain.RepositoryTypeFTP},
			wantErr: true,
		},
		{
			name:    "non numeric port",
			input:   domain.UpdateInput{RepositoryType: domain.RepositoryTypeFTP, Endpoint: "host:abc"},
			wantErr: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			host, port, err := resolveRemoteArtifactRepositoryAddress(testCase.input)
			if testCase.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("err = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if host != testCase.wantHost || port != testCase.wantPort {
				t.Fatalf("got %s:%d, want %s:%d", host, port, testCase.wantHost, testCase.wantPort)
			}
		})
	}
}

// testSFTPPrivateKeyPEM generates a throwaway RSA key so the private key path is
// exercised against a genuinely parseable PEM rather than a placeholder string.
func testSFTPPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block))
}

func mustGenerateSSHPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	publicKey, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("build ssh public key: %v", err)
	}
	return publicKey
}

type artifactRepositoryFake struct {
	items map[string]domain.ArtifactRepository
}

func newArtifactRepositoryFake() *artifactRepositoryFake {
	return &artifactRepositoryFake{items: map[string]domain.ArtifactRepository{}}
}

func (r *artifactRepositoryFake) InitSchema(context.Context) error { return nil }

func (r *artifactRepositoryFake) Create(_ context.Context, item domain.ArtifactRepository) error {
	r.items[item.ID] = item
	return nil
}

func (r *artifactRepositoryFake) GetByID(_ context.Context, id string) (domain.ArtifactRepository, error) {
	item, ok := r.items[id]
	if !ok {
		return domain.ArtifactRepository{}, domain.ErrNotFound
	}
	return item, nil
}

func (r *artifactRepositoryFake) List(_ context.Context, filter domain.ListFilter) ([]domain.ArtifactRepository, int64, error) {
	items := make([]domain.ArtifactRepository, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}

func (r *artifactRepositoryFake) Update(_ context.Context, id string, input domain.UpdateInput, updatedAt time.Time) (domain.ArtifactRepository, error) {
	item, ok := r.items[id]
	if !ok {
		return domain.ArtifactRepository{}, domain.ErrNotFound
	}
	item.Name = input.Name
	item.RepositoryType = input.RepositoryType
	item.Endpoint = input.Endpoint
	item.Port = input.Port
	item.Bucket = input.Bucket
	item.Directory = input.Directory
	item.AccessKeyID = input.AccessKeyID
	item.AccessKeySecret = input.AccessKeySecret
	item.Username = input.Username
	item.Password = input.Password
	item.PrivateKey = input.PrivateKey
	item.DisableEPSV = input.DisableEPSV
	item.HostKeyFingerprint = input.HostKeyFingerprint
	item.ACL = input.ACL
	item.Status = input.Status
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return item, nil
}

func (r *artifactRepositoryFake) Delete(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

type artifactRepositoryConnectionTesterFake struct {
	input domain.UpdateInput
}

func (t *artifactRepositoryConnectionTesterFake) TestConnection(_ context.Context, input domain.UpdateInput) (ArtifactRepositoryConnectionTestResult, error) {
	t.input = input
	return ArtifactRepositoryConnectionTestResult{Success: true, Message: "连接成功"}, nil
}
