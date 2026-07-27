#!/bin/bash
set -e
cd /opt/decay-main
git pull origin master
/usr/local/go/bin/go build -o decay-main .
systemctl restart decay-main
