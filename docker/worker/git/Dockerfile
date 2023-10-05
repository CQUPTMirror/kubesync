FROM ghcr.io/cquptmirror/worker:dev

ADD git.sh /

RUN chmod +x /git.sh && apt update && apt install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*
