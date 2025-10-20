package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

const (
	successReponseCode    = 200
	maxIdleConnsCount     = 10
	idleConnTimoutSeconds = 30
)

type httpExporter struct {
	client *http.Client
	base   string // http(s)://host:port (no trailing slash)
}

func newHTTPExporter(base, caCert, clientCert, clientKey string, skipVerify bool) (*httpExporter, error) {

	// default transport
	client := &http.Client{Transport: &http.Transport{
		MaxIdleConns:    maxIdleConnsCount,
		IdleConnTimeout: idleConnTimoutSeconds * time.Second,
		// InsecureSkipVerify: true because this is updated with the user defined value
		// when SSL is enabled on the collector
		// nolint:gosec
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}

	// TLS
	if caCert != "" {
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM([]byte(caCert)); !ok {
			slog.Warn("Unable to apppend cert to pool")
		}

		config := tls.Config{
			RootCAs:            pool,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: skipVerify, // nolint:gosec
		}

		// Client Authentication - optional
		if clientCert != "" && clientKey != "" {
			cert, err := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
			if err != nil {
				slog.Warn("X509KeyPair error", "error", err)
				return nil, err
			}
			config.Certificates = []tls.Certificate{cert}
		}
		client.Transport = &http.Transport{
			MaxIdleConns:    maxIdleConnsCount,
			IdleConnTimeout: idleConnTimoutSeconds * time.Second,
			TLSClientConfig: &config,
		}
	}
	return &httpExporter{
		client: client,
		base:   strings.TrimRight(base, "/"),
	}, nil

}

func (e *httpExporter) exportMetrics(ctx context.Context, rm *metricsv1.ResourceMetrics) error {
	reqMsg := &collectormetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricsv1.ResourceMetrics{rm}}
	return e.doHTTP(ctx, "/v1/metrics", reqMsg, new(collectormetrics.ExportMetricsServiceResponse))
}

func (e *httpExporter) exportLogs(ctx context.Context, rl *logsv1.ResourceLogs) error {
	reqMsg := &collectorlogs.ExportLogsServiceRequest{ResourceLogs: []*logsv1.ResourceLogs{rl}}
	return e.doHTTP(ctx, "/v1/logs", reqMsg, new(collectorlogs.ExportLogsServiceResponse))
}

func (e *httpExporter) doHTTP(ctx context.Context, path string, pb proto.Message, out proto.Message) error {
	var bodyBytes []byte
	var contentType string
	var err error

	bodyBytes, err = proto.Marshal(pb)
	if err != nil {
		return fmt.Errorf("marshal protobuf: %w", err)
	}
	contentType = "application/x-protobuf"

	url := e.base + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("new http request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("http %s %s -> %d: %s", req.Method, url, resp.StatusCode, strings.TrimSpace(string(b)))
	}

	// Some receivers return empty body for success; others return a protobuf/JSON response.
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read http response: %w", err)
	}
	if len(respBytes) == 0 {
		return nil
	}

	if err := proto.Unmarshal(respBytes, out); err != nil {
		return fmt.Errorf("unmarshal response (proto): %w", err)
	}

	// partial success visibility
	switch r := out.(type) {
	case *collectormetrics.ExportMetricsServiceResponse:
		if ps := r.GetPartialSuccess(); ps != nil && (ps.GetRejectedDataPoints() > 0 || ps.GetErrorMessage() != "") {
			log.Printf("metrics partial success (http): rejected=%d err=%q", ps.GetRejectedDataPoints(), ps.GetErrorMessage())
		}
	case *collectorlogs.ExportLogsServiceResponse:
		if ps := r.GetPartialSuccess(); ps != nil && (ps.GetRejectedLogRecords() > 0 || ps.GetErrorMessage() != "") {
			log.Printf("logs partial success (http): rejected=%d err=%q", ps.GetRejectedLogRecords(), ps.GetErrorMessage())
		}
	}
	return nil
}
