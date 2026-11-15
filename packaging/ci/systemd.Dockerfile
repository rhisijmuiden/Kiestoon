# Debian 13 with systemd as PID 1, for the install test in CI.
FROM debian:trixie
RUN apt-get update -qq \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq systemd systemd-sysv curl ca-certificates >/dev/null \
 && rm -rf /var/lib/apt/lists/*
CMD ["/sbin/init"]
