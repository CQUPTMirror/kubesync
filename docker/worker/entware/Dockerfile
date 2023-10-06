FROM ghcr.io/cquptmirror/worker:dev

ADD entware.sh /

RUN chmod +x /entware.sh && apt update && apt install -y --no-install-recommends wget ca-certificates && rm -rf /var/lib/apt/lists/*
