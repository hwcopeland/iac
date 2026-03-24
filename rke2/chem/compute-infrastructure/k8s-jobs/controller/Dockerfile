# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY controller/go.mod controller/go.sum ./
RUN go mod download

# Copy source code
COPY controller/ .

# Build the controller
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o controller .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/controller .

# Create non-root user
RUN adduser -D -u 1000 appuser
USER appuser

EXPOSE 8080

CMD ["./controller"]
