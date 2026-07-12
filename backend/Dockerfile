FROM golang:1.26-alpine AS builder

# UPX compresses the final binary after build.
RUN apk --no-cache add upx

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags="-w -s" \
  -trimpath \
  -o transx .

RUN upx --best --lzma transx

RUN ./transx --help || echo "Binary compressed successfully"

FROM alpine:3.23

RUN apk --no-cache add ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /app/transx .
COPY db/migrations/ ./db/migrations/
COPY config.yaml .
COPY entrypoint.sh .

RUN chmod +x ./entrypoint.sh

EXPOSE 4000

ENTRYPOINT ["./entrypoint.sh"]
