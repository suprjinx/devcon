FROM debian:bookworm-slim

# A non-root user so we can exercise remoteUser. bash ships with debian, so the
# example builds without needing apt/network beyond pulling the base image.
RUN useradd --create-home --shell /bin/bash dev
