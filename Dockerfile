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

COPY --from=build /build/bin/surl /

EXPOSE 8080

ENTRYPOINT [ "/surl" ]
CMD [ ":8080" ]
