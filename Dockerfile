FROM golang:1-alpine AS build
RUN apk add --update --no-cache make git
WORKDIR /build

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./

# Download dependencies with cache mount for go modules
# This layer only rebuilds when go.mod or go.sum changes
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code (this invalidates cache only when code changes)
COPY . .

# Build with cache mount for go build cache
# This dramatically speeds up rebuilds by caching compiled packages
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux make

FROM alpine:3
LABEL org.opencontainers.image.source=https://github.com/thorsager/surl
WORKDIR /

# Create non-root user
RUN addgroup -g 10001 -S appgroup && \
    adduser -u 10001 -S appuser -G appgroup

COPY --from=build --chown=appuser:appgroup /build/bin/surl /

# Switch to non-root user
USER appuser

EXPOSE 8080

ENTRYPOINT [ "/surl" ]
CMD [ ":8080" ]
