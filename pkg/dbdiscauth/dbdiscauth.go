package dbdiscauth

import (
	"database/sql"
	"encoding/json"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
)

func GetInstancesFromDB(db *sql.DB) ([]auth.Service, error) {
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

func DeleteServiceFromDB(db *sql.DB, service auth.Service, authService *auth.AuthorizationService) error {
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

func AddServiceToDB(db *sql.DB, service auth.Service, authService *auth.AuthorizationService) error {
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
