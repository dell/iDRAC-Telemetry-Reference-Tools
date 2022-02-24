# WARNING: Make sure the last command that is run runs in the foreground
# WARNING: That is to say every command except the last command should end
# with an ampersand (&)
go run cmd/dbdiscauth/dbdiscauth.go &
go run cmd/configui/configui.go &
go run cmd/redfishread/redfishread.go

# The Following applications are optional and
# can be used for file based
# discovery and authentication
# (look at sample config.ini for details)
# go run cmd/simpleauth/simpleauth.go &
# go run cmd/simpledisc/simpledisc.go
