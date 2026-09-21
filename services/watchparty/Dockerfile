FROM golang:1.27-alpine AS build
WORKDIR /src
COPY pkg ./pkg
COPY services/watchparty ./services/watchparty
RUN cd services/watchparty && CGO_ENABLED=0 go build -o /out/watchparty ./cmd/watchparty

FROM alpine:3.20
COPY --from=build /out/watchparty /usr/local/bin/watchparty
ENTRYPOINT ["/usr/local/bin/watchparty"]
