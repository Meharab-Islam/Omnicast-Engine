# ==============================================================================
# Stage 1: Build the OmniCast Engine binary
# ==============================================================================
FROM golang:alpine AS builder


WORKDIR /app

# Install git, ca-certificates, and build tools
RUN apk add --no-cache git ca-certificates tzdata

# Cache go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile optimized static binary (-w -s strips debug information and symbol table for minimal size)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s -extldflags '-static'" -o omnicast-engine ./cmd/main.go

# ==============================================================================
# Stage 2: Ultra-Minimal Production Runner Image (< 30MB)
# ==============================================================================
FROM alpine:3.20 AS runner

WORKDIR /app

# Install essential runtime dependencies: CA certificates for HTTPS/WSS and tzdata for timestamps
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S omnicast && adduser -S omnicast -G omnicast \
    && mkdir -p /app/data /app/config && chown -R omnicast:omnicast /app

# Copy compiled Go binary and public frontend assets only (no source code)
COPY --from=builder /app/omnicast-engine /app/omnicast-engine
COPY --from=builder /app/public /app/public

# Run as non-root user for security
USER omnicast:omnicast

# Expose internal signaling port and WebRTC UDP media port range
EXPOSE 8080
EXPOSE 50000-50050/udp

# Set execution entrypoint
ENTRYPOINT ["/app/omnicast-engine"]
