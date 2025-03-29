FROM golang:1.23 as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o pollbot .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/pollbot .
CMD ["./pollbot"]

