# ==============================================================================
# Stage 1: Build the Go Media Server binary
# ==============================================================================
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git and build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go module definitions and pre-download dependencies for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code into the container
COPY . .

# Compile optimized, statically linked binary for Linux
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o media-server ./cmd/main.go

# ==============================================================================
# Stage 2: Minimal Runtime Container
# ==============================================================================
FROM alpine:latest AS runner

WORKDIR /app

# Install CA root certificates (for secure TLS outbound calls) and tzdata
RUN apk add --no-cache ca-certificates tzdata

# Copy compiled binary and static assets from builder stage
COPY --from=builder /app/media-server .
COPY --from=builder /app/public ./public

# Expose HTTP / WebSocket signaling port and WebRTC UDP media port range
EXPOSE 8080
EXPOSE 50000-50050/udp

# Set binary execution entrypoint
CMD ["./media-server"]
