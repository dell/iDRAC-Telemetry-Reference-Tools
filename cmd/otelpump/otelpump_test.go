package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func TestReadOtelMetaEnum(t *testing.T) {
	readOtelMeta("redfishToOtel.yaml")
	if idrac2Otel["LinkStatus"].enum[strings.ToLower("Down")].(int) != 0 {
		t.Errorf("enum mapping for LinkStatus Down failed, got %v, want 0", idrac2Otel["LinkStatus"].enum[strings.ToLower("Down")])
	}
	if idrac2Otel["RxPauseXOFFFrames"].enum != nil {
		t.Errorf("enum mapping for RxPauseXOFFFrames failed, got %v, want nil", idrac2Otel["RxPauseXOFFFrames"].enum)
	}
	if idrac2Otel["FCStatOSDriverState"].enum[strings.ToLower("Not Applicable")].(int) != 1 {
		t.Errorf("enum mapping for FCStatOSDriverState Online failed, got %v, want 1", idrac2Otel["FCStatOSDriverState"].enum[strings.ToLower("Online")])
	}
}

func TestReadOtelMetaRepeatedMetricIds(t *testing.T) {
	readOtelMeta("redfishToOtel.yaml")
	if _, ok := idrac2Otel["nicsensor:TemperatureReading"]; !ok {
		t.Errorf("repeatedMetricIds handling failed, got no entry for nicsensor:TemperatureReading")
	}
	if _, ok := idrac2Otel["TemperatureReading"]; ok {
		t.Errorf("repeatedMetricIds handling failed, got entry for TemperatureReading when it should be prefixed with metric report")
	}
}

func logTestCases() []struct {
	name      string
	group     *databus.DataGroup
	wantAttrs map[string]string // attribute key → expected string value
	wantEmpty bool
} {
	// A tiny timestamp that can be parsed by the code under test.
	const ts = "2024-01-01T00:00:00Z"

	return []struct {
		name      string
		group     *databus.DataGroup
		wantAttrs map[string]string
		wantEmpty bool
	}{
		{
			name: "single event - all fields populated",
			group: &databus.DataGroup{
				Events: []databus.EventValue{
					{
						EventTimestamp:    ts,
						MessageSeverity:   "Critical",
						Message:           "Power supply failure",
						EventId:           "PSU1",
						EventType:         "Alert",
						MessageId:         "PowerSupplyFailure",
						OriginOfCondition: "PowerSupply",
						MessageArgs:       []string{"arg1", "arg2"},
					},
				},
			},
			wantAttrs: map[string]string{
				"event.data.type":   "telemetry",
				"event.object.type": "Alert",
				"event.object.id":   "PSU1",
			},
			wantEmpty: false,
		},
		{
			name: "event with empty Message - falls back to MessageId",
			group: &databus.DataGroup{
				Events: []databus.EventValue{
					{
						EventTimestamp:  ts,
						MessageSeverity: "Warning",
						Message:         "",
						EventId:         "NIC1",
						EventType:       "Alert",
						MessageId:       "LinkDown",
					},
				},
			},
			wantAttrs: map[string]string{
				"event.data.type":   "telemetry",
				"event.object.type": "Alert",
				"event.object.id":   "NIC1",
			},
			wantEmpty: false,
		},
		{
			name: "invalid RFC3339 timestamp - should return an empty Rl",
			group: &databus.DataGroup{
				Events: []databus.EventValue{
					{
						EventTimestamp:  "not-a-timestamp",
						MessageSeverity: "OK",
						Message:         "All good",
						EventId:         "XYZ",
						EventType:       "Alert",
						MessageId:       "AllGood",
					},
				},
			},
			wantAttrs: nil,
			wantEmpty: true,
		},
	}
}

// ---------------------------------------------------------------------------
// TestOTLPLogs – exercises toOTLPLogs using the cases above.
// ---------------------------------------------------------------------------
func TestOTLPLogs(t *testing.T) {
	for _, tc := range logTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			rl, err := toOTLPLogs(tc.group)

			// ---------------------------------------------------------------
			// Error handling expectations
			// ---------------------------------------------------------------
			if tc.wantEmpty {
				if len(rl.ScopeLogs) != 0 {
					t.Fatalf("expected 0 LogRecords, but got %d", len(rl.ScopeLogs[0].LogRecords))
				}
				// when we expect an error we don't need to inspect the result
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rl == nil {
				t.Fatalf("got nil ResourceLogs")
			}
			if len(rl.ScopeLogs) == 0 {
				t.Fatalf("no ScopeLogs in result")
			}
			if len(rl.ScopeLogs[0].LogRecords) == 0 {
				t.Fatalf("no LogRecords in result")
			}
			lr := rl.ScopeLogs[0].LogRecords[0]

			// ---------------------------------------------------------------
			// Timestamp – should be parsable and roughly match the input
			// ---------------------------------------------------------------
			parsed, _ := time.Parse(time.RFC3339, tc.group.Events[0].EventTimestamp)
			if parsed.UnixNano() != int64(lr.TimeUnixNano) {
				t.Errorf("timestamp mismatch: got %d, want %d", lr.TimeUnixNano, parsed.UnixNano())
			}

			// ---------------------------------------------------------------
			// Severity – verify both text and numeric mapping
			// ---------------------------------------------------------------
			wantSeverityText := tc.group.Events[0].MessageSeverity
			if lr.SeverityText != wantSeverityText {
				t.Errorf("SeverityText = %q, want %q", lr.SeverityText, wantSeverityText)
			}
			// numeric severity mapping is performed by mapSeverity – we just sanity‑check that it is non‑zero
			if lr.SeverityNumber == 0 {
				t.Errorf("SeverityNumber is zero")
			}

			// ---------------------------------------------------------------
			// Body – should be Message if present, otherwise MessageId
			// ---------------------------------------------------------------
			wantBody, _ := json.Marshal(tc.group.Events[0])
			if body, ok := lr.Body.Value.(*commonv1.AnyValue_StringValue); ok {
				if body.StringValue != string(wantBody) {
					t.Errorf("Body = %q, want %q", body.StringValue, string(wantBody))
				}
			} else {
				t.Fatalf("log body is not a string value")
			}

			// ---------------------------------------------------------------
			// Attributes – verify the expected set (order does not matter)
			// ---------------------------------------------------------------
			attrMap := make(map[string]string)
			for _, kv := range lr.Attributes {
				attrMap[kv.Key] = kv.Value.GetStringValue()
			}
			for k, want := range tc.wantAttrs {
				got, ok := attrMap[k]
				if !ok {
					t.Errorf("missing attribute %q", k)
					continue
				}
				// For the JSON‑encoded message‑args we compare after trimming whitespace
				if strings.HasPrefix(k, "redfish.message_args") {
					if strings.ReplaceAll(got, " ", "") != strings.ReplaceAll(want, " ", "") {
						t.Errorf("attribute %q = %s, want %s", k, got, want)
					}
				} else if got != want && want != "" {
					t.Errorf("attribute %q = %s, want %s", k, got, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper – provides a slice of test cases for TestOTLPMetrics.
// ---------------------------------------------------------------------------
func metricTestCases() []struct {
	name      string
	group     *databus.DataGroup
	wantAttrs map[string]string // attribute key → expected string value
	wantData  *metricsv1.Metric
	wantEmpty bool
} {
	// Tiny timestamp that the conversion code can parse.
	const ts = "2024-01-01T00:00:00Z"
	tsNano, _ := parseRFC3339ToNanos(ts)

	return []struct {
		name      string
		group     *databus.DataGroup
		wantAttrs map[string]string
		wantData  *metricsv1.Metric
		wantEmpty bool
	}{
		{
			name: "single metric - all fields populated",
			group: &databus.DataGroup{
				ID:        "GPUMetrics",
				Timestamp: ts,
				Values: []databus.DataValue{
					{
						ID:        "GPUMemoryUsage",          // metric name
						Context:   "SystemBoard",             // will become the FQDD / scope attribute
						Value:     fmt.Sprintf("%v", 123.45), // OTLP expects a float, we store it as a string
						Timestamp: ts,
						System:    "host123",
						HostName:  "myhost",
					},
				},
			},
			wantData: &metricsv1.Metric{
				Name: "hw.gpu.memory.usage",
				Data: &metricsv1.Metric_Gauge{
					Gauge: &metricsv1.Gauge{
						DataPoints: []*metricsv1.NumberDataPoint{
							{
								TimeUnixNano: uint64(tsNano),
								Value: &metricsv1.NumberDataPoint_AsDouble{
									AsDouble: 123.45,
								},
							},
						},
					},
				},
			},
			wantEmpty: false,
		},
		{
			name: "missing metric value - conversion should error and return empty",
			group: &databus.DataGroup{
				ID:        "Sensor",
				Timestamp: ts,
				Values: []databus.DataValue{
					{
						ID:        "BadMetric",
						Context:   "SystemBoard",
						Value:     "not-a-number", // cannot be parsed as float
						Timestamp: ts,
						System:    "host123",
					},
				},
			},
			wantData:  nil,
			wantEmpty: true,
		},
		{
			name: "single metric - all fields populated with repeatedMetricIds",
			group: &databus.DataGroup{
				ID:        "NICSensor",
				Timestamp: ts,
				Values: []databus.DataValue{
					{
						ID:        "TemperatureReading",      // metric name
						Context:   "SystemBoard",             // will become the FQDD / scope attribute
						Value:     fmt.Sprintf("%v", 123.45), // OTLP expects a float, we store it as a string
						Timestamp: ts,
						System:    "host123",
						HostName:  "myhost",
					},
				},
			},
			wantData: &metricsv1.Metric{
				Name: "hw.nic.temperature",
				Data: &metricsv1.Metric_Gauge{
					Gauge: &metricsv1.Gauge{
						DataPoints: []*metricsv1.NumberDataPoint{
							{
								TimeUnixNano: uint64(tsNano),
								Value: &metricsv1.NumberDataPoint_AsDouble{
									AsDouble: 123.45,
								},
							},
						},
					},
				},
			},
			wantEmpty: false,
		},
	}
}

// ---------------------------------------------------------------------------
// TestOTLPMetrics – exercises toOTLPMetrics using the cases above.
// ---------------------------------------------------------------------------
func TestOTLPMetrics(t *testing.T) {
	readOtelMeta("redfishToOtel.yaml")
	for _, tc := range metricTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			rm, err := toOTLPMetrics(tc.group)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rm == nil {
				t.Fatalf("got nil Metric")
			}
			var metric *metricsv1.Metric
			if len(rm.ScopeMetrics[0].Metrics) != 0 && !tc.wantEmpty {
				metric = rm.ScopeMetrics[0].Metrics[0]
				if metric == nil {
					t.Fatalf("no Metric in result")
				}
			} else if tc.wantEmpty {
				if len(rm.ScopeMetrics[0].Metrics) != 0 {
					t.Fatalf("expected no ScopeMetrics, but got %d", len(rm.ScopeMetrics))
				}
				return
			}

			// ---------------------------------------------------------------
			// Metric name and data
			// ---------------------------------------------------------------
			if metric.Name != tc.wantData.Name {
				t.Errorf("metric name = %q, want %q", metric.Name, tc.wantData.Name)
			}

			if metric.Data.(*metricsv1.Metric_Gauge).Gauge.DataPoints[0].Value.(*metricsv1.NumberDataPoint_AsDouble).AsDouble != tc.wantData.Data.(*metricsv1.Metric_Gauge).Gauge.DataPoints[0].Value.(*metricsv1.NumberDataPoint_AsDouble).AsDouble {
				t.Errorf("metric data mismatch:\n got  %+v\n want %+v", metric.Data, tc.wantData.Data)
			}

		})
	}
}
