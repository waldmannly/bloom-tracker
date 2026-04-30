# ── Build stage ──────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bloom .

# ── Runtime stage ────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/bloom .

# Data volume for SQLite database and backups
VOLUME /data

ENV DB_PATH=/data/period_tracker.db
ENV PORT=8080

EXPOSE 8080

ENTRYPOINT ["./bloom"]
