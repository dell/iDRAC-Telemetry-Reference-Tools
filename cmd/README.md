# Telemetry Reference Tools - Design Philosophy 

The toolset is designed with scalability and flexibility in to consideration and can be ideally deployed as containers. Additional endpoints of iDRACs can be supported by adding more containers as it is needed to support the additional processing and data load. Flexibility allows ingesting to perform downstream analytics in to one or more database of choice.  

Following big databases/analytics platforms are intergrated at this time. 
* ElasticSearch(ELK stack) 
* Prometheus database
* Timescale Database (PostgreSQL)
* Influx DB


![Screenshot](highleveldesign.png)


## Components 
* simpleauth and simpledisc applications - Abstracts the discovery and authentication functions
* redfishread application - Abstracts as the SSE client for data sources(iDRACs) and process the telemetry report streams. Telemetry reports are DMTF redfish compliant.   
* pump applications - Ingest the report streams in to specific big databases. 

