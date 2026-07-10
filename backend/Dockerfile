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
COPY --from=builder /src/data /app/data
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/backend"]
