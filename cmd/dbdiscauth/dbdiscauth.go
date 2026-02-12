// Licensed to You under the Apache License, Version 2.0.
// This is responsible for initializing an empty instance of a mysql database with the settings for the entire pipeline

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/wire"
)

var configStrings = map[string]string{
	//default settings
	"mbhost":        "activemq",                    //to be provided by user
	"mbport":        "61613",                       //to be provided by user
	"mysqluser":     "",                            //to be provided by user
	"mysqlpwd":      "",                            //to be provieed by user
	"mysqlHost":     "localhost",                   //to be provided by user
	"mysqlHostPort": "3306",                        //to be provided by user
	"mysqlDBName":   "telemetrysource_services_db", //to be provided by user
}

func getHECInstancesFromDB(db *sql.DB) ([]auth.SplunkConfig, error) {
	results, err := db.Query("SELECT url, `key`, `index` FROM HttpEventCollector")
	if err != nil {
		return nil, err
	}

	var ret []auth.SplunkConfig
	for results.Next() {
		var value auth.SplunkConfig
		var tmp string
		err = results.Scan(&value.Url, &value.Key, &value.Index, &tmp)
		if err != nil {
			return nil, err
		}
		ret = append(ret, value)
	}
	return ret, nil
}

func getInstancesFromDB(db *sql.DB) ([]auth.Service, error) {
	results, err := db.Query("SELECT serviceType, ip, authType, auth FROM services")
	if err != nil {
		return nil, err
	}

	var ret []auth.Service
	for results.Next() {
		var value auth.Service
		var tmp string
		err = results.Scan(&value.ServiceType, &value.Ip, &value.AuthType, &tmp)
		if err != nil {
			return nil, err
		}
		err := json.Unmarshal([]byte(tmp), &value.Auth)
		if err != nil {
			return nil, err
		}
		ret = append(ret, value)
	}
	return ret, nil
}

func deleteServiceFromDB(db *sql.DB, service auth.Service, authService *auth.AuthorizationService) error {
	stmt, err := db.Prepare("DELETE FROM services WHERE ip = ?")
	if err != nil {
		return err
	}
	_, err = stmt.Exec(service.Ip)
	if err != nil {
		return err
	}
	return nil
}

func addServiceToDB(db *sql.DB, service auth.Service, authService *auth.AuthorizationService) error {
	stmt, err := db.Prepare("INSERT INTO services(serviceType, ip, authType, auth) VALUES(?, ?, ?, ?)")
	if err != nil {
		return err
	}
	jsonStr, err := json.Marshal(service.Auth)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(service.ServiceType, service.Ip, service.AuthType, string(jsonStr))
	if err != nil {
		return err
	}
	_ = authService.BroadcastService(service)
	return nil
}

// splunk configuration are getting added in the database
// TO DO: Update it in the db
func splunkAddHECToDB(db *sql.DB, splunkConfig auth.SplunkConfig, authService *auth.AuthorizationService) error {
	stmt, err := db.Prepare("INSERT INTO HttpEventCollector(url, `key`, `index`) VALUES(?, ?, ?)")
	if err != nil {
		return err
	}
	_, err = stmt.Exec(splunkConfig.Url, splunkConfig.Key, splunkConfig.Index)
	if err != nil {
		return err
	}
	// Broadcast the config using envelope format
	env, err := wire.NewEnvelope("splunkconfig", splunkConfig)
	if err != nil {
		return err
	}
	return authService.SendEnvelope(auth.EventQueue, env)
}

func getEnvSettings() {
	mbHost := os.Getenv("MESSAGEBUS_HOST")
	if len(mbHost) > 0 {
		configStrings["mbhost"] = mbHost
	}
	mbPort := os.Getenv("MESSAGEBUS_PORT")
	if len(mbPort) > 0 {
		configStrings["mbport"] = mbPort
	}
	username := os.Getenv("MYSQL_USER")
	if len(username) > 0 {
		configStrings["mysqluser"] = username
	}
	pwd := os.Getenv("MYSQL_PASSWORD")
	if len(pwd) > 0 {
		configStrings["mysqlpwd"] = pwd
	}
	host := os.Getenv("MYSQL_HOST")
	if len(host) > 0 {
		configStrings["mysqlHost"] = host
	}
	hostport := os.Getenv("MYSQL_HOST_PORT")
	if len(host) > 0 {
		configStrings["mysqlHostPort"] = hostport
	}
	dbname := os.Getenv("MYSQL_DATABASE")
	if len(dbname) > 0 {
		configStrings["mysqlDBName"] = dbname
	}
}

func initMySQLDatabase() (*sql.DB, error) {
	var db *sql.DB
	var err error

	// Connect to the postgresql database
	connStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		configStrings["mysqluser"],
		configStrings["mysqlpwd"],
		configStrings["mysqlHost"],
		configStrings["mysqlHostPort"],
		configStrings["mysqlDBName"])

	// Debug log: dont print out sensitive info.
	// DONT LOG PASSWORDS! - instead we log "X"s the same length as the password.
	log.Printf("%s:%s@tcp(%s:%s)/%s",
		configStrings["mysqluser"],
		strings.Repeat("X", len(configStrings["mysqlpwd"])),
		configStrings["mysqlHost"],
		configStrings["mysqlHostPort"],
		configStrings["mysqlDBName"])

	for {
		db, err = sql.Open("mysql", connStr)
		if err != nil {
			log.Print("Could not connect to mysql database: ", err)
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}

	for {
		_, err := db.Query("CREATE TABLE IF NOT EXISTS services(ip VARCHAR(255) PRIMARY KEY, serviceType INT, authType INT, auth VARCHAR(4096));")
		if err != nil {
			log.Print("Could not create DB Table: ", err)
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}

	for {
		_, err = db.Query("CREATE TABLE IF NOT EXISTS HttpEventCollector(url VARCHAR(255), `key` VARCHAR(4096), `index` VARCHAR(4096));")
		if err != nil {
			log.Print("Could not create DB Table: ", err)
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}

	return db, err
}

func main() {
	//Gather configuration from environment variables
	getEnvSettings()

	//Initialize messagebus
	var authorizationService *auth.AuthorizationService
	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			authorizationService = auth.NewAuthorizationService(mb)
			defer mb.Close()
			break
		}
	}

	//Initialize mysql db instance which stores service authorizations
	db, err := initMySQLDatabase()
	if err != nil {
		log.Print("Failed to initalize db: ", err)
	} else {
		defer db.Close()
	}

	//Fetch and publish configured services in the database
	authServices, err := getInstancesFromDB(db)
	if err != nil {
		log.Print("Failed to get db entries: ", err)
	} else {
		for _, element := range authServices {
			go authorizationService.BroadcastService(element) //nolint: errcheck
		}
	}

	//Process commands using the new envelope format
	envelopes := make(chan wire.Envelope)
	go authorizationService.ReceiveEnvelopes(envelopes) //nolint: errcheck
	for {
		env := <-envelopes
		log.Printf("Received command in dbdiscauth: %s", env.Type)
		switch env.Type {
		case auth.RESEND:
			authServices, err := getInstancesFromDB(db)
			if err != nil {
				log.Print("Failed to get db entries: ", err)
				break
			}
			for _, element := range authServices {
				go authorizationService.BroadcastService(element) //nolint: errcheck
			}
		case auth.ADDSERVICE:
			var service auth.Service
			if err := wire.DecodePayload(env.Payload, &service); err != nil {
				log.Print("Failed to decode service payload: ", err)
				break
			}
			err = addServiceToDB(db, service, authorizationService)
			if err != nil {
				log.Print("Addservice,Failed to write db entries: ", err)
			}
		case auth.DELETESERVICE:
			var service auth.Service
			if err := wire.DecodePayload(env.Payload, &service); err != nil {
				log.Print("Failed to decode service payload: ", err)
				break
			}
			err = deleteServiceFromDB(db, service, authorizationService)
			if err != nil {
				log.Print("Deleteservice Failed to delete db entries: ", err)
			}
		case auth.SPLUNKADDHEC:
			var splunkConfig auth.SplunkConfig
			if err := wire.DecodePayload(env.Payload, &splunkConfig); err != nil {
				log.Print("Failed to decode splunk config payload: ", err)
				break
			}
			err = splunkAddHECToDB(db, splunkConfig, authorizationService)
			if err != nil {
				log.Print("AddHec Failed to write db entries: ", err)
			}
		case auth.GETHECCONFIG:
			HecConfig, err := getHECInstancesFromDB(db)
			if err != nil {
				log.Print("Failed to get db entries: ", err)
				break
			}
			log.Print(HecConfig)
		case auth.TERMINATE:
			os.Exit(0)
		}
	}
}
