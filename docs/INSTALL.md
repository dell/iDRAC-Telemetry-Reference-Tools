# Installation

- [Installation](#installation)
  - [Navigation](#navigation)
  - [Splunk](#splunk)
    - [Installing Splunk](#installing-splunk)
    - [Configure Metrics Index](#configure-metrics-index)
    - [Configure HTTP Event Collector](#configure-http-event-collector)
  - [Elasticsearch](#elasticsearch)
  - [Pre Install for OpenTelemetry](#opentelemetry)
  - [Do for All Pipelines](#do-for-all-pipelines)
    - [Install Docker](#install-docker)
    - [Install docker-compose](#install-docker-compose)
    - [Deploying the Framework](#deploying-the-framework)
    - [Post Install for Elasticsearch](#post-install-for-elasticsearch)
    - [Post Install for InfluxDB, Prometheus, or TimescaleDB](#post-install-for-influxdb-prometheus-or-timescaledb)
   - [Kafka Configuration](#kafka-configuration)
  - [victoria DB deployment](#Victoria-DB-deployement)

## Navigation

- [Main README](../README.md)
- [Architecture](ARCHITECTURE.md)
- [Debugging](DEBUGGING.md)
- [Install](INSTALL.md)

Before following these instructions make sure you have followed the instructions in 
[Getting Started](../README.md#getting-started). Follow the instructions for the time series database of your choice and then perform the instructions listed under [Do for All Pipelines](#do-for-all-pipelines).

## Splunk

### Installing Splunk

**NOTE**: Splunk server install instructions tested on RHEL 8.3
**NOTE** Currently, we are not using a Splunk container and
are running Splunk from a manual installation detailed below

1. Download [trial of Splunk](https://www.splunk.com/en_us/download/splunk-enterprise.html?skip_request_page=1)
2. Follow [Splunk installation instructions](https://docs.splunk.com/Documentation/Splunk/8.2.4/Installation/InstallonLinux)
3. By default it will install to /opt/splunk. Run `/opt/splunk/bin/splunk start` (I suggest you do this in tmux or 
   another terminal emulator)
4. Run the following command: `vim /opt/splunk/etc/apps/splunk_httpinput/default/inputs.conf` and make sure your 
   config looks like this:

    ```
    [http]
    disabled=0
    port=8088
    enableSSL=0
    dedicatedIoThreads=2
    maxThreads = 0
    maxSockets = 0
    useDeploymentServer=0
    # ssl settings are similar to mgmt server
    sslVersions=*,-ssl2
    allowSslCompression=true
    allowSslRenegotiation=true
    ackIdleCleanup=true
    ```

5. Run `firewall-cmd --permanent --zone public --add-port={8000/tcp,8088/tcp} && firewall-cmd --reload`
6. Make splunk start on boot with `/opt/splunk/bin/splunk enable boot-start`

### Configure Metrics Index

1. Browse to your Splunk management dashboard at `<IP>:8000`.
2. Go to Settings -> Indexes

![](../images/2022-02-24-10-59-57.png)

3. In the top right of the screen click "New Index"
4. Create a name, set Index Data Type to Metrics, and Timestamp Resolution to Seconds

![](../images/2022-02-24-11-01-12.png)

### Configure HTTP Event Collector

1. Browse to your Splunk management dashboard at `<IP>:8000`.
2. Go to Settings -> Data Inputs

![](../images/2022-02-24-10-08-22.png)

3. On the following screen click "Add new" next to HTTP Event Collector

![](../images/2022-02-24-10-09-42.png)

4. Select any name you like for the collector and click "Next" at the top of the screen
5. Select "Automatic" for Source type and for Index select the metrics index you created previously

![](../images/2022-02-24-11-02-38.png)

6. Click Review at the top, make sure everything is correct and then click "Submit" (again at the top)

At this juncture, you have done everything you need to on the Splunk side to get everything up and running. Next you 
need to finish configuring the docker pipeline. Proceed to [Do for All Pipelines](#do-for-all-pipelines)

## Elasticsearch

For Elasticsearch, there are some external settings you must configure first. The below instructions are written for
Linux and were tested on Ubuntu 20.04.3 LTS.

1. Set vm.max_map_count to at least 262144
    1. `grep vm.max_map_count /etc/sysctl.conf`. If you do not see `vm.max_map_count=262144` edit the file and add
       that line.
    2. You can apply the setting to a live system with `sysctl -w vm.max_map_count=262144`
2. Depending on whether this is a lab or production there are several other settings which should be configured to
   tune ES' performance according to your system. See: https://www.elastic.
   co/guide/en/elasticsearch/reference/current/docker.html

Next you need to finish configuring the docker pipeline. Proceed to [Do for All Pipelines](#do-for-all-pipelines)

## OpenTelemetry

Set the following environment variables per the OpenTelemetry Collector configuration

* OTEL_COLLECTOR
   * Base URL for OTel Collector (http://\<IP\>:\<PORT\>)
* OTEL_CACERT
   * CA Cert for OTel Collector
* OTEL_CLIENT_CERT
   * Client cert for OTel Collector
* OTEL_CLIENT_KEY
   * Client key for OTel Collector
* OTEL_SKIP_VERIFY
   * true/false

## Do for All Pipelines

These instructions apply to all pipelines

### Install Docker

See [Get Docker](https://docs.docker.com/get-docker/) for installation details. You will need to be able to run 
Docker commands as a user rather than root. See 
[these instructions](https://docs.docker.com/engine/install/linux-postinstall/). On most Linux distributions this 
consists of running:

```bash
sudo groupadd docker
sudo usermod -aG docker $USER
```

and then log out and back in. Run `docker run hello-world` as the user in question to test your privileges.

### Install docker-compose

You will also need to install docker-compose version 2. The code **will not work** with version 1. Instructions for a standalone installation of 
docker compose version 2 are [here](https://docs.docker.com/compose/install/standalone/). The following versions of docker-compose have been tested - 
* 2.3.3
* 2.6.0
* 2.17.1
* 2.20.2

Note - 
* Standalone docker compose installation is required because backward compatibility for the docker-compose command is needed to run `compose.sh`.
* Use docker compose version greater than 2.3.3 for a faster container setup. 

### Deploying the Framework

**NOTE** These instructions are performed on whatever host you would like to use to connect to all of your 
iDRACs

1. git clone https://github.com/dell/iDRAC-Telemetry-Reference-Tools
2. (For Splunk) Set the following environment variables as per the "HTTP Event Collector" configuration in the Splunk.  
       SPLUNK_HEC_KEY=\<Token value\>  
       SPLUNK_HEC_URL=http://\<Splunk hostname or ip\>:\<HTTP Port Number\>  
       SPLUNK_HEC_INDEX=\<Index name\>  
3. Next run `bash compose.sh`. The options you use will depend on what you want to do. There are five different 
   "pumps" for the 6 different databases: `--influx-pump`, `--prometheus-pump`, `--splunk-pump`, `--elk-pump`, 
   `--timescale-pump`, `--otel-pump`. These pumps are responsible for feeding the data from the pipeline into the pipeline of your 
   choice. The other option you may want to add is a command to build the time series database of your choosing. If 
   you already have an external instance of the database running then this won't be necessary. These options are: 
   `--influx-test-db`, `--prometheus-test-db`, `--elk-test-db`, `--timescale-test-db`. We have not currently built 
   out a splunk option. So the command to connect to build a data pipeline for an external splunk instance would be 
   `bash compose.sh --splunk-pump start` Running this command will trigger a build of all the necessary containers 
   in the pipeline as specified in [the Docker compose file](../docker-compose-files/docker-compose.yml).
   1. **WARNING** There is a known bug where docker compose throws erroneous errors. These can be safely ignored. 
      See https://github.com/dell/iDRAC-Telemetry-Reference-Tools/issues/46. It will look like this:

![](images/2022-03-03-16-56-01.png)

   2. **Running influx with grafana** This is a 2 step process of generating influx and grafana tokens and starting --influx-test-db.
    a. ./docker-compose-files/compose.sh setup --influx-test-db
    b. ./docker-compose-files/compose.sh start --influx-test-db
    

4. On your system, you will need to allow ports 8161 and 8080 through your firewall
   1. If you are running Elasticsearch, you will also need to open port 5601 for Kibana if you chose to run compose 
      with the `--elk-test-db` option.
   2. If you ran compose with the `--influx-test-db`, `--prometheus-test-db`, or `--timescale-test-db` options you will need to open port 3000 for Grafana.
5. After you run this, the next step is to specify the iDRACs of the machines you are using. There is a webgui 
   called `configui` which by default runs on port 8080. Browse to it and click "Add New Service". Alternatively you 
   can upload a CSV file with three columns and no headers with each line in the format: `host, username, password` 
   for each iDRAC.

![](images/2022-03-03-17-26-26.png)

6. Below is an example CSV file.

![](images/2022-03-03-17-30-51.png)

7. Refresh the page and you should see your host appear in the list.
8. At this point the pipeline should be up and running. You can run `docker ps -a` to make sure all the containers 
   in the pipeline are running. To confirm the final stage of the pump is working run `docker logs <pump_name>` and 
   you should see that the pump is forwarding events.
9.  For additional troubleshooting steps see [DEBUGGING.md](DEBUGGING.md)

### Post Install for Elasticsearch

This is not required. It only demonstrates a possible Elasticsearch workflow.

1. To configure the data source and visualization dashboards please access Kibana homepage in the
   browser (`http://<YOUR_IP>:5601`)
2. Select Stack Management from the Management section in the tools menu. Now go to the Data -> Index Management tab and find the index named "poweredge_telemetry_metrics". In the index pattern tab, create an index pattern called `poweredge_telemetry_metrics*`.

![](../images/2022-03-02-06-00-32.png)

![](../images/2022-03-02-06-00-55.png)

3. Next, browse to the discover tab to view the ingested data. You can find fields of interest and view the data in tabular form with your chosen fields.

![](../images/2022-03-02-06-01-58.png)

![](../images/2022-03-02-06-02-16.png)

4. Next we will create charts which can then be placed in a dashboard. We will configure an aggregation metric on CRCErrorCount:

![](../images/2022-03-02-06-05-08.png)

![](../images/2022-03-02-06-05-24.png)

### Post Install for Influx UI
1. Browse to influx (`http://<YOUR_IP>:8086`) using the admin/DOCKER_INFLUXDB_INIT_PASSWORD ![](../images/Tokens.PNG)
2. load data ![](../images/Influxdb-loadData.png)
3. view telmetry metrics from my-org-bucket database ![](../images/Influxdb-my-org-bucket.png)

### Post Install for InfluxDB, Prometheus, or TimescaleDB

1. Browse to Grafana (`http://<YOUR_IP>:3000`)
2. Add InfluxDB datasource, select the url (`http://influx:8086`) with the header `Authorization: Token DOCKER_INFLUXDB_INIT_ADMIN_TOKEN`, and `organization:my-org`. Correct addition of datasource will show the available buckets. ![](../images/grafanaAddDataSources.png) 
3. Visualize metrics using add panel and wrting query for the respective metric in the Query inspector(quick way is to get the query from the influxUI)

![](../images/Influx-Grafana-FanRPMReading.png)

3. For Prometheus:

![](../images/2022-03-02-06-10-39.png)

4. For TimescaleDB:

![](../images/2022-03-02-06-12-47.png)

## Kafka Configuration

Kafka can be configured through the UI, an HTTP request or environment variables.

### Configuring through UI

1. Browse to the UI  - http://localhost:8080 
2. Click on the `Config` button in the `Kafka Broker Configuration` section.
3. Fill the `Broker ID` and `Topic Name` in the pop-up.
4. Check the `TLS` and/or `Client Auth` boxes if the kafka instance needs those details for a successful connection
   - If the `TLS` option is checked, please choose the `Kafka CACert` and select `Skip Hostname Verification` if you want to skip hostname verification.
   - If the `Client Auth` option is checked, please choose the `Kafka Client Certificate` and `Kafka Client Key` from the local filesystem.

### Configuring through HTTP request

1. Send a `POST` request to `http://localhost:8080/api/v1/KafkaConfig` with the following body 

```json
{
   "kafkaBroker": "<Broker ID>", 
   "kafkaTopic": "<Topic>",
   "kafkaPartition": "<Partition>",
   "kafkaCACert": "<CA Cert>",
   "kafkaSkipVerify": true/false,
   "kafkaClientCert": "<Client Cert>",
   "kafkaClientKey": "<Client Key>"
}
```
   - Sample cURL request
   ```
   curl -N -k -X POST -H 'content-type: application/json' http://localhost:8080/api/v1/KafkaConfig  -d '{"kafkaBroker": "http://localhost:9090", "kafkaTopic": "metrics"}'
   ```

### Configuring through environment variables

Define the following environment variables in your local environment 

```
export KAFKA_BROKER=<kafka-broker-ip>:9092
export KAFKA_TOPIC=<topic>
export KAFKA_PARTITION=<partition>
export KAFKA_CACERT="<CA Cert>"
export KAFKA_CLIENT_CERT="<Client Cert>"
export KAFKA_CLIENT_KEY="<Client Key>"
export KAFKA_SKIP_VERIFY=true/false
```

## Victoria DB deployement

# VictoriaMetrics Configuration

**VictoriaMetrics** is a high-performance, scalable, and cost-efficient **time-series database** designed for fast ingestion and querying of **Prometheus-compatible metrics**.  
Within the iDRAC Telemetry framework, the **VictoriaPump** component collects **iDRAC Redfish telemetry data** from the **ActiveMQ message bus** and pushes it directly into **VictoriaMetrics** using the **Prometheus exposition format**.

This **push-based model** eliminates the need for a Prometheus intermediary, providing:

* Lower latency
* Reduced memory usage
* Simplified deployment


## Victoria Pipeline Configuration Using Environment Variables

Configure credentials and the target URL of the VictoriaMetrics using the following environment variables.

| Variable | Description | Example |
|-----------|--------------|----------|
| `VICTORIA_URL` | URL of the VictoriaMetrics instance | `http://victoriametrics:8428` |
| `VICTORIA_USER` | Username for authentication | `admin` |
| `VICTORIA_PASSWORD` | Password for authentication | `secret` |

> **Note:**  
 * These variables can be set in your terminal session or defined in your Docker Compose file.  
* This approach makes deployments consistent, flexible, and portable across environments.


## Start VictoriaPump and VictoriaMetrics

To launch the entire iDRAC Telemetry pipeline using VictoriaMetrics, execute the following command:

```
./compose.sh start --victoria-db --victoria-pump
```

If you already have an external VictoriaMetrics instance running, you can skip the local database by omitting the `--victoria-db` flag:

```
./compose.sh start --victoria-pump
```


## Add iDRAC Endpoints

1. Open your web browser and navigate to: http://localhost:8080. Replace `localhost` with your host machine’s IP address. <img width="1434" height="447" alt="image" src="https://github.com/user-attachments/assets/2b19b5ec-c8d2-44e3-ba2f-17c948eb89f0" />

2. From the **Config UI**, do the following:
	1. Click **Add Endpoint**.  
	2. Enter your **iDRAC IP address**, **username**, and **password**.  
	3. Click **Save**.

This enables the Telemetry Service to start collecting telemetry data from your Dell PowerEdge servers using the Redfish protocol.


## Verify VicroriaPump Container Logs

To confirm that **VictoriaPump** is running and pushing data correctly into VictoriaMetrics, view the container logs using the following command:

```
docker logs idrac-telemetry-reference-tools-victoriapump-standalone-1
```

This screenshot shows real-time logs from the VictoriaPump container. Each log entry in the following screenshot confirms the telemetry data flow from iDRAC to VictoriaMetrics.

<img width="811" height="109" alt="image" src="https://github.com/user-attachments/assets/2c594e19-0789-4a03-ad6f-25f310dbbba0" />



## Verify Telemetry Data in VictoriaMetrics

Once VictoriaPump starts sending data, you can validate the metrics stored in VictoriaMetrics using **VMUI**.

Open your browser and navigate to: http://localhost:8428/vmui

You will see metric statistics and telemetry data collected from iDRAC.

<img width="589" height="437" alt="image" src="https://github.com/user-attachments/assets/2cd89e19-2087-4497-b536-10c44bd96df7" />


## Filter Telemetry Data using VMUI

You can filter and view telemetry metrics using queries in **VMUI**.

For example, running the following query displays the detailed temperature readings for each hardware component,

{name="PowerEdge_TemperatureReading", FQDD!=""}
<img width="1853" height="672" alt="image" src="https://github.com/user-attachments/assets/e56be821-4faf-4fe5-bc01-25dc3d24fdc7" />

<img width="1878" height="476" alt="image" src="https://github.com/user-attachments/assets/d64dea24-ceeb-40f2-927e-a4a11b64770b" />



(Re-)build the docker containers from source

```
./compose.sh --build  --victoria-pump --victoria-db start
```

If you already have an external VictoriaMetrics instance running, you can skip the local database by omitting the --victoria-db flag:

```
./compose.sh --build  --victoria-pump start
```

## iDRAC Telemetry Receiver container
 
``cmd/idrac-telemetry-receiver/idrac-telemetry-receiver.go`` script is created specific for [dell/omnia](https://github.com/dell/omnia) usecases. This go script internally invokes dbdiscauth, configui and redfishread go script and avoid need of having 3 different docker containers. To build this container use the docker file  ``docker-compose-files/Dockerfile.telemetry_receiver``. Run the below command to build image.

```
docker build -t idrac-telemetry-receiver:latest -f docker-compose-files/Dockerfile.telemetry_receiver .
```

































