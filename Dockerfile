FROM golang:1.13 as builder

RUN mkdir -p /flagger-appmesh-gateway/

WORKDIR /flagger-appmesh-gateway

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -a -o bin/flagger-appmesh-gateway cmd/flagger-appmesh-gateway/*

FROM alpine:3.10

RUN addgroup -S app \
    && adduser -S -g app app \
    && apk --no-cache add \
    curl openssl netcat-openbsd

WORKDIR /home/app

COPY --from=builder /flagger-appmesh-gateway/bin/flagger-appmesh-gateway .
RUN chown -R app:app ./

USER app

CMD ["./flagger-appmesh-gateway"]
