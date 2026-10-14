// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// S3Config holds credentials and endpoint details for AWS S3 / Cloudflare R2 / MinIO.
type S3Config struct {
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string // Custom endpoint, e.g. https://<account>.r2.cloudflarestorage.com or http://localhost:9000
	PathStyle       bool   // Force path-style access (/bucket/key vs bucket.s3.amazonaws.com/key)
	HTTPClient      *http.Client
}

// S3Store provides an S3-compatible Store implementation with zero external dependencies.
type S3Store struct {
	cfg       S3Config
	client    *http.Client
	baseURL   *url.URL
	pathStyle bool
	region    string
	accessKey string
	secretKey string
}

// NewS3Store creates a new S3-compatible Store.
func NewS3Store(cfg S3Config) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("objstore/s3: bucket name is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	var rawEndpoint string
	var pathStyle bool

	if cfg.Endpoint != "" {
		rawEndpoint = strings.TrimRight(cfg.Endpoint, "/")
		// If custom endpoint provided, default to path style (standard for MinIO / local / R2)
		pathStyle = true
	} else {
		// Standard AWS S3 endpoint
		if cfg.Region == "us-east-1" {
			rawEndpoint = "https://s3.amazonaws.com"
		} else {
			rawEndpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.Region)
		}
		pathStyle = cfg.PathStyle
	}
	if cfg.PathStyle {
		pathStyle = true
	}

	u, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, fmt.Errorf("objstore/s3: invalid endpoint %q: %w", rawEndpoint, err)
	}

	return &S3Store{
		cfg:       cfg,
		client:    client,
		baseURL:   u,
		pathStyle: pathStyle,
		region:    cfg.Region,
		accessKey: cfg.AccessKeyID,
		secretKey: cfg.SecretAccessKey,
	}, nil
}

func (s *S3Store) endpointURL(key string) string {
	cleanKey := strings.TrimPrefix(key, "/")
	if s.pathStyle {
		if cleanKey == "" {
			return fmt.Sprintf("%s/%s", s.baseURL.String(), s.cfg.Bucket)
		}
		return fmt.Sprintf("%s/%s/%s", s.baseURL.String(), s.cfg.Bucket, cleanKey)
	}
	if cleanKey == "" {
		return fmt.Sprintf("%s://%s.%s", s.baseURL.Scheme, s.cfg.Bucket, s.baseURL.Host)
	}
	return fmt.Sprintf("%s://%s.%s/%s", s.baseURL.Scheme, s.cfg.Bucket, s.baseURL.Host, cleanKey)
}

// Sign signs an HTTP request using AWS Signature Version 4.
func (s *S3Store) Sign(req *http.Request, payloadHash string, t time.Time) {
	if s.accessKey == "" || s.secretKey == "" {
		return // Unauthenticated (public buckets)
	}

	dateStamp := t.UTC().Format("20060102")
	amzDate := t.UTC().Format("20060102T150405Z")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	// 1. Canonical Headers & Signed Headers
	headers := map[string]string{
		"host":                 req.URL.Host,
		"x-amz-date":           amzDate,
		"x-amz-content-sha256": payloadHash,
	}
	if r := req.Header.Get("Range"); r != "" {
		headers["range"] = r
	}

	var headerNames []string
	for k := range headers {
		headerNames = append(headerNames, k)
	}
	sort.Strings(headerNames)

	var canonicalHeaders strings.Builder
	for _, k := range headerNames {
		fmt.Fprintf(&canonicalHeaders, "%s:%s\n", k, strings.TrimSpace(headers[k]))
	}
	signedHeaders := strings.Join(headerNames, ";")

	// 2. Canonical URI & Query
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalQuery := req.URL.RawQuery
	if canonicalQuery != "" {
		// Sort query parameters
		vals := req.URL.Query()
		canonicalQuery = vals.Encode()
	}

	// 3. Canonical Request
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	)

	hashedCanonicalRequest := sha256Hex([]byte(canonicalRequest))

	// 4. String to Sign
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credentialScope,
		hashedCanonicalRequest,
	)

	// 5. Signature
	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	// 6. Authorization Header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authHeader)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// Get opens an object from S3 for reading.
func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	u := s.endpointURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	s.Sign(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", time.Now())
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3 get: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("s3 get status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// GetRange reads a specific byte range of an object from S3.
func (s *S3Store) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	u := s.endpointURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	s.Sign(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", time.Now())

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3 get range: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("s3 get range status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// Put uploads an object to S3.
func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	u := s.endpointURL(key)

	// Read body to compute SHA-256 (or use buffer for SigV4 compliance)
	bodyReader := r
	payloadHash := "UNSIGNED-PAYLOAD"

	if size >= 0 && size <= 64*1024*1024 {
		data, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("read put payload: %w", err)
		}
		payloadHash = sha256Hex(data)
		bodyReader = bytes.NewReader(data)
		size = int64(len(data))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bodyReader)
	if err != nil {
		return err
	}

	if size >= 0 {
		req.ContentLength = size
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	s.Sign(req, payloadHash, time.Now())
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("s3 put: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3 put failed (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// Head fetches object metadata without body.
func (s *S3Store) Head(ctx context.Context, key string) (ObjectInfo, error) {
	u := s.endpointURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return ObjectInfo{}, err
	}

	s.Sign(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", time.Now())
	resp, err := s.client.Do(req)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("s3 head: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ObjectInfo{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return ObjectInfo{}, fmt.Errorf("s3 head status %d", resp.StatusCode)
	}

	var modTime time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		modTime, _ = time.Parse(http.TimeFormat, lm)
	}

	return ObjectInfo{
		Key:          key,
		Size:         resp.ContentLength,
		ETag:         strings.Trim(resp.Header.Get("ETag"), `"`),
		LastModified: modTime,
	}, nil
}

// Delete removes an object from S3.
func (s *S3Store) Delete(ctx context.Context, key string) error {
	u := s.endpointURL(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}

	s.Sign(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", time.Now())
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("s3 delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("s3 delete status %d", resp.StatusCode)
	}
	return nil
}

type listBucketResultV2 struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Contents []struct {
		Key          string    `xml:"Key"`
		Size         int64     `xml:"Size"`
		ETag         string    `xml:"ETag"`
		LastModified time.Time `xml:"LastModified"`
	} `xml:"Contents"`
}

// List enumerates keys with prefix.
func (s *S3Store) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	u := s.endpointURL("")
	q := url.Values{}
	q.Set("list-type", "2")
	if prefix != "" {
		q.Set("prefix", prefix)
	}

	reqURL := u + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	s.Sign(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", time.Now())
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3 list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("s3 list status %d", resp.StatusCode)
	}

	var res listBucketResultV2
	if err := xml.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode s3 list xml: %w", err)
	}

	objects := make([]ObjectInfo, 0, len(res.Contents))
	for _, c := range res.Contents {
		objects = append(objects, ObjectInfo{
			Key:          c.Key,
			Size:         c.Size,
			ETag:         strings.Trim(c.ETag, `"`),
			LastModified: c.LastModified,
		})
	}
	return objects, nil
}

// Exists checks whether key exists via Head.
func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Head(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

// URL returns the S3 URI string.
func (s *S3Store) URL(key string) string {
	cleanKey := strings.TrimPrefix(key, "/")
	return fmt.Sprintf("s3://%s/%s", s.cfg.Bucket, cleanKey)
}
