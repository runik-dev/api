FROM golang:1.21.4-alpine

RUN apk add --no-cache build-base dumb-init nodejs

COPY . /app

WORKDIR /app

ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=1
ENV CC="zig cc -target x86_64-linux-musl"
ENV CXX="zig c++ -target x86_64-linux-musl"
COPY go.mod go.sum ./
RUN go mod download
RUN go build -o bin .

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/app/bin"]

EXPOSE 3000
