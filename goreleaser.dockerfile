FROM scratch

LABEL repo="https://github.com/iptecharch/data-server"

COPY data-server /app/
WORKDIR /app

ENTRYPOINT [ "/app/data-server" ]
