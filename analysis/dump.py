#!/usr/bin/env python3
# -*- coding: utf-8 -*-
#   @Time    : 2019/12/18 18:03
#   @Author  : PlatON-Developer
#   @Site    : https://github.com/PlatONnetwork/

import os
import sys

if sys.platform != "linux":
    raise Exception("Unsupported operating system")

BASE_DIR = os.path.dirname(os.path.abspath(__file__))

FILTER_PATH = os.path.join(BASE_DIR, "filter.sh")

STATIC_PATH = os.path.join(BASE_DIR, "static")


def run(cmd):
    """
    The machine executes the cmd command and gets the result
    :param cmd:
    :return:
    """
    r = os.popen(cmd)
    out = r.read()
    r.close()
    return out


def chmod():
    base_cmd = "sudo chmod +x "
    filter_cmd = base_cmd + FILTER_PATH
    run(filter_cmd)
    static_cmd = base_cmd + STATIC_PATH
    run(static_cmd)


def dump_data(datadir, node_cfg, output):
    cmd = "sudo {} -cmd dump -datadir {} -nodes_cfg {} -output {}".format(STATIC_PATH, datadir, node_cfg, output)
    out = run(cmd)
    print("dump data result: ", out)



def dum_validator(node_cfg, output, logdir):
    cmd = "sudo {} -cmd dump_vd -nodes_cfg {} -output {} -logdir {}".format(STATIC_PATH, node_cfg, output, logdir)
    out = run(cmd)
    print("dump validator result: ", out)


def import_data(url, index_name, dump_data_file):
    cmd = "sudo {} -cmd import -url {} -index_name {} -dump_file {}".format(STATIC_PATH, url, index_name, dump_data_file)
    out = run(cmd)
    print("import data result: ", out)


def import_validator(url, index_name, dump_validator_file):
    cmd = "sudo {} -cmd import_vd -url {} -index_name {} -dump_file {}".format(STATIC_PATH, url, index_name, dump_validator_file)
    out = run(cmd)
    print("import validator result: ", out)


def main():
    if len(sys.argv) < 1:
        print("python3 dump.py <commitid>")
    if len(sys.argv) < 2:
        raise Exception("must input commit id")
    commit_id = sys.argv[1]
    node_path = "/data/platon-stress/node"
    url = "http://10.10.8.209:9200"
    if len(commit_id) > 6:
        index_name = "platon-" + commit_id[0:6]
    else:
        index_name = "platon-" + commit_id
    validator_file_name = "validator-nodes.json"
    validator_file = os.path.join(BASE_DIR, validator_file_name)
    tmp = os.path.join(BASE_DIR, "tmp")
    if not os.path.exists(tmp):
        os.mkdir(tmp)
    dump_data_file = os.path.join(tmp, index_name + "_data.json")
    dump_validator_file = os.path.join(tmp, index_name + "_validator.json")
    datadir = os.path.join(node_path, "data")
    logdir = os.path.join(node_path, "log")
    chmod()
    dump_data(datadir, validator_file, dump_data_file)
    dum_validator(validator_file, dump_validator_file, logdir)
    import_data(url, index_name, dump_data_file)
    import_validator(url, index_name, dump_validator_file)
    print("over")


if __name__ == "__main__":
    main()
