// Licensed to You under the Apache License, Version 2.0.

package metricreport

type MetricReport struct {
	Id             string
	Name           string
	ReportSequence string
	MetricValues   []MetricValue
}

type MetricValue struct {
	MetricID    string
	Timestamp   string
	MetricValue string
	Oem         OemMetricValue
}

type OemMetricValue struct {
	Dell DellOemMetricValue
}

type DellOemMetricValue struct {
	ContextID string
	Label     string
}
