FROM golang:1.21 AS builder
ARG UID=10000
# no need to include cgo bindings
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# add ca certificates and timezone data files
RUN apt-get install --yes --no-install-recommends ca-certificates tzdata

# add unprivileged user
RUN adduser --shell /usr/sbin/nologin --uid $UID --disabled-login --no-create-home --home /app/ --gecos '' app \
    && sed -i -r "/^(app|root)/!d" /etc/group /etc/passwd \
    && sed -i -r 's#^(.*):[^:]*$#\1:/bin/false#' /etc/passwd
RUN mkdir -p /schemas
#

FROM scratch
ARG UID=10000
# add-in our timezone data file
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
# add-in our unprivileged user
COPY --from=builder /etc/passwd /etc/group /etc/shadow /etc/
# add-in our ca certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --chown=$UID:$UID data-server /app/
COPY --chown=$UID:$UID datactl /app/
COPY --from=builder --chown=$UID:$UID /schemas /schemas
WORKDIR /app

# from now on, run as the unprivileged user
USER $UID

ENTRYPOINT [ "/app/data-server" ]
