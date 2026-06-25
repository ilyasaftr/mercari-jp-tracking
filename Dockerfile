FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/tracker ./cmd/tracker/

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/tracker /usr/local/bin/tracker

ENTRYPOINT ["tracker"]
