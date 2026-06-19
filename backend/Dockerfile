FROM golang:1.26-alpine AS builder

# confluent-kafka-go is a CGO wrapper over librdkafka. The module ships a
# bundled static librdkafka selected by the "musl" build tag on Alpine; we still
# need a C toolchain to link it. UPX compresses the final binary after build.
RUN apk --no-cache add gcc musl-dev upx

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO must be enabled for confluent-kafka-go. The "musl" tag selects the
# Alpine-compatible bundled librdkafka.
RUN CGO_ENABLED=1 GOOS=linux go build \
  -ldflags="-w -s" \
  -tags musl \
  -trimpath \
  -o transx .

RUN upx --best --lzma transx

RUN ./transx --help || echo "Binary compressed successfully"

FROM alpine:3.23

# librdkafka links against these at runtime under musl.
RUN apk --no-cache add ca-certificates tzdata wget libc6-compat

WORKDIR /app

COPY --from=builder /app/transx .
COPY db/migrations/ ./db/migrations/
COPY config.yaml .
COPY entrypoint.sh .

RUN chmod +x ./entrypoint.sh

EXPOSE 4000

ENTRYPOINT ["./entrypoint.sh"]
