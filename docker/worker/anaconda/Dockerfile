FROM ghcr.io/cquptmirror/worker:dev

ADD anaconda.py /

RUN chmod +x /anaconda.py && apt update && apt install -y --no-install-recommends curl python3-pyquery && rm -rf /var/lib/apt/lists/*
