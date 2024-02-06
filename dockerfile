FROM golang:1.21.4-alpine

RUN apk add --no-cache build-base dumb-init nodejs

COPY . /app

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go build -o bin .

ENTRYPOINT CGO_ENABLED=1 ["/usr/bin/dumb-init", "--", "/app/bin"]

EXPOSE 3000
