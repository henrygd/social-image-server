FROM --platform=$BUILDPLATFORM golang:alpine as builder

WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY internal ./internal

# Build
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags "-w -s" -o /social-image-server .

# ? -------------------------
FROM chromedp/headless-shell:latest

# add ca-certificates
RUN export DEBIAN_FRONTEND=noninteractive \
  && apt-get update \
  && apt-get install -y --no-install-recommends \
  ca-certificates \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/

COPY --from=builder /social-image-server /

EXPOSE 8080

ENTRYPOINT ["/social-image-server"]
