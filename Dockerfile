FROM golang:1.21.4 as builder

RUN apt-get update && apt-get install -y ca-certificates git-core ssh tzdata
RUN mkdir -p -m 0700 /root/.ssh && ssh-keyscan github.com >> /root/.ssh/known_hosts
RUN git config --global url.ssh://git@github.com/.insteadOf https://github.com/

# add unprivileged user
RUN adduser --shell /usr/sbin/nologin --uid 1000 --disabled-login --no-create-home --gecos '' app \
    && sed -i -r "/^(app|root)/!d" /etc/group /etc/passwd \
    && sed -i -r 's#^(.*):[^:]*$#\1:/usr/sbin/nologin#' /etc/passwd

COPY go.mod go.sum /build/
WORKDIR /build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=ssh \
    go mod download

ADD . /build
WORKDIR /build

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o data-server .

FROM scratch
# add-in our timezone data file
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
# add-in our nologin binary
COPY --from-builder /usr/sbin/nologin /usr/sbin/nologin
# add-in our unprivileged user
COPY --from=builder /etc/passwd /etc/group /etc/shadow /etc/
# add-in our ca certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
#
COPY --from=builder /build/data-server /app/
WORKDIR /app
# from now on, run as the unprivileged user
USER 1000
ENTRYPOINT [ "/app/data-server" ]
