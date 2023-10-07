FROM ghcr.io/cquptmirror/worker:dev

ADD pypi.sh /

RUN chmod +x /pypi.sh && pip install bandersnatch --break-system-packages --no-cache-dir
