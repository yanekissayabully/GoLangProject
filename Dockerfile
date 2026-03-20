# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go.mod first (go.sum may not exist on host without Go installed)
COPY go.mod ./

# Download dependencies - this generates go.sum inside the container
RUN go mod download

# Copy source code
COPY . .

# Ensure modules are tidy (handles any new dependencies)
RUN go mod tidy

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install CA certificates for HTTPS calls
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/bin/api /app/api

# Copy migrations
COPY --from=builder /app/migrations /app/migrations

# Expose port
EXPOSE 8080

# Run the binary
CMD ["/app/api"]
