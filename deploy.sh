#!/bin/bash
set -e
cd /opt/decay-main
git pull origin main
/usr/local/go/bin/go build -o decay-main .
systemctl restart decay-main
