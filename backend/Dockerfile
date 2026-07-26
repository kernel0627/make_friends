FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/backend ./cmd/server

FROM alpine:3.21
WORKDIR /app
RUN adduser -D -u 10001 appuser
COPY --from=builder /out/backend /app/backend
# The SQLite database is runtime state, not part of the image. Baking it in
# shipped every user row and password hash to anyone who could pull the image.
# docker-compose mounts ./data here; this just makes sure the mount point
# exists and is writable by the unprivileged user.
RUN mkdir -p /app/data && chown appuser:appuser /app/data
VOLUME ["/app/data"]
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/backend"]
