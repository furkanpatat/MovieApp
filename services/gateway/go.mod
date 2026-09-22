module github.com/furkanpatat/movieapp/services/gateway

go 1.27.1

require (
	github.com/alicebob/miniredis/v2 v2.39.0
	github.com/furkanpatat/movieapp/pkg v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/redis/go-redis/v9 v9.22.0
)

require (
	github.com/caarlos0/env/v11 v11.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/furkanpatat/movieapp/pkg => ../../pkg
