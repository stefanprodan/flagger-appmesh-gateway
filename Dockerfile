FROM golang:1.13 as builder

RUN mkdir -p /appmesh-gateway/

WORKDIR /appmesh-gateway

COPY . .

RUN go mod download

RUN go test -v -race ./...

RUN CGO_ENABLED=0 GOOS=linux go build -a -o bin/appmesh-gateway cmd/appmesh-gateway/*

FROM alpine:3.10

RUN addgroup -S app \
    && adduser -S -g app app \
    && apk --no-cache add \
    curl openssl netcat-openbsd

WORKDIR /home/app

COPY --from=builder /appmesh-gateway/bin/appmesh-gateway .
RUN chown -R app:app ./

USER app

CMD ["./appmesh-gateway"]
