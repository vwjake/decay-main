#!/bin/bash
# Redeploy on the production host: pull, rebuild, restart. Config/secrets are not
# touched here — they live in /etc/decay-main.env (read by the systemd unit). See
# deploy/README.md for one-time server setup.
set -e
cd /opt/decay-main
git pull origin master
/usr/local/go/bin/go build -o decay-main .
systemctl restart decay-main
