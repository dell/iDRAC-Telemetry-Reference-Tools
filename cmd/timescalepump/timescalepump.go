// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"time"

	"gopkg.in/ini.v1"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

var configStrings = map[string]string{
	"timescaleDBHost": "postgres:postgres@localhost",
	"timescaleDBHostPort": "5432",
	"timescaleDBName": "poweredge_telemetry_metrics",
}

///////////////////////////////////////////////
/* PostgresSQL Table schema
timeseries_metrics 
	ID 					TEXT NOT NULL,
	Context 			TEXT NOT NULL,
	Label 				TEXT NOT NULL,
	Value 				TEXT,
	System 				TEXT,
	time	 			TIMESTAMPTZ NOT NULL
*/
///////////////////////////////////////////////

func handleGroups(groupsChan chan *databus.DataGroup, dbpool *pgxpool.Pool, ctx context.Context) {

	queryInsertTimeseriesData := `INSERT INTO timeseries_metrics 
					(ID, Context, Label, Value, System, time) 
					VALUES ($1, $2, $3, $4, $5, $6);`
 
	for {
		group := <-groupsChan
		batch := &pgx.Batch{}
		numInserts := 0
		for _, value := range group.Values {
			log.Print("value: ", value)

			//load insert statements into batch queue
			batch.Queue(queryInsertTimeseriesData, 
				value.ID, value.Context, value.Label,
				value.Value, value.System, value.Timestamp)
			numInserts++	
		}

		//send batch to connection pool
		br := dbpool.SendBatch(ctx, batch)
		//execute statements in batch queue
		for i := 0; i < numInserts; i++ {
			_, err := br.Exec()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to execute statement in batch queue %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Println("Successfully batch inserted data")

		err := br.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to closer batch %v\n", err)
			os.Exit(1)
		}
	}
}

func initalizePQLWithTimescale(ctx context.Context) (*pgxpool.Pool, error) {
	// Connect to the postgresql database
	connStr := fmt.Sprintf("postgresql://%s:%s/%s", 
					configStrings["timescaleDBHost"],
					configStrings["timescaleDBHostPort"],
					configStrings["timescaleDBName"])
	dbpool, err := pgxpool.Connect(ctx, connStr)
	if err != nil {
		return dbpool, err
	}

	/********************************************/
	// setup timescaledb extension on postgresql
	queryAddTimescaleDBExtn := `CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;`
	_, err = dbpool.Exec(ctx, queryAddTimescaleDBExtn)
	if err != nil {
		return dbpool, err
	}

	//Setup hypertable
	queryCreateHypertable := `CREATE TABLE IF NOT EXISTS timeseries_metrics (
		ID 					TEXT NOT NULL,
		Context 			TEXT NOT NULL,
		Label 				TEXT NOT NULL,
		Value 				TEXT,
		System 				TEXT,
		time	 			TIMESTAMPTZ NOT NULL
		);
		SELECT create_hypertable('timeseries_metrics', 'time', if_not_exists => TRUE);`
	_, err = dbpool.Exec(ctx, queryCreateHypertable)
	if err != nil {
		return dbpool, err
	}

	return dbpool, err
}

func main() {

	configName := flag.String("config", "config.ini", "The configuration ini file")

	flag.Parse()

	config, err := ini.Load(*configName)
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	stompHost := config.Section("General").Key("StompHost").MustString("0.0.0.0")
	stompPort := config.Section("General").Key("StompPort").MustInt(61613)

	dbClient := new(databus.DataBusClient)
    for {
        mb, err := stomp.NewStompMessageBus(stompHost, stompPort)
        if err != nil {
            log.Printf("Could not connect to message bus: ", err)
            time.Sleep(5 * time.Second)
        } else {
            dbClient.Bus = mb
            defer mb.Close()
            break
        }
    }

	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/tscalestack")
	dbClient.Get("/tscalestack")
	go dbClient.GetGroup(groupsIn, "/tscalestack")

	//Initialize timescale client
	ctx := context.Background()
	dbpool, err := initalizePQLWithTimescale(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to PQL database: %v\n", err)
		os.Exit(1)
	}
	defer dbpool.Close()
 

	go handleGroups(groupsIn, dbpool, ctx)

	err = http.ListenAndServe(":5555", nil)
	if err != nil {
		log.Printf("Failed to start webserver %v", err)
	}

}
