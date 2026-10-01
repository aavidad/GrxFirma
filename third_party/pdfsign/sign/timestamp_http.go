// Copyright (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: MIT

package sign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/digitorus/timestamp"
)

const maxTimestampResponseBytes = 1024 * 1024
const timestampRequestTimeout = 15 * time.Second

// requestTimestamp bounds network work and authenticates the response's binding
// to this request. Chain trust remains the caller's validation-policy decision.
func requestTimestamp(tsa TSA, content []byte, algorithm crypto.Hash) ([]byte, error) {
	if !algorithm.Available() {
		return nil, errors.New("timestamp digest algorithm unavailable")
	}
	endpoint, err := url.Parse(tsa.URL)
	if err != nil || endpoint == nil || endpoint.Hostname() == "" ||
		(endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, errors.New("invalid timestamp HTTP(S) endpoint")
	}
	ctx := tsa.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timestampRequestTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	nonce, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("timestamp nonce: %w", err)
	}
	requestDER, err := timestamp.CreateRequest(bytes.NewReader(content), &timestamp.RequestOptions{
		Hash: algorithm, Certificates: true, Nonce: nonce,
	})
	if err != nil {
		return nil, fmt.Errorf("create timestamp request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(requestDER))
	if err != nil {
		return nil, errors.New("cannot prepare timestamp request")
	}
	req.Header.Set("Content-Type", "application/timestamp-query")
	req.Header.Set("Content-Transfer-Encoding", "binary")
	if tsa.Username != "" && tsa.Password != "" {
		req.SetBasicAuth(tsa.Username, tsa.Password)
	}
	client := timestampHTTPClient(tsa.HTTPClient)
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("timestamp request: %w", ctx.Err())
		}
		// Do not expose URLs, query credentials or a remote response in errors.
		return nil, errors.New("timestamp HTTP request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("timestamp HTTP status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxTimestampResponseBytes {
		return nil, errors.New("timestamp response exceeds size limit")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTimestampResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("timestamp response: %w", ctx.Err())
		}
		return nil, errors.New("cannot read timestamp response")
	}
	if len(body) > maxTimestampResponseBytes {
		return nil, errors.New("timestamp response exceeds size limit")
	}
	parsed, err := timestamp.ParseResponse(body)
	if err != nil {
		return nil, errors.New("invalid or rejected timestamp response")
	}
	if len(parsed.Certificates) == 0 {
		return nil, errors.New("timestamp response has no certificate to verify its signature")
	}
	hash := algorithm.New()
	_, _ = hash.Write(content)
	if parsed.HashAlgorithm != algorithm || subtle.ConstantTimeCompare(parsed.HashedMessage, hash.Sum(nil)) != 1 {
		return nil, errors.New("timestamp messageImprint does not match request")
	}
	if parsed.Nonce == nil || parsed.Nonce.Cmp(nonce) != 0 {
		return nil, errors.New("timestamp nonce does not match request")
	}
	return body, nil
}

func timestampHTTPClient(configured *http.Client) *http.Client {
	client := &http.Client{}
	if configured != nil {
		*client = *configured
	}
	if client.Timeout <= 0 || client.Timeout > timestampRequestTimeout {
		client.Timeout = timestampRequestTimeout
	}
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || len(via) >= 5 {
			return errors.New("timestamp redirect limit reached")
		}
		origin := via[0].URL
		if req.Method != http.MethodPost || req.URL.Scheme != origin.Scheme ||
			!strings.EqualFold(req.URL.Host, origin.Host) || req.URL.User != nil {
			return errors.New("timestamp redirect must preserve origin and POST")
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
	return client
}
