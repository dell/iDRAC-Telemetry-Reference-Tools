// Licensed to You under the Apache License, Version 2.0.

package metricreport

import ( 
	
)

//TODO
// Replacable tags - REPORTNAME, REPORTSEQUENCE, TIMESTAMP, SERVICETAG, DIGEST
// Fill in metric values

func GetMetricReport() string {
	mrheader := "\"@odata.type\": \"#MetricReport.v1_4_1.MetricReport\", \"@odata.context\": \"/redfish/v1/$metadata#MetricReport.MetricReport\", \"@odata.id\": \"/redfish/v1/TelemetryService/MetricReports/REPORTNAME\", \"Id\": \"REPORTNAME\", \"Name\": \"REPORTNAME Metric Report\", \"ReportSequence\": \"REPORTSEQUENCE\", \"Timestamp\": \"TIMESTAMP\",  \"MetricReportDefinition\": {\"@odata.id\": \"/redfish/v1/TelemetryService/MetricReportDefinitions/REPORTNAME\"},"

	mrfooter := "\"Oem\": {\"Dell\": {\"@odata.type\": \"#DellMetricReport.v1_0_0.DellMetricReport\",\"ServiceTag\": \"SERVICETAG\",\"MetricReportDefinitionDigest\": \"DIGEST\",\"iDRACFirmwareVersion\": \"FWVERSION\"}}"
	return "{" + mrheader + mrfooter + "}"
}

