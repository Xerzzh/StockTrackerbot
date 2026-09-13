FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o stocktrackerbot .

FROM alpine:latest
RUN apk --no-cache add \
        ca-certificates \
        tzdata \
    && adduser -D -u 10001 app \
    && mkdir -p /data && chown app:app /data

COPY --from=builder /app/stocktrackerbot /home/app/stocktrackerbot
WORKDIR /data
USER app

CMD ["/home/app/stocktrackerbot"]
