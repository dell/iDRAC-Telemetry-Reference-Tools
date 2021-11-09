#
# GetSensorThresholds.py Python script using Redfish http get to find the sensor thresholds of a given metric property.
#
#
#
# _author_ = Sailaja Mahendrakar <sailaja_mahendrakar@dell.com>
# _version_ = 1.0
#
# Copyright (c) 2021, Dell Technologies, Inc.
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
import logging, logging.handlers
from logging import StreamHandler, Formatter
import sys
import warnings
from argparse import RawTextHelpFormatter

import requests


warnings.filterwarnings("ignore")
logger = logging.getLogger()
logger.setLevel(logging.WARNING) # Change to logging.DEBUG for detailed logs

parser = argparse.ArgumentParser( description = "Python script using Redfish http get to find the sensor thresholds of a given metric property or metric report.",
                                  epilog = 'Examples: \nfind the sensor thresholds of a given metric property or metric report:\
                                  \n- To get sensor threshold values for a metricproperty.\
                                  \n1)GetSensorThresholds.py 1.10.1.10 root mypasswd /redfish/v1/Systems/System.Embedded.1/Oem/Dell/DellNumericSensors/iDRAC.Embedded.1_0x23_SystemBoardSYSUsage#CurrentReading\
                                - To get sensor threshold values for all the metricproperties in a metric report.\
                                  \n2)GetSensorThresholds.py 1.10.1.10 root mypasswd Sensor \
                                  \n3)GetSensorThresholds.py 1.10.1.10 root mypasswd /redfish/v1/TelemetryService/MetricReports/Sensor',
                                  formatter_class=RawTextHelpFormatter)


parser.add_argument('ip',  help = 'iDRAC IP address')
parser.add_argument('user',  help = 'iDRAC username')
parser.add_argument('password',  help = 'iDRAC password')
parser.add_argument('metricproperty', help = 'Metric property or Metric Report')

args = parser.parse_args()

idrac_ip = args.ip
idrac_username = args.user
idrac_password = args.password
metricproperty = args.metricproperty

def get_sensor_thresholds():

    # to remove INFO:root: or DEBUG:root: from logging
    FORMAT = "%(message)s"
    formatter = logging.Formatter(fmt=FORMAT)
    handler = logging.StreamHandler()
    handler.setFormatter(formatter)
    logger.addHandler(handler)

    MetricPropertiesListDict = {}
    ####################################
    if '#' in metricproperty:
        # remove #CurrentReading from metricproperty
        mprop = metricproperty.split('#')[0]
        MetricPropertiesListDict[mprop] = mprop
    else:
        MetricReport_uri = "/redfish/v1/TelemetryService/MetricReports/"

        logging.warning("For metric report: " + str(metricproperty))

        if 'MetricReports/' in metricproperty:
            url = 'https://{}{}'.format(idrac_ip,metricproperty)# metricproperty is the report here
        else:
            url = 'https://{}{}'.format(idrac_ip,MetricReport_uri+metricproperty)# metricproperty is the report here
        headers = {'content-type': 'application/json'}
        response = requests.get(url, headers = headers, verify = False,auth = (idrac_username, idrac_password))
        if response.status_code != 200:
            logging.error("FAIL, status code for reading report is not 200, code is: {}".format(response.status_code))
            sys.exit()
        try:
            logging.debug("Successfully pulled metric report")
            OutputD = json.loads(response.text)
            metricvalues = OutputD.get("MetricValues",{})
            for metric in metricvalues:
                #logging.warning("got metric " + str(metric))
                mid = metric["MetricId"]
                mprop = metric["MetricProperty"]
                MetricPropertiesListDict[mprop] = mprop
        except Exception as e:
            logging.error("FAIL: detailed error message: {0}".format(e))
            sys.exit()

    error = 0
    for mprop in MetricPropertiesListDict:
        # remove #CurrentReading from metricproperty
        thresholduri = mprop.split('#')[0]

        logging.warning("Thresholds for mp:'"+str(thresholduri)+"' are below:")

        url = 'https://{}{}'.format(idrac_ip,thresholduri)
        headers = {'content-type': 'application/json'}
        response = requests.get(url, headers = headers, verify = False,auth = (idrac_username, idrac_password))
        if response.status_code != 200:
            logging.error("FAIL, status code for reading attributes is not 200, code is: {}".format(response.status_code))
            sys.exit()
        try:
            logging.debug("Successfully pulled threshold attributes")
            thresholdAttr_dict = json.loads(response.text)
            UnitModifier = thresholdAttr_dict.get('UnitModifier',{})
            ReadingUnits = thresholdAttr_dict.get('ReadingUnits',{})
            makedecimal = 0
            logging.debug("Readindunits is :{}".format(ReadingUnits))
            if ReadingUnits == 'Amps' or ReadingUnits == 'Watts' or ReadingUnits == 'Volts' :
                makedecimal = 1

            if UnitModifier == None:
                logging.error("FAIL,No Unit Modifier")
                sys.exit()

            # need to multiply with 10 power unit modifier
            logging.debug("-----------------------------------------------------")
            LwrThresholdCrt = thresholdAttr_dict.get('LowerThresholdCritical',{})
            if LwrThresholdCrt == {}:
                logging.error("FAIL,No LwrThresholdCrt property")
                sys.exit()

            if LwrThresholdCrt != None:
                LwrThresholdCrt = LwrThresholdCrt*pow(10,UnitModifier)
                if makedecimal == 0:
                    logging.warning("Lower Threshold Critical:{}".format(int(LwrThresholdCrt)))
                else:
                    logging.warning("Lower Threshold Critical :{}".format(float(LwrThresholdCrt)))
            else:
                logging.warning("Lower Threshold Critical property is null")

            LwrThresholdWarn = thresholdAttr_dict.get('LowerThresholdNonCritical',{})
            if LwrThresholdCrt == {}:
                logging.error("FAIL,No LwrThresholdWarn property")
                sys.exit()

            if LwrThresholdWarn != None:
                LwrThresholdWarn = LwrThresholdWarn*pow(10,UnitModifier)
                if makedecimal == 0:
                    logging.warning("Lower Threshold Warning:{}".format(int(LwrThresholdWarn)))
                else:
                    logging.warning("Lower Threshold Warning :{}".format(float(LwrThresholdWarn)))
            else:
                logging.warning("Lower Threshold Warning property is null")

            UpprThresholdCrt = thresholdAttr_dict.get('UpperThresholdCritical',{})
            if UpprThresholdCrt == {}:
                logging.error("FAIL,No UpprThresholdCrt property")
                sys.exit()

            if UpprThresholdCrt != None:
                UpprThresholdCrt = UpprThresholdCrt*pow(10,UnitModifier)
                if makedecimal == 0:
                    logging.warning("Upper Threshold Critical:{}".format(int(UpprThresholdCrt)))
                else:
                    logging.warning("Upper Threshold Critical :{}".format(float(UpprThresholdCrt)))
            else:
                logging.warning("Upper Threshold Critical property is null")

            UpprThresholdWarn = thresholdAttr_dict.get('UpperThresholdNonCritical',{})
            if UpprThresholdWarn == {}:
                logging.error("FAIL,No UpprThresholdWarn property")
                sys.exit()

            if UpprThresholdWarn != None:
                UpprThresholdWarn = UpprThresholdWarn*pow(10,UnitModifier)
                if makedecimal == 0:
                    logging.warning("Upper Threshold Warning:{}".format(int(UpprThresholdWarn)))
                else:
                    logging.warning("Upper Threshold Warning :{}".format(float(UpprThresholdWarn)))
            else:
                logging.warning("Upper Threshold Warning property is null")

            logging.debug("UnitModifier:{}".format(UnitModifier))
            logging.debug("-----------------------------------------------------")

        except Exception as e:
            logging.error("FAIL: detailed error message: {0}".format(e))
            sys.exit()

    return metricproperty
    
    ########################################

if __name__ == "__main__":
    thresholduri = get_sensor_thresholds()
    logging.warning("Successfully found the thresholds for '{}'.".format(thresholduri))


