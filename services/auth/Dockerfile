FROM golang:1.27-alpine AS build
WORKDIR /src
COPY pkg ./pkg
COPY services/auth ./services/auth
RUN cd services/auth && CGO_ENABLED=0 go build -o /out/auth ./cmd/auth

FROM alpine:3.20
COPY --from=build /out/auth /usr/local/bin/auth
ENTRYPOINT ["/usr/local/bin/auth"]
