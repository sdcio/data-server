FROM golang:1.21.5 as builder

RUN apt-get update && apt-get install -y ca-certificates git-core ssh
RUN mkdir -p -m 0700 /root/.ssh && ssh-keyscan github.com >> /root/.ssh/known_hosts
RUN git config --global url.ssh://git@github.com/.insteadOf https://github.com/

COPY go.mod go.sum /build/
WORKDIR /build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=ssh \
    go mod download
ADD . /build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=ssh \
    make build


FROM alpine
COPY --from=builder /build/bin/* /app/bin/
COPY ./tests /app/tests/
WORKDIR /app
RUN --mount=type=cache,target=/root/.cache \
    apk add py3-pip ;\
    pip3 install -r tests/robot/requirements.txt
