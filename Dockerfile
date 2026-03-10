FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN CGO_ENABLED=0 go build \
  -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
  -o /releasewave ./cmd/releasewave

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    adduser -D -h /home/releasewave releasewave

COPY --from=builder /releasewave /usr/local/bin/releasewave

USER releasewave
WORKDIR /home/releasewave

ENTRYPOINT ["releasewave"]
CMD ["serve", "--transport=sse"]

EXPOSE 7891
