FROM golang:1.27-alpine AS build
WORKDIR /src
COPY pkg ./pkg
COPY services/catalog ./services/catalog
RUN cd services/catalog && CGO_ENABLED=0 go build -o /out/catalog ./cmd/catalog

FROM alpine:3.20
COPY --from=build /out/catalog /usr/local/bin/catalog
ENTRYPOINT ["/usr/local/bin/catalog"]
