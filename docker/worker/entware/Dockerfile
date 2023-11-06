FROM ghcr.io/cquptmirror/worker:dev

ADD entware.sh /

RUN chmod +x /entware.sh && apt update && apt install -y --no-install-recommends wget && rm -rf /var/lib/apt/lists/*
