FROM ghcr.io/cquptmirror/worker:dev

ADD pypi.sh /

RUN chmod +x /pypi.sh && apt update && apt install -y --no-install-recommends python3-pip && rm -rf /var/lib/apt/lists/* && pip install bandersnatch --break-system-packages --no-cache-dir
