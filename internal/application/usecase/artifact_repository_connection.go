package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	domain "gos/internal/domain/artifactrepo"
)

const artifactRepositoryConnectionTimeout = 5 * time.Second

type ArtifactRepositoryConnectionTestResult struct {
	Success bool
	Message string
}

type ArtifactRepositoryConnectionTester interface {
	TestConnection(ctx context.Context, input domain.UpdateInput) (ArtifactRepositoryConnectionTestResult, error)
}

type ossArtifactRepositoryConnectionTester struct {
	client *http.Client
	now    func() time.Time
}

func newOSSArtifactRepositoryConnectionTester(client *http.Client, now func() time.Time) *ossArtifactRepositoryConnectionTester {
	if client == nil {
		client = http.DefaultClient
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &ossArtifactRepositoryConnectionTester{
		client: client,
		now:    now,
	}
}

func (t *ossArtifactRepositoryConnectionTester) TestConnection(ctx context.Context, input domain.UpdateInput) (ArtifactRepositoryConnectionTestResult, error) {
	if input.RepositoryType != domain.RepositoryTypeOSS {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: repository_type is invalid", ErrInvalidInput)
	}

	bucketURL, err := buildOSSBucketURL(input.Endpoint, input.Bucket)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, artifactRepositoryConnectionTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, bucketURL.String(), nil)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	date := t.now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", buildOSSAuthorization(input.AccessKeyID, input.AccessKeySecret, http.MethodHead, date, input.Bucket))

	resp, err := t.client.Do(req)
	if err != nil {
		return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %v", ErrArtifactConnectionFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ArtifactRepositoryConnectionTestResult{Success: true, Message: "制品库连通性检测通过"}, nil
	}

	return ArtifactRepositoryConnectionTestResult{}, fmt.Errorf("%w: %s", ErrArtifactConnectionFailed, describeOSSConnectionFailure(resp.StatusCode))
}

func buildOSSBucketURL(endpoint string, bucket string) (*url.URL, error) {
	cleanEndpoint := strings.TrimSpace(endpoint)
	if cleanEndpoint == "" {
		return nil, fmt.Errorf("%w: endpoint is required", ErrInvalidInput)
	}
	if !strings.Contains(cleanEndpoint, "://") {
		cleanEndpoint = "https://" + cleanEndpoint
	}

	parsed, err := url.Parse(cleanEndpoint)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("%w: endpoint is invalid", ErrInvalidInput)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: endpoint scheme is invalid", ErrInvalidInput)
	}

	cleanBucket := strings.TrimSpace(bucket)
	if cleanBucket == "" {
		return nil, fmt.Errorf("%w: bucket is required", ErrInvalidInput)
	}

	host := parsed.Host
	if !strings.HasPrefix(strings.ToLower(host), strings.ToLower(cleanBucket)+".") {
		host = cleanBucket + "." + host
	}
	parsed.Host = host
	parsed.Path = "/"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func buildOSSAuthorization(accessKeyID string, accessKeySecret string, method string, date string, bucket string) string {
	canonicalizedResource := "/" + bucket + "/"
	stringToSign := method + "\n\n\n" + date + "\n" + canonicalizedResource
	mac := hmac.New(sha1.New, []byte(accessKeySecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return "OSS " + accessKeyID + ":" + signature
}

func describeOSSConnectionFailure(statusCode int) string {
	switch statusCode {
	case http.StatusForbidden:
		return "OSS 拒绝访问，请检查 AccessKey、Bucket 权限和 Endpoint"
	case http.StatusNotFound:
		return "OSS Bucket 不存在或 Endpoint 不匹配"
	case http.StatusUnauthorized:
		return "OSS 认证失败，请检查访问凭证"
	default:
		return fmt.Sprintf("OSS 返回状态码 %d", statusCode)
	}
}
