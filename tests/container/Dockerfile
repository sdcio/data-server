FROM golang:1.19.5 as builder
COPY go.mod go.sum /build/
WORKDIR /build
RUN --mount=type=cache,target=/root/.cache/go-build \
    go mod download
ADD . /build
RUN --mount=type=cache,target=/root/.cache/go-build \
    make build


FROM alpine
COPY --from=builder /build/bin/* /app/bin/
COPY ./tests /app/tests/
WORKDIR /app
RUN --mount=type=cache,target=/root/.cache \
    apk add py3-pip ;\
    pip3 install -r tests/robot/requirements.txt