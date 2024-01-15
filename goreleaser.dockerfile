FROM scratch

LABEL repo="https://github.com/iptecharch/data-server"

COPY data-server /app/
COPY datactl /app/
WORKDIR /app

ENTRYPOINT [ "/app/data-server" ]
