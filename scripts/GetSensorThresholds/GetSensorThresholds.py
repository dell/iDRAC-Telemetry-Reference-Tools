#
# GetSensorThresholds.py Python script using Redfish http get to find the sensor thresholds of a given metric property.
#
#
#
# _author_ = Sailaja Mahendrakar <sailaja_mahendrakar@dell.com>
# _version_ = 1.1
#
# Copyright (c) 2022, Dell Technologies, Inc.
#
# This software is licensed to you under the GNU General Public License,
# version 2 (GPLv2). There is NO WARRANTY for this software, express or
# implied, including the implied warranties of MERCHANTABILITY or FITNESS
# FOR A PARTICULAR PURPOSE. You should have received a copy of GPLv2
# along with this software; if not, see
# http://www.gnu.org/licenses/old-licenses/gpl-2.0.txt.
#


import argparse
import json
import logging
import logging.handlers
import sys
import warnings
from argparse import RawTextHelpFormatter
import requests

warnings.filterwarnings("ignore")
logger = logging.getLogger()
logger.setLevel(logging.WARNING)  # Change to logging.DEBUG for detailed logs

parser = argparse.ArgumentParser(
    description="Python script using Redfish http get to find the sensor thresholds of a given metric property or metric report.",
    epilog='Examples: \nfind the sensor thresholds of a given metric property or metric report:\
                                  \n- To get sensor threshold values for a metricproperty.\
                                  \n1)GetSensorThresholds.py 1.10.1.10 root mypasswd /redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellNumericSensors/iDRAC.Embedded.1_0x23_SystemBoardSYSUsage#CurrentReading\
                                - To get sensor threshold values for all the metricproperties in a metric report.\
                                  \n2)GetSensorThresholds.py 1.10.1.10 root mypasswd Sensor \
                                  \n3)GetSensorThresholds.py 1.10.1.10 root mypasswd /redfish/v1/TelemetryService/MetricReports/Sensor',
    formatter_class=RawTextHelpFormatter)

parser.add_argument('ip', help='iDRAC IP address')
parser.add_argument('user', help='iDRAC username')
parser.add_argument('password', help='iDRAC password')
parser.add_argument('metricproperty', help='Metric property or Metric Report')

args = parser.parse_args()

idrac_ip = args.ip
idrac_username = args.user
idrac_password = args.password
metric_property = args.metricproperty


def get_sensor_thresholds():
    # to remove INFO:root: or DEBUG:root: from logging
    FORMAT = "%(message)s"
    formatter = logging.Formatter(fmt=FORMAT)
    handler = logging.StreamHandler()
    handler.setFormatter(formatter)
    logger.addHandler(handler)

    metric_properties_list_dict = {}
    ######################################################################################
    # Handle instances where the user specifies a specific property they want to examine #
    ######################################################################################
    if '#' in metric_property:
        # remove #CurrentReading from metricproperty
        mprop = metric_property.split('#')[0]
        metric_properties_list_dict[mprop] = mprop

    #################################################################################
    # Handle instances where the user specifies a specific report they want to pull #
    #################################################################################
    else:
        metric_report_uri = "/redfish/v1/TelemetryService/MetricReports/"

        logging.warning("For metric report: " + str(metric_property))

        if 'MetricReports/' in metric_property:
            url = 'https://{}{}'.format(idrac_ip, metric_property)  # metricproperty is the report here
        else:
            url = 'https://{}{}'.format(idrac_ip,
                                        metric_report_uri + metric_property)  # metricproperty is the report here
        headers = {'content-type': 'application/json'}
        response = requests.get(url, headers=headers, verify=False, auth=(idrac_username, idrac_password))
        if response.status_code != 200:
            logging.error("FAIL, status code for reading report is not 200, code is: {}".format(response.status_code))
            sys.exit()
        try:
            logging.debug("Successfully pulled metric report")
            output_d = json.loads(response.text)
            metric_values = output_d.get("MetricValues", {})
            for metric in metric_values:
                if 'MetricProperty' in metric:
                    mprop = metric["MetricProperty"]
                    metric_properties_list_dict[mprop] = mprop
                else:
                    logging.error("FAIL, The value 'MetricProperty' is not available in the results. This typically"
                                  " indicates that you have iDRAC version <6.x. Either upgrade your iDRAC or you can"
                                  " use metric properties instead of metric reports.")
                    sys.exit()
        except Exception as e:
            logging.error("FAIL: detailed error message: {0}".format(e))
            sys.exit()

    for mprop in metric_properties_list_dict:
        # remove #CurrentReading from metricproperty
        threshold_uri = mprop.split('#')[0]

        logging.warning("Thresholds for mp:'" + str(threshold_uri) + "' are below:")

        url = 'https://{}{}'.format(idrac_ip, threshold_uri)
        headers = {'content-type': 'application/json'}
        response = requests.get(url, headers=headers, verify=False, auth=(idrac_username, idrac_password))
        if response.status_code != 200:
            logging.error("FAIL, status code for reading attributes is not 200, code "
                          "is: {}".format(response.status_code))
            sys.exit()
        try:
            logging.debug("Successfully pulled threshold attributes")
            threshold_attr_dict = json.loads(response.text)
            if threshold_attr_dict["CurrentState"] == "Unknown":
                logging.error("The current state of %s is 'Unknown'. This means the values displayed here are unlikely"
                              " to be correct. Is the server on? If it is on, you may have a hardware issue or the"
                              " sensor is not reporting for some other reason." % threshold_attr_dict["@odata.type"])
            unit_modifier = threshold_attr_dict.get('UnitModifier', {})
            reading_units = threshold_attr_dict.get('ReadingUnits', {})
            make_decimal = 0
            logging.debug("Readindunits is :{}".format(reading_units))
            if reading_units == 'Amps' or reading_units == 'Watts' or reading_units == 'Volts':
                make_decimal = 1

            if unit_modifier is None:
                logging.error("FAIL,No Unit Modifier")
                sys.exit()

            # need to multiply with 10 power unit modifier
            logging.debug("-----------------------------------------------------")
            lwr_threshold_crt = threshold_attr_dict.get('LowerThresholdCritical', {})
            if lwr_threshold_crt == {}:
                logging.error("FAIL,No LwrThresholdCrt property")
                sys.exit()

            if lwr_threshold_crt is not None:
                lwr_threshold_crt = lwr_threshold_crt * pow(10, unit_modifier)
                if make_decimal == 0:
                    logging.warning("Lower Threshold Critical:{}".format(int(lwr_threshold_crt)))
                else:
                    logging.warning("Lower Threshold Critical :{}".format(float(lwr_threshold_crt)))
            else:
                logging.warning("Lower Threshold Critical property is null")

            lwr_threshold_warn = threshold_attr_dict.get('LowerThresholdNonCritical', {})
            if lwr_threshold_crt == {}:
                logging.error("FAIL,No LwrThresholdWarn property")
                sys.exit()

            if lwr_threshold_warn is not None:
                lwr_threshold_warn = lwr_threshold_warn * pow(10, unit_modifier)
                if make_decimal == 0:
                    logging.warning("Lower Threshold Warning:{}".format(int(lwr_threshold_warn)))
                else:
                    logging.warning("Lower Threshold Warning :{}".format(float(lwr_threshold_warn)))
            else:
                logging.warning("Lower Threshold Warning property is null")

            uppr_threshold_crt = threshold_attr_dict.get('UpperThresholdCritical', {})
            if uppr_threshold_crt == {}:
                logging.error("FAIL,No UpprThresholdCrt property")
                sys.exit()

            if uppr_threshold_crt is not None:
                uppr_threshold_crt = uppr_threshold_crt * pow(10, unit_modifier)
                if make_decimal == 0:
                    logging.warning("Upper Threshold Critical:{}".format(int(uppr_threshold_crt)))
                else:
                    logging.warning("Upper Threshold Critical :{}".format(float(uppr_threshold_crt)))
            else:
                logging.warning("Upper Threshold Critical property is null")

            uppr_threshold_warn = threshold_attr_dict.get('UpperThresholdNonCritical', {})
            if uppr_threshold_warn == {}:
                logging.error("FAIL,No UpprThresholdWarn property")
                sys.exit()

            if uppr_threshold_warn is not None:
                uppr_threshold_warn = uppr_threshold_warn * pow(10, unit_modifier)
                if make_decimal == 0:
                    logging.warning("Upper Threshold Warning:{}".format(int(uppr_threshold_warn)))
                else:
                    logging.warning("Upper Threshold Warning :{}".format(float(uppr_threshold_warn)))
            else:
                logging.warning("Upper Threshold Warning property is null")

            logging.debug("UnitModifier:{}".format(unit_modifier))
            logging.debug("-----------------------------------------------------")

        except Exception as e:
            logging.error("FAIL: detailed error message: {0}".format(e))
            sys.exit()

    return metric_property

    ########################################


if __name__ == "__main__":
    threshold_uri_result = get_sensor_thresholds()
    logging.warning("Successfully found the thresholds for '{}'.".format(threshold_uri_result))
