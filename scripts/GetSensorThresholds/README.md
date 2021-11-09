# GetSensorThresholds.py

## Prerequisites
Python 3.X

## HELP

 python tests/idrac_ft/telemetry_ft/GetSensorThresholds.py -h
usage: GetSensorThresholds.py [-h] ip user password metricproperty

Python script using Redfish http get to find the sensor thresholds of a given
metric property or a metric report.

positional arguments:
  ip              iDRAC IP address
  user            iDRAC username
  password        iDRAC password
  metricproperty  Metric property or metric report

optional arguments:
  -h, --help      show this help message and exit

Examples:
find the sensor thresholds of a given metric property or metric report:
- To get sensor threshold values for a metricproperty.
1)GetSensorThresholds.py 1.10.1.10 root mypasswd /redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellNumericSensors/iDRAC.Embedded.1_0x23_SystemBoardSYSUsage#CurrentReading
- To get sensor threshold values for all the metricproperties in a metric report.
2)GetSensorThresholds.py 1.10.1.10 root mypasswd Sensor
3)GetSensorThresholds.py 1.10.1.10 root mypasswd /redfish/v1/TelemetryService/MetricReports/Sensor

## NOTES: 
Please note the argument list is positional arguments, example provided.

## RESULT:
## with metric property option:
python tests/idrac_ft/telemetry_ft/GetSensorThresholds.py 1.10.1.10 root mypasswd /redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellNumericSensors/iDRAC.Embedded.1_0x23_SystemBoardInletTemp
Lower Threshold Critical:-7
Lower Threshold Warning:3
Upper Threshold Critical:42
Upper Threshold Warning:38
Successfully found the thresholds for '/redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellNumericSensors/iDRAC.Embedded.1_0x23_SystemBoardInletTemp'.
## with metric report option:
python tests/idrac_ft/telemetry_ft/GetSensorThresholds.py 100.69.116.150 root calvin Sensor
For metric report: Sensor
Thresholds for mp:'/redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellNumericSensors/0x17__Fan.Embedded.1' are below:
Lower Threshold Critical:480
Lower Threshold Warning:840
Upper Threshold Critical property is null
Upper Threshold Warning property is null
Thresholds for mp:'/redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellNumericSensors/0x17__Fan.Embedded.4' are below:
Lower Threshold Critical:480
Lower Threshold Warning:840
Upper Threshold Critical property is null
Upper Threshold Warning property is null
...
Thresholds for mp:'/redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellNumericSensors/0x17__Fan.Embedded.6' are below:
Lower Threshold Critical:480
Lower Threshold Warning:840
Upper Threshold Critical property is null
Upper Threshold Warning property is null
Successfully found the thresholds for 'Sensor'.

## RESULTS LOG Level:
log level currently is warning, could be increased to info,debug, to see detailed log.

## LICENSE
This project is licensed under Apache 2.0 License. See the [LICENSE](LICENSE.md) for more information.

## Contributing
We welcome your contributions this reference toolset. See [Contributing Guidelines](CONTRIBUTING.md) for more details.
You can refer our [Code of Conduct](CODE_OF_CONDUCT.md) here.

## Disclaimer
The software applications included in this package are  considered "BETA". They are intended for testing use in non-production  environments only. 

No support is implied or offered. Dell Corporation assumes no  responsibility for results or performance of "BETA" files.  Dell does NOT warrant that the Software will meet your requirements, or that operation of the Software will be uninterrupted or error free. The Software is provided to you "AS IS" without warranty of any kind. DELL DISCLAIMS ALL WARRANTIES, EXPRESS OR IMPLIED, INCLUDING, WITHOUT LIMITATION, THE IMPLIED WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, TITLE AND NON-INFRINGEMENT. The entire risk as to the results and performance of the Software is assumed by you. No technical support provided with this Software. 

IN NO EVENT SHALL DELL OR ITS SUPPLIERS BE LIABLE FOR ANY DIRECT OR INDIRECT DAMAGES WHATSOEVER (INCLUDING, WITHOUT LIMITATION, DAMAGES FOR LOSS OF BUSINESS PROFITS, BUSINESS INTERRUPTION, LOSS OF BUSINESS INFORMATION, OR OTHER PECUNIARY LOSS) ARISING OUT OF USE OR INABILITY TO USE THE SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGES. Some jurisdictions do not allow an exclusion or limitation of liability for consequential or incidental damages, so the above limitation may not apply to you.


## Support
  * To report any issue, create an issue [here](https://github.com/dell/iDRAC-Telemetry-Reference-Tools/issues).
  * If any requirements have not been addressed, then create an issue [here](https://github.com/dell/iDRAC-Telemetry-Reference-Tools/issues).
  * To provide feedback to the development team, send an email to **idractelemetryteam@dell.com**.
