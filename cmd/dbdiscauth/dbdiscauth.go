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
	_ = authService.SendService(service)
	return nil
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

	return db, err
}

func main() {
	//Gather configuration from environment variables
	getEnvSettings()

	//Setu authorization service
	authorizationService := new(auth.AuthorizationService)

	//Initialize messagebus
	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			authorizationService.Bus = mb
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
			go authorizationService.SendService(element) //nolint: errcheck
		}
	}

	//Process ADDSERVICE and RESEND requests for authorization services
	commands := make(chan *auth.Command)
	go authorizationService.ReceiveCommand(commands) //nolint: errcheck
	for {
		command := <-commands
		log.Printf("Received command in dbdiscauth: %s", command.Command)
		switch command.Command {
		case auth.RESEND:
			authServices, err := getInstancesFromDB(db)
			if err != nil {
				log.Print("Failed to get db entries: ", err)
				break
			}
			for _, element := range authServices {
				go authorizationService.SendService(element) //nolint: errcheck
			}
		case auth.ADDSERVICE:
			err = addServiceToDB(db, command.Service, authorizationService)
			if err != nil {
				log.Print("Addservice,Failed to write db entries: ", err)
			}
		case auth.DELETESERVICE:
			err = deleteServiceFromDB(db, command.Service, authorizationService)
			if err != nil {
				log.Print("Deleteservice Failed to delete db entries: ", err)
			}
		case auth.TERMINATE:
			os.Exit(0)
		}
	}
}
