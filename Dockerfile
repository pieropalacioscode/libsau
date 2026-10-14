FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ladmincabo-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ladmincabo-import ./cmd/import

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/ladmincabo-api .
COPY --from=builder /out/ladmincabo-import .
COPY --from=builder /src/static ./static
COPY --from=builder /src/templates ./templates
USER app
ENV APP_PORT=8080 TZ=America/Lima
EXPOSE 8080
CMD ["./ladmincabo-api"]