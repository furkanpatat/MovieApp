FROM golang:1.27-alpine AS build
WORKDIR /src
COPY pkg ./pkg
COPY services/interaction ./services/interaction
RUN cd services/interaction && CGO_ENABLED=0 go build -o /out/interaction ./cmd/interaction

FROM alpine:3.20
COPY --from=build /out/interaction /usr/local/bin/interaction
ENTRYPOINT ["/usr/local/bin/interaction"]
