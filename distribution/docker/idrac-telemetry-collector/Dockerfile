FROM    golang:1.15
LABEL   MAINTAINER "iDRAC Telemetry Team <idractelemetryteam@dell.com>"

WORKDIR /go/src/github.com/telemetry-reference-tools

ADD . .
ENTRYPOINT ["/bin/sh","-c", "cmd/idrac-telemetry-collector-all.sh", "-db"]