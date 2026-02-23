# Stage 1: Build the binary
FROM golang:1.21-alpine AS builder

WORKDIR /build

# Cache dependency downloads separately from source compilation.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /demo-csi-plugin ./cmd/

# Stage 2: Minimal runtime image
# We use alpine (not scratch) because NodePublishVolume calls syscall.Mount,
# which requires the kernel mount helpers available in util-linux.
FROM alpine:3.19

RUN apk add --no-cache util-linux

COPY --from=builder /demo-csi-plugin /demo-csi-plugin

ENTRYPOINT ["/demo-csi-plugin"]
