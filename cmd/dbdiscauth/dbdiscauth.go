// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/ini.v1"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/auth"
	//"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/amqp"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

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
		log.Print(value)
		ret = append(ret, value)
	}
	return ret, nil
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
	authService.SendService(service)
	return nil
}

func main() {
	db, err := sql.Open("mysql", "gofish:gofish@tcp(127.0.0.1:3306)/gofish")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	configName := flag.String("config", "config.ini", "The configuration ini file")

	flag.Parse()

	config, err := ini.Load(*configName)
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	//amqpHost := config.Section("General").Key("AmpqHost").MustString("0.0.0.0")
	//amqpPort := config.Section("General").Key("AmqpPort").MustInt(5672)
	stompHost := config.Section("General").Key("StompHost").MustString("0.0.0.0")
	stompPort := config.Section("General").Key("StompPort").MustInt(61613)

	//mb, err := amqp.NewAmqpMessageBus(amqpHost, amqpPort)
	mb, err := stomp.NewStompMessageBus(stompHost, stompPort)
	if err != nil {
		log.Fatal("Could not connect to message bus: ", err)
	}
	defer mb.Close()

	authorizationService := new(auth.AuthorizationService)
	authorizationService.Bus = mb
	commands := make(chan *auth.Command)

	_, err = db.Query("CREATE TABLE IF NOT EXISTS services(ip VARCHAR(255) PRIMARY KEY, serviceType INT, authType INT, auth VARCHAR(4096));")
	if err != nil {
		log.Fatal("Unable to create DB Table! ", err)
	}

	authServices, err := getInstancesFromDB(db)
	if err != nil {
		log.Print("Failed to get db entries: ", err)
	} else {
		for _, element := range authServices {
			go authorizationService.SendService(element)
		}
	}
	go authorizationService.RecieveCommand(commands)
	for {
		command := <-commands
		log.Printf("Recieved command: %s", command.Command)
		switch command.Command {
		case auth.RESEND:
			authServices, err := getInstancesFromDB(db)
			if err != nil {
				log.Print("Failed to get db entries: ", err)
				break
			}
			for _, element := range authServices {
				go authorizationService.SendService(element)
			}
		case auth.ADDSERVICE:
			err = addServiceToDB(db, command.Service, authorizationService)
			if err != nil {
				log.Print("Failed to write db entries: ", err)
			}
		case auth.TERMINATE:
			os.Exit(0)
		}
	}
}
