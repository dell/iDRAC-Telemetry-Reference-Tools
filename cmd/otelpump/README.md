# OpenTelemetry + Loki + Prometheus + Grafana Stack Setup

This guide walks you through setting up a complete observability stack using OpenTelemetry Collector, Loki, Prometheus, and Grafana via Docker Compose.

> **_NOTE:_**  This setup is **only necessary** if you want to create a new OpenTelemetry collector and the corresponding prometheus and grafana containers. If you have an OpenTelemetry collecter and just want to connect iDRAC-reference-tools to it, please read [this section](../../docs/INSTALL.md#opentelemetry) 

---

## 📁 Folder Structure

```bash
project-root/
├── docker-compose.yaml
├── otel-collector-config.yaml
├── collector-export/
├── prometheus.yml
├── prometheus-data/
├── grafana/
│   ├── storage/
│   ├── provisioning/
│   └── dashboards/
└── loki/
    └── loki-config.yaml
```

---

## 🚀 Setup Steps

### 🔐 File & Folder Permissions (Optional but Recommended)

1. **Ensure Read Access for Config Files**  
   Make sure configuration files like `otel-collector-config.yaml`, `prometheus.yml`, and `loki-config.yaml` are readable by Docker:
   ```bash
   chmod 644 <filename>
   ```

2. **Ensure Write Access for Data Directories**  
   Directories like `collector-export/`, `prometheus-data/`, and `grafana/storage/` should be writable by the container processes:
   ```bash
   chmod -R 777 <folder>
   ```

### 1. Clone or Create the Project Directory

```bash
mkdir otel-loki-stack && cd otel-loki-stack
```

### 2. Create Required Files and Folders

```bash
mkdir -p collector-export prometheus-data grafana/storage grafana/provisioning grafana/dashboards loki
```

Add the following files:

- `docker-compose.yaml`[content](#-docker-compose-file-docker-composeyaml)
- `otel-collector-config.yaml`[content](#️-opentelemetry-collector-configuration-otel-collector-configyaml)
- `prometheus.yml`[content](#prometheus-configuration-prometheusyml) 
- `loki/loki-config.yaml`[content](#️-loki-configuration-lokiloki-configyaml)

### 3. Start the Stack

```bash
docker-compose up -d
```

---

## 🧩 Services and Ports

| Service         | Description                    | Port               | URL                        |
|----------------|--------------------------------|--------------------|----------------------------|
| Grafana         | Dashboards & visualization     | 3000               | http://localhost:3000     |
| Prometheus      | Metrics storage & querying     | 9090               | http://localhost:9090     |
| Loki            | Log aggregation backend        | 3100               | http://localhost:3100     |
| OTEL Collector  | Telemetry pipeline             | 4317 (gRPC), 4318 (HTTP) | -                  |
| Health Check    | OTEL Collector health endpoint | 13133              | http://localhost:13133    |

---

## 📦 Docker Compose File (docker-compose.yaml)

```yaml
networks:
  otel-net:
    driver: bridge

services:
  otel-collector:
    image: otel/opentelemetry-collector-contrib:0.136.0
    volumes:
      - ./otel-collector-config.yaml:/etc/otelcol-contrib/config.yaml
      - ./collector-export:/collector-export
    ports:
      - 13133:13133 # health_check extension
      - 4317:4317 # OTLP gRPC receiver
      - 4318:4318 # OTLP http receiver
      - 55679:55679 # zpages extension
      - 9000:9000 # Prometheus metrics
      - 8888:8888 # OpenTelemetry metrics
    networks:
      - otel-net 
  
  prometheus:
    image: prom/prometheus:v2.36.0
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - ./prometheus-data:/prometheus
    command: 
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--enable-feature=remote-write-receiver'
    ports:
      - 9090:9090
    networks:
      - otel-net 

  grafana:
    image: grafana/grafana:9.0.1
    volumes:
      - ./grafana/storage:/var/lib/grafana
      - ./grafana/provisioning:/etc/grafana/provisioning
      - ./grafana/dashboards:/var/lib/grafana/dashboards
    ports:
      - 3000:3000
    networks:
      - otel-net 
  
  loki:
    image: grafana/loki:3.4.1
    volumes:
      - ./loki:/mnt/config
    ports:
      - 3100:3100
    command: -config.file=/mnt/config/loki-config.yaml
    networks:
      - otel-net

```

---

## ⚙️ Loki Configuration (`loki/loki-config.yaml`)

```yaml
auth_enabled: false

server:
  http_listen_port: 3100
  grpc_listen_port: 9096
  log_level: debug
  grpc_server_max_concurrent_streams: 1000

common:
  instance_addr: 127.0.0.1
  path_prefix: /tmp/loki
  storage:
    filesystem:
      chunks_directory: /tmp/loki/chunks
      rules_directory: /tmp/loki/rules
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory

query_range:
  results_cache:
    cache:
      embedded_cache:
        enabled: true
        max_size_mb: 100

limits_config:
  metric_aggregation_enabled: true
  allow_structured_metadata: true

schema_config:
  configs:
    - from: 2020-10-24
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

pattern_ingester:
  enabled: true
  metric_aggregation:
    loki_address: localhost:3100

ruler:
  alertmanager_url: http://localhost:9093

frontend:
  encoding: protobuf

# By default, Loki will send anonymous, but uniquely-identifiable usage and configuration
# analytics to Grafana Labs. These statistics are sent to https://stats.grafana.org/
#
# Statistics help us better understand how Loki is used, and they show us performance
# levels for most users. This helps us prioritize features and documentation.
# For more information on what's sent, look at
# https://github.com/grafana/loki/blob/main/pkg/analytics/stats.go
# Refer to the buildReport method to see what goes into a report.
#
# If you would like to disable reporting, uncomment the following lines:
#analytics:
#  reporting_enabled: false

```

---

## ⚙️ OpenTelemetry Collector Configuration (`otel-collector-config.yaml`)

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:

exporters:
  debug:
    verbosity: detailed

  file:
    path: /collector-export/collector-file-export.jsonl

  prometheusremotewrite:
    endpoint: "http://prometheus:9090/api/v1/write"
  
  otlphttp:
    endpoint: http://loki:3100/otlp

extensions:
  health_check:
    endpoint: "0.0.0.0:13133"

service:
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [debug, file, prometheusremotewrite]
    logs:
      receivers: [otlp]
      exporters: [debug, file, otlphttp]
  extensions: [health_check]
```

## Prometheus Configuration (`prometheus.yml`)

```
# my global config
global:
  scrape_interval: 5s # Set the scrape interval to every 15 seconds. Default is every 1 minute.
  evaluation_interval: 15s # Evaluate rules every 15 seconds. The default is every 1 minute.
  # scrape_timeout is set to the global default (10s).

# Load rules once and periodically evaluate them according to the global 'evaluation_interval'.
rule_files:
# - "first_rules.yml"
# - "second_rules.yml"

# A scrape configuration containing exactly one endpoint to scrape:
# Here it's Prometheus itself.
scrape_configs:
  # The job name is added as a label `job=<job_name>` to any timeseries scraped from this config.
  - job_name: 'prometheus'
    # metrics_path defaults to '/metrics'
    # scheme defaults to 'http'.
    static_configs:
      - targets: [ 'prometheus:9090' ]
```

---

## ✅ Verification

- **Grafana**: Login (default: `admin`/`admin`) and add Loki and Prometheus as data sources.
- **Prometheus**: Visit `/targets` to verify OTEL Collector is scraping metrics.
- **Loki**: Use Grafana Explore to query logs.
- **OTEL Collector**: Check logs and `/metrics` endpoint for activity.

---

## 🧹 Cleanup

```bash
docker-compose down -v
```

This will stop and remove all containers and volumes.

---

## 📘 Notes

- Ensure all services are on the same Docker network (`otel-net`).
- OTEL Collector exports logs to Loki using the `otlphttp` exporter.
- Prometheus scrapes metrics from OTEL Collector and stores them.
- Grafana visualizes both logs and metrics.

---

Happy Observing! 🎯
