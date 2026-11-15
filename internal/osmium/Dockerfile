FROM debian:trixie-slim

# osmium-tool is packaged by Debian, so the version is pinned by the base image.
RUN apt-get update \
 && apt-get install -y --no-install-recommends osmium-tool \
 && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["osmium"]
