# syntax=docker/dockerfile:1

# Stage 1: build
FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /cisco-api-guide ./cmd/cisco-api-guide/.

# Stage 2: minimal runtime image
FROM alpine:3

RUN apk add --no-cache ca-certificates

COPY --from=builder /cisco-api-guide /usr/local/bin/cisco-api-guide

EXPOSE 8080

ENTRYPOINT ["cisco-api-guide"]
CMD ["--http", "--addr", ":8080"]
