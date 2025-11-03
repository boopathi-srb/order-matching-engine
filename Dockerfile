# ---- Build Stage ----
FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o order-matching-engine main.go

# ---- Final Image ----
FROM gcr.io/distroless/static-debian11
WORKDIR /
COPY --from=builder /build/order-matching-engine /order-matching-engine
EXPOSE 8080
ENTRYPOINT ["/order-matching-engine"]
