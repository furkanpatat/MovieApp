FROM golang:1.27-alpine AS build
WORKDIR /src
COPY pkg ./pkg
COPY services/gateway ./services/gateway
RUN cd services/gateway && CGO_ENABLED=0 go build -o /out/gateway ./cmd/gateway

FROM alpine:3.20
COPY --from=build /out/gateway /usr/local/bin/gateway
ENTRYPOINT ["/usr/local/bin/gateway"]
