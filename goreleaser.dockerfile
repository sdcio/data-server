FROM scratch

COPY data-server /app/
COPY datactl /app/
WORKDIR /app

ENTRYPOINT [ "/app/data-server" ]
