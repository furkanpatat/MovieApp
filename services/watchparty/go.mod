module github.com/furkanpatat/movieapp/services/watchparty

go 1.27.1

require (
	github.com/furkanpatat/movieapp/pkg v0.0.0
	github.com/gorilla/websocket v1.5.3
	golang.org/x/time v0.16.0
)

require (
	github.com/caarlos0/env/v11 v11.4.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
)

replace github.com/furkanpatat/movieapp/pkg => ../../pkg
