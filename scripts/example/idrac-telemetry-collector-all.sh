go run cmd/dbdiscauth/dbdiscauth.go &
go run cmd/configui/configui.go &
go run cmd/redfishread/redfishread.go &
## Following applications start the database ingestors
## comment out to remove unnecessary ingestors
go run cmd/prometheuspump/prometheuspump.go &
go run cmd/elkpump/elkpump-basic.go &
go run cmd/influxpump/influxpump.go &

## Following applications are optional and 
## can be used for file based 
## discovery and authentication
## (look at sample config.ini for details)
go run cmd/simpleauth/simpleauth.go &
go run cmd/simpledisc/simpledisc.go

