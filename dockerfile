FROM golang:alpine as builder

WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY database ./database

# Build
RUN CGO_ENABLED=0 go build -ldflags "-w -s" -o /social-image-server .

# ? -------------------------
FROM alpine:latest

COPY --from=builder /social-image-server /

EXPOSE 8080

CMD ["/social-image-server"]