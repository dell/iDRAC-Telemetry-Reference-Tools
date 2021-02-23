
# Telemetry Reference Tools  


PowerEdge Telemetry reference toolset collects the metric reports from various devices at the PowerEdge compute and outlines a reference design to integrate with external big databases for downstream analytics and visualization.


![Screenshot](overview.png)

PowerEdge servers with iDRAC9 version 4.00 or higher and Datacenter license can stream server telemetry data out for downstream analytics and consumption. iDRAC telemetry data simply put is a timestamped metrics that represent various datapoints about the server components and is streamed out in a format defined by the DMTF Telemetry Redfish standard. This datapoints includes information from various sensors, storage and networking subsystem and helps IT administrators better understand health and necessary details about their server infrastructure.


## Prerequisites
* Go - https://golang.org/
* ActiveMQ (Message broker framework)

## Setup Instructions  
Please reference the included Docker Compose files for setup instructions.

## LICENSE
This project is licensed under Apache 2.0 License. See the [LICENSE](LICENSE.md) for more information.

## Contributing
We welcome your contributions this reference toolset. See [Contributing Guidelines](CONTRIBUTING.md) for more details.
You can refer our [Code of Conduct](CODE_OF_CONDUCT.md) here.

## Support
  * To report any issue, create an issue [here](https://github.com/dell/iDRAC-Telemetry-Reference-Tools/issues).
  * If any requirements have not been addressed, then create an issue [here](https://github.com/dell/iDRAC-Telemetry-Reference-Tools/issues).
  * To provide feedback to the development team, send an email to **idractelemetryteam@dell.com**.
