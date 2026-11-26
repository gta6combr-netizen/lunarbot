FROM golang:1.26-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o lunarbot ./cmd/nyxbot

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    ffmpeg \
    python3 \
    python3-pip \
    && pip3 install --no-cache-dir --break-system-packages yt-dlp edge-tts \
    && rm -rf /var/lib/apt/lists/*

ENV TZ=America/Sao_Paulo
WORKDIR /app
COPY --from=builder /app/lunarbot /app/lunarbot
COPY config ./config
COPY assets ./assets
COPY public ./public
COPY database ./database
RUN mkdir -p /app/temp

EXPOSE 8080
CMD ["./lunarbot"]
