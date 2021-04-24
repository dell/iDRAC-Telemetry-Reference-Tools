# Telemetry Reference Tools - Design Philosophy 

The toolset is designed with scalability and flexibility in to consideration and can be ideally deployed as containers.

Flexibility - Major functionalities like remote source (iDRAC) discovery, credentials management for authorization, and telemetry report processing are abstracted as separate standalone applications. All IPC are through the message bus. Provided the IPC message interface structure is maintained, applications can be easily replaced or extended to suite the environments these applications are targeted to use. One or more ingest applications can be run  to perform metrics ingestion into one or more database of choice.

Scalability - Additional endpoints of iDRACs can be supported by adding more containers as it is needed to support the additional processing and data load in the environment.

Following big databases/analytics platforms are intergrated and tested at this toolset.
* ElasticSearch(ELK stack) 
* Prometheus database
* Timescale Database (PostgreSQL)
* Influx DB


![Screenshot](highleveldesign.png)


## Components 

* idrac-telemetry-receiver
    simpleauth and simpledisc applications - Abstracts the discovery and authentication functions
    redfishread application - Make SSE (Server Sent Event) connection with each discovered data sources(iDRACs) and process the telemetry report streams. iDRAC Telemetry reports are DMTF redfish compliant.   
* sink applications - Ingest the report streams into specific analytical solution.
    timescalepump - Ingest timeseries metrics into Elasticsearch database.
    influxpump - Ingest timeseries metrics into InfluxDB database.
    prometheuspump - Ingest timeseries metrics into Prometheus database.
    timescalepump - Ingest timeseries metrics into TimeScale database.

