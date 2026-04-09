FROM golang:1.26-alpine3.23 AS builder

LABEL authors="vovamod"
RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v3 \
    go build -o timelapse \
    -ldflags="-s -w" \
    -trimpath \
    -tags netgo \
    ./cmd/timelapse-pb

# Runner
FROM alpine:3.21

LABEL org.opencontainers.image.source="https://github.com/vovamod/Timelapse-PixelBattle"

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata ffmpeg && \
    adduser -D -u 10001 timelapseuser

COPY --from=builder /app/timelapse /app/timelapse

RUN chown timelapseuser:timelapseuser /app/timelapse
USER timelapseuser

ENTRYPOINT ["./timelapse"]