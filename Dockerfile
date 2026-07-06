# Stage 1: Build Vue frontend (on build host platform — Node.js/V8 crashes under QEMU)
ARG BUILDPLATFORM=linux/amd64
FROM --platform=$BUILDPLATFORM node:22-slim AS frontend
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary with CGO (on target platform via QEMU)
FROM golang:1.26-bookworm AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    librtlsdr-dev libusb-1.0-0-dev pkg-config gcc ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /web/dist ./web/dist
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /go-rtl-sdr-mon .

# Stage 3: Export only the binary (for cross-compilation extraction via Makefile)
FROM scratch AS export
COPY --from=builder /go-rtl-sdr-mon /

# Stage 4: Runtime image (for Docker deployment)
FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
    librtlsdr0 libusb-1.0-0 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /go-rtl-sdr-mon /usr/local/bin/go-rtl-sdr-mon
EXPOSE 8080
ENTRYPOINT ["go-rtl-sdr-mon"]
