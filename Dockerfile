FROM golang:1.22 AS build-stage

WORKDIR /app

COPY go.mod main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /composer-packagist


FROM gcr.io/distroless/base-debian12 AS release-stage

WORKDIR /

COPY --from=build-stage /composer-packagist /composer-packagist
COPY --chown=nonroot:nonroot data /data
EXPOSE 3000
USER nonroot:nonroot

ENTRYPOINT ["/composer-packagist"]
