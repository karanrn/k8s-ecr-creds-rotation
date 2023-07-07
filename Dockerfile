FROM golang:1.19 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY pkg/ ./pkg/

WORKDIR pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -o /ecr-creds-rotate


FROM ubuntu:20.04 AS release-stage

WORKDIR /opt

COPY --from=build-stage /ecr-creds-rotate /opt

ENTRYPOINT ["/opt/ecr-creds-rotate"]