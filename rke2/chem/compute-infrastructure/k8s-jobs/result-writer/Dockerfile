# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the result-writer
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o result-writer .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/result-writer .

# Create non-root user
RUN adduser -D -u 1000 appuser
USER appuser

EXPOSE 8081

CMD ["./result-writer"]
