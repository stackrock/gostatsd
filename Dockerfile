ARG TARGET="amd64"
FROM ${TARGET}/golang:1.15 AS builder

WORKDIR /app
COPY . /app/
RUN go env && \
    GOARCH=${ARCH} go build -o gostatsd ./cmd/gostatsd
RUN ls -lah ./gostatsd

FROM debian:buster-slim AS runner
COPY --from=builder /app/gostatsd /usr/local/bin/gostatsd
RUN ls -lah /usr/local/bin

ENTRYPOINT ["/usr/local/bin/gostatsd"]
