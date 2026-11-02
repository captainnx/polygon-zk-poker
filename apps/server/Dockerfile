FROM golang:1.25.0-alpine AS build
WORKDIR /src
COPY go.work ./
COPY apps/server/go.* ./apps/server/
WORKDIR /src/apps/server
RUN go mod download
COPY apps/server/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /server ./cmd/server

FROM alpine:3.22
RUN addgroup -S poker && adduser -S -G poker poker && mkdir -p /var/lib/poker && chown poker:poker /var/lib/poker
USER poker
COPY --from=build /server /usr/local/bin/server
ENV LISTEN_ADDR=0.0.0.0:8080
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/server"]
