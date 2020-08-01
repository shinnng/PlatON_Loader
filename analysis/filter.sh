#!/usr/bin/env bash

zgrep -a "Update validator" $1/platon.* $1/platon/buffer.*.log | grep "lastNumber" > $2
