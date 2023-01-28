FROM golang:1.18.1 as builder
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o schema-server .

FROM scratch
COPY --from=builder /build/schema-server /app/
WORKDIR /app
ENTRYPOINT [ "/app/schema-server" ]
