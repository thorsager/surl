FROM golang:1-alpine AS build
RUN apk add --update --no-cache make git
RUN mkdir /build
WORKDIR /build
COPY go.mod go.sum /build/
RUN go list -m all
RUN go mod download

ADD . /build
RUN CGO_ENABLED=0 GOOS=linux make

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
