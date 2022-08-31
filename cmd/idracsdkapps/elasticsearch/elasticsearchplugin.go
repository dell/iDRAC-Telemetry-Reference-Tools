package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/elastic/go-elasticsearch/v8"
)

var mtx sync.Mutex
var dat map[string]interface{}
var reportCount = 0
var invalidJsonPkts = 0

var telemetryserverIP = "100.98.72.157"

type stagDellData struct {
	ServiceTag string `json:"ServiceTag"`
}

type stagOem struct {
	DELL stagDellData `json:"Dell"`
}

type dellData struct {
	ContextID string `json:"ContextID"`
	Label     string `json:"Label"`
	Source    string `json:"Source"`
	FQDD      string `json:"FQDD"`
}

type oemData struct {
	DELL dellData `json:"Dell"`
}

type MetricValue struct {
	MetricID  string  `json:"MetricId"`
	TimeStamp string  `json:"Timestamp"`
	Value     string  `json:"MetricValue"`
	OEM       oemData `json:"Oem"`
}

type Resp struct {
	MType          string `json:"@odata.type"`
	MContext       string `json:"@odata.context"`
	MId            string `json:"@odata.id"`
	Id             string `json:"Id"`
	Name           string `json:"Name"`
	ReportSequence string `json:"ReportSequence"`
	Timestamp      string `json:"Timestamp"`
	MetricValues   []MetricValue
	Count          int     `json:"MetricValues@odata.count"`
	ServiceTag     stagOem `json:"Oem"`
}

func checkHeader(tmp []byte) bool {

	idPresent := bytes.Contains(tmp, []byte("id: "))
	dataPresent := bytes.Contains(tmp, []byte("data: "))
	namePresent := bytes.Contains(tmp, []byte("Name"))

	if idPresent == true && dataPresent == true && namePresent == true {
		return true
	} else {
		return false
	}
}
func InsertMetricReportData(es *elasticsearch.Client, index1 int, tmp []byte) {
	var res Resp
	mtx.Lock()
	reportCount++
	err := json.Unmarshal(tmp, &dat)
	mtx.Unlock()
	// NOTE: For the time being I am using the json unmarshall function as a validator for the parsed metric reports
	if err != nil {
		// Don't panic, but keep a count of these invalid packets for analysis
		// panic(err)
		invalidJsonPkts++
		return
	}
	err = json.Unmarshal(tmp, &res)
	if err != nil {
		panic(err)
	}
	fmt.Printf("The size of the received buffer in bytes: %d, The number of Metric Reports processed: %d\n", len(tmp), reportCount)
	esIndex := res.Id
	fmt.Printf("Metric Report Name: %s\n", esIndex)
	for _, value := range res.MetricValues {
		docIndex := value.MetricID
		//fmt.Printf("MetricId: %s\n", docIndex)
		fmt.Println("\nDOC _id:", docIndex)
		//fmt.Println(value)
		docval, _ := json.Marshal(value)
		request := esapi.IndexRequest{Index: strings.ToLower(esIndex), Body: strings.NewReader(string(docval)), DocumentID: docIndex, Refresh: "true"}
		res, err := request.Do(context.Background(), es)
		if err != nil {
			fmt.Printf("IndexRequest ERROR: %s\n", err)
			continue
		}
		defer res.Body.Close()
		if res.IsError() {
			fmt.Printf("%s ERROR indexing document ID=%s\n", res.Status(), docIndex)
			fmt.Println("Error str: ", res.String())
		} else {
			// Deserialize the response into a map.
			var resMap map[string]interface{}
			if err := json.NewDecoder(res.Body).Decode(&resMap); err != nil {
				fmt.Printf("Error parsing the response body: %s\n", err)
			} else {
				fmt.Printf("\nIndexRequest() RESPONSE:\n")
				// Print the response status and indexed document version.
				fmt.Println("Status:", res.Status())
				fmt.Println("Result:", resMap["result"])
				fmt.Println("Version:", int(resMap["_version"].(float64)))
				fmt.Println("resMap:", resMap)
				//fmt.Println("\n")
			}
		}
	}
	fmt.Println("inserted into ealsticEngine")
}
func LoadTelemetryData(es *elasticsearch.Client) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	var outerloop int
	for {
		req, err := http.NewRequest("GET", "https://"+telemetryserverIP+"/redfish/v1/SSE?$filter=EventFormatType%20eq%20MetricReport", nil)
		if err != nil {
			fmt.Printf("error %s", err)
			return
		}
		req.SetBasicAuth("root", "calvin")
		req.Header.Add("Accept", "application/json")
		response, err := client.Do(req)
		if err != nil {
			fmt.Printf("error %s", err)
			return
		}
		fmt.Println(response.StatusCode)
		//body, _ := ioutil.ReadAll(response.Body)
		defer response.Body.Close()
		//bodyString := string(body)
		//fmt.Println(bodyString)
		var cnt int
		reportStart := false
		reportEnd := false
		reportChunks := 0
		outerloop++
		validHeader := 0

		byt := make([]byte, 8192*24, 8192*32)

		for {
			buf := make([]byte, 4096)

			n, err := response.Body.Read(buf)

			fmt.Printf("\n\nThe response size and response error %d, %s\n\n", n, err)

			if n == 0 {
				fmt.Printf("Got zero lenght when reading body - Don't break, continue for now %d, %s", n, err)
				break
			}

			reportStart = checkHeader(buf)
			if reportStart {
				validHeader++
			}

			fmt.Printf("The reportStart is %t", reportStart)
			fmt.Printf("The Name starts at index %d", bytes.Index(buf, []byte("Name")))
			fmt.Printf("The ReportSequence starts at index %d", bytes.Index(buf, []byte("ReportSequence")))
			fmt.Printf("The iDRACFirmwareVersion  starts at index %d", bytes.Index(buf, []byte("iDracFirmwareVersion")))
			var index1 int
			if reportStart == true {
				if validHeader > 1 {

					fmt.Printf("\n Detected a new header -- process it \n")
					if bytes.Contains(buf, []byte("id: ")) {
						index1 = bytes.Index(buf, []byte("id: "))
						fmt.Printf("The index of the id: is at: %d", index1)
					}
					fmt.Printf("***  stream START ***")
					// Check the last 20 bytes of the byt buffer
					bytSize := len(byt)
					fmt.Printf(string(byt[bytSize-20 : bytSize]))
					fmt.Printf("***  stream END  ***")

					// End the buffer and start a new one
					validHeader = 1
					byt = append(byt, buf[:(index1)]...)
					// Make a copy of the buffer and send it over to be processed.
					newMetricReport := byt
					fmt.Printf("Last chunk and new header %s", string(buf))
					fmt.Printf("New header detected: %s", string(newMetricReport))
					go InsertMetricReportData(es, index1, newMetricReport)
					// Update buffer to point to the next header
					if index1 > 2 {
						buf = buf[index1-2:]
						n = n - (index1 - 2)
					} else {
						buf = buf[index1:]
						n = n - index1
					}
					fmt.Printf("The new size %d: and new part buffer :%s", n, string(buf))
				}

				jsonOpenBraceAt := bytes.IndexByte(buf, byte('{'))

				// Move the data over to a temporary buffer
				byt = buf[jsonOpenBraceAt:n]
				reportChunks++
				if n > 15 {
					fmt.Printf("The init buf start content %s", string(byt[:10]))
					fmt.Printf("The init buf end content %s", string(buf[n-15:n]))
					fmt.Printf("The init buf Real end content %s", string(buf[n-5:n]))
				}
			} else {
				// Append to the buffer
				byt = append(byt, buf[:n]...)
				if n > 15 {
					fmt.Printf("The chunk buf content %s", string(buf[:10]))
					fmt.Printf("The chunk buf end content %s", string(buf[n-15:n]))
					fmt.Printf("The chunk buf Real end content %s", string(buf[n-5:n]))
				}
				reportChunks++
			}

			// Error check the length of the buffer and adjust TheEnd check accordingly.
			TheEnd := false
			if n > 10 {
				fmt.Printf(string(buf))
				TheEnd = (bytes.Contains(buf[n-10:n], []byte(".00")) && bytes.Contains(buf[n-10:n], []byte("}}}")))
			}

			//fmt.Printf(TheEnd, len(buf))

			if TheEnd == true {
				reportEnd = true
			}
			fmt.Printf("The reportEnd is %t", reportEnd)

			if reportEnd == true {

				// Move the buffer for post processing
				fmt.Printf("\nExtracted a Metric Report with %d chunks\n", reportChunks)
				// Make a copy of the buffer and send it over to be processed.
				metricReport := byt

				// Validate the buffer -- test
				bytSz := len(metricReport)
				fmt.Printf("The metric report size in bytes, %d", bytSz)
				fmt.Printf("The metric report buf content start %s", string(byt[:10]))
				fmt.Printf("The metric report buf content end %s", string(byt[bytSz-10:bytSz]))

				finBuf := string(metricReport)
				fmt.Printf("FinBuf: %s", finBuf)
				reportChunks = 0
				reportEnd = false
				reportStart = false
				validHeader = 0

				go InsertMetricReportData(es, index1, metricReport)

				time.Sleep(1 * time.Second)

				cnt++

				fmt.Printf("\nThe outerloop is: %d The inner loop count is: %d \n", outerloop, cnt)

			}
		}
	}
}
func GetData(es *elasticsearch.Client, reader *bufio.Scanner) {
	id := ReadText(reader, "Enter telemetry id")
	request := esapi.GetRequest{Index: "telemetry", DocumentID: id}
	response, _ := request.Do(context.Background(), es)
	var results map[string]interface{}
	json.NewDecoder(response.Body).Decode(&results)
	fmt.Println(results)
}
func Exit() {
	fmt.Println("Goodbye!")
	os.Exit(0)
}

func ReadText(reader *bufio.Scanner, prompt string) string {
	fmt.Print(prompt + ": ")
	reader.Scan()
	return reader.Text()
}

var cert, _ = ioutil.ReadFile("http_ca.crt")
var cfg = elasticsearch.Config{
	Addresses: []string{
		"https://localhost:9200",
	},
	Username: "elastic",
	Password: "DzO+KNQSLxgd79Bc3Z4t",
	CACert:   cert,
}

func main() {
	es, _ := elasticsearch.NewClient(cfg)
	//reader := bufio.NewScanner(os.Stdin)
	for {
		/*fmt.Println("0) Exit")
		fmt.Println("1) Load Telemetry Data")
		fmt.Println("2) Get Telemetry Data")
		option := ReadText(reader, "Enter option")
		if option == "0" {
			Exit()
		} else if option == "1" {
			LoadTelemetryData(es)
		} else if option == "2" {
			GetData(es, reader)
		} else {
			fmt.Println("Invalid option")
		}*/
		LoadTelemetryData(es)
	}
}
