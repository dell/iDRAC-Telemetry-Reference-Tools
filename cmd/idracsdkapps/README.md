# iDRAC SDK plugin apps

The applications under this folder provide iDRAC telemetry custom container applications. They are bare telemetry to metrics translators for various big database backends like splunk and grafana. These run as container based plugins.

# splunkapp
This listens to the iDRAC SSE endpoint for various metric reports and translates them to splunk format, and sends them to a configured splunk application server.