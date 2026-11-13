# Stage 1: Build the Go binary using only standard library
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy Go source files
COPY go.mod ./
COPY main.go ./

# Build a statically linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o national-parks .

# Stage 2: Final minimal runtime container
FROM alpine:3.19

# Install ca-certificates so the app can verify Wikipedia's TLS certificate
RUN apk --no-cache add ca-certificates tzdata

# Create a non-privileged user for security
RUN adduser -D -g '' appuser

WORKDIR /app
COPY --from=builder /app/national-parks .

USER appuser

# GCP Cloud Run injects $PORT (defaults to 8080)
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/national-parks"]
