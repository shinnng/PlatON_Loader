#!/usr/bin/env bash

has_cron=`grep "$1" /etc/crontab | grep -v grep|wc -l`
if [ $has_cron -gt 0 ]; then
    exit 0
fi

cp /etc/crontab /etc/crontab.bak
echo "* * * * * root /usr/sbin/logrotate /etc/logrotate.d/$1.conf" >> /etc/crontab
