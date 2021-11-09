go run cmd/dbdiscauth/dbdiscauth.go &
go run cmd/configui/configui.go &
go run cmd/redfishread/redfishread.go &
## Following applications are optional and 
## can be used for file based 
## discovery and authentication
## (look at sample config.ini for details)
go run cmd/dbdiscauth/dbdiscauth.go
#go run cmd/simpleauth/simpleauth.go &
#go run cmd/simpledisc/simpledisc.go

