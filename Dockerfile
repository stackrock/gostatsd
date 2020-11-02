ARG TARGET="amd64"
FROM ${TARGET}/golang:1.15-alpine AS builder

WORKDIR /app
COPY . /app/
RUN go env && \
    GOARCH=${ARCH} go build -o gostatsd ./cmd/gostatsd
RUN ls -lah ./gostatsd

FROM ${TARGET}/alpine AS runner
COPY --from=builder /app/gostatsd /usr/local/bin/gostatsd
RUN ls -lah /usr/local/bin && /usr/local/bin/gostatsd --version

ENTRYPOINT ["/usr/local/bin/gostatsd"]
