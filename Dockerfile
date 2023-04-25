FROM golang:1.19.5 as builder
COPY go.mod go.sum /build/
WORKDIR /build
RUN --mount=type=cache,target=/root/.cache/go-build \
    go mod download
ADD . /build
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o schema-server .

FROM scratch
COPY --from=builder /build/schema-server /app/
WORKDIR /app
ENTRYPOINT [ "/app/schema-server" ]
