#!/usr/bin/env python3
# -*- coding: utf-8 -*-
#   @Time    : 2019/12/19 14:32
#   @Author  : PlatON-Developer
#   @Site    : https://github.com/PlatONnetwork/

import os
import sys
import time
from dump import run, BASE_DIR


PLATON_BIN = os.path.join(BASE_DIR, "platon")


def chmod(*args):
    base_cmd = "sudo chmod +x "
    for arg in args:
        cmd = base_cmd + arg
        run(cmd)


def get_txs(url):
    cmd = "{} attach {} --exec 'platon.getBlock(platon.blockNumber).transactions.length'".format(PLATON_BIN, url)
    return run(cmd)


def main():
    if len(sys.argv) >= 2:
        url = sys.argv[1]
    else:
        url = "ws://localhost:8808"
    chmod(PLATON_BIN)
    try:
        while True:
            print(get_txs(url))
            time.sleep(1)
    except KeyboardInterrupt as e:
        print(e)


if __name__ == "__main__":
    main()
