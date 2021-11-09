# Barely adapted from: https://docs.docker.com/language/golang/build-images/

##
## Build
##
FROM golang:1.17-buster AS build

WORKDIR /app
COPY go.mod ./
RUN go mod download

COPY *.go ./

RUN go build -o /mq-go


##
## Deploy
##
FROM gcr.io/distroless/base-debian10

WORKDIR /

COPY --from=build /mq-go /mq-go

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/mq-go"]
