# Build stage
FROM zot.hwcopeland.net/library/golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o khemeia-controller .

# Final stage
FROM zot.hwcopeland.net/library/alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/khemeia-controller .
# Plugins are mounted via ConfigMap or copied at build time
RUN mkdir -p /app/plugins
RUN adduser -D -u 1000 appuser
USER appuser
EXPOSE 8080
CMD ["./khemeia-controller"]
