FROM docker.io/library/golang:1.27.4-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl git jq openssh-client python3 ripgrep tar unzip xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -s /bin/bash vscode \
    && install -d -o vscode -g vscode /workspaces

WORKDIR /workspaces
