#!/usr/bin/env python3
#

"""
pip install ruamel.yaml
pip install fabric
"""

import concurrent.futures as futures
import json
import os
import sys

import fabric
from fabric import Connection
from ruamel.yaml import YAML

DEPLOY_HOME = "./deploy-batch-gen"

"""
deploy batch

Example:
python ./deploy-bath.py deploy ./config.json bath_tag

config.json:
[{
  "host": "10.10.10.10",
  "user": "pchuant",
  "passwd": "123456",
  "path": "/tmp/batch",
  "name": "test",
  "url": "http://127.0.0.1:6789",
  "nodekey": "1111",
  "delegate": true,
  "num_accounts": 40,
}]
"""


def _connection(node):
    kw = None
    cfg = None
    kw = {"password": node["passwd"]}
    cfg = fabric.Config(overrides={"sudo": {"password": node["passwd"]}})
    return Connection(host=node["host"], user=node["user"], connect_kwargs=kw, config=cfg)


def put_file(node, tag):
    c = _connection(node)
    # upload file
    c.put("{}/{}/docker-compose.yaml".format(DEPLOY_HOME,
                                             node["name"]), "/tmp/docker-compose-{}.yaml".format(node["name"]),)
    # pull docker image
    c.sudo("docker pull {}:{}".format(node["registry"], tag), hide=True)
    # make path
    c.sudo("mkdir -p {}/{}".format(node["path"], node["name"]), hide=True)
    # copy file
    c.sudo("cp /tmp/docker-compose-{}.yaml {}/{}/docker-compose.yaml".format(
        node["name"], node["path"], node["name"]), hide=True)
    # run
    c.sudo('bash -c "cd {}/{} && docker-compose up -d"'.format(
        node["path"], node["name"]), hide=True)


def batch_executor(nodes, perform, name):
    with futures.ThreadPoolExecutor(max_workers=10) as executor:
        i = 0
        fs = []
        for node in nodes:
            if "boot" in node and node["boot"]:
                continue
            path = "{}/{}".format(DEPLOY_HOME, node["name"])
            os.makedirs(name=path, mode=0o766, exist_ok=True)
            fs.append((executor.submit(perform, node, i), (node["name"], node["host"])))
            i = i + 1

        for (f, n) in fs:
            e = f.exception()
            if e:
                print("{} {}@{:<10} result: exception<{}>".format(
                    name, n[0], n[1], e))
            else:
                print("{} {}@{:<10} result: success".format(name, n[0], n[1]))


def delegate(cfg, tag="bech32"):
    old_nodes = None
    with open(cfg, "rb") as file:
        old_nodes = json.load(file)
        file.close()
    cmd = "side_delegate"
    nodes = [node for node in old_nodes if not node.get("consensus")]

    def perform(node, i):
        _gen_docker_compose(node, tag, i, cmd)
        put_file(node, tag)

    batch_executor(nodes, perform, delegate.__name__)


def transfer(cfg, tag="bech32"):
    nodes = None
    with open(cfg, "rb") as file:
        nodes = json.load(file)
        file.close()
    cmd = "side_transfer"

    def perform(node, idx):   
        _gen_docker_compose(node, tag, idx, cmd)
        put_file(node, tag)

    batch_executor(nodes, perform, transfer.__name__)


# def pro_transfer(cfg, tag="batch", send_txs=3, proportion=7):
#     nodes = None
#     with open(cfg, "rb") as file:
#         nodes = json.load(file)
#         file.close()
#     cmd = "proportion_transfer"

#     def perform(node, i):
#         _gen_docker_compose(node, tag, i, cmd, send_txs, proportion)
#         put_file(node, tag)

#     batch_executor(nodes, perform, pro_transfer.__name__)


# def from_transfer(cfg, tag="batch", send_txs=3, proportion=7):
#     nodes = None
#     with open(cfg, "rb") as file:
#         nodes = json.load(file)
#         file.close()
#     cmd = "same_from_transfer"

#     def perform(node, i):
#         _gen_docker_compose(node, tag, i, cmd, send_txs, proportion)
#         put_file(node, tag)

#     batch_executor(nodes, perform, from_transfer.__name__)


# def to_transfer(cfg, tag="batch", send_txs=3, proportion=7):
#     nodes = None
#     with open(cfg, "rb") as file:
#         nodes = json.load(file)
#         file.close()
#     cmd = "same_to_transfer"

#     def perform(node, i):
#         _gen_docker_compose(node, tag, i, cmd, send_txs, proportion)
#         put_file(node, tag)

#     batch_executor(nodes, perform, to_transfer.__name__)


def _gen_docker_compose(node, tag, idx, cmd):
    count = int(node.get("num_accounts", 50))
    nm = node["name"]
    if len(nm) > 30:
        nm = nm[:30]
    dc = {
        "version": "3.7",
        "services": {
            "batch": {
                "image": "{}:{}".format(node["registry"], tag),
                "container_name": nm + "-batch",
                "network_mode": "host",
                "environment": {
                    "USE_CMD": cmd,
                    "URL": node.get("ws") or node.get("url"),
                    "IDX": idx * count,
                    "COUNT": count,
                    "NODEKEY": node["nodekey"],
                    "BLSKEY": node["blskey"],
                    "NODENAME": nm,
                    "STAKING_FLAG": "false",
                    "ONLY_CONSENSUS_FLAG": "false",
                    "DELEGATE_FLAG": 'true',
                    "R_ACCOUNTS": "true",
                    "RAND_COUNT": 200000,
                    "R_IDX": idx * 200000,
                    "CHAINID": 101,
                    # "SENDTXS": send_txs,
                    # "PROPORTION": proportion,
                },
            }
        },
    }

    with open("{}/{}/docker-compose.yaml".format(DEPLOY_HOME, nm), "wb") as file:
        yaml = YAML()
        yaml.dump(dc, file)
        file.close()


def staking(cfg, program_version=2816, tag="bech32"):
    old_nodes = None
    with open(cfg, "rb") as file:
        old_nodes = json.load(file)
        file.close()
    nodes = []
    nodes = [node for node in old_nodes if node.get("staking")]

    def perform(node, i):
        _gen_staking_compose(node, tag, program_version)

        put_file(node, tag)

    batch_executor(nodes, perform, staking.__name__)


def _gen_staking_compose(node, tag, program_version):
    nm = node["name"]
    if len(nm) > 30:
        nm = nm[:30]
    dc = {
        "version": "3.7",
        "services": {
            "staking":
            {
                "network_mode": "host",
                "image": "%s:%s" % (node["registry"], "batch"),
                "container_name": "{}-batch-staking".format(nm),
                "environment": {
                    "USE_CMD": "batch_staking",
                    "URL": "ws://{}:{}".format("127.0.0.1", node["ws_port"]),
                    "NODENAME": nm,
                    "BLSKEY": node["blskey"],
                    "NODEKEY": node["nodekey"],
                    "PRIVATE_KEY": node["private_key"],
                    "PROGRAM_VERSION": int(program_version),
                    "CHAINID": 101,
                },
            }
        },
    }

    with open("{}/{}/docker-compose.yaml".format(DEPLOY_HOME, nm), "wb") as file:
        yaml = YAML()
        yaml.dump(dc, file)
        file.close()


def wasm(cfg, tag="batch-wasm"):
    nodes = None
    with open(cfg, "rb") as file:
        nodes = json.load(file)
        file.close()
    cmd = "side_wasm"
    contract_addr, invoke_data = "", ""

    def perform(node, idx):
        _gen_vm_docker_compose(node, tag, idx, cmd, contract_addr, invoke_data)
        put_file(node, tag)

    batch_executor(nodes, perform, wasm.__name__)


def evm(cfg, tag="batch-wasm"):
    nodes = None
    with open(cfg, "rb") as file:
        nodes = json.load(file)
        file.close()
    cmd = "side_wasm"
    contract_addr, invoke_data = "", ""

    def perform(node, idx):
        _gen_vm_docker_compose(node, tag, idx, cmd, contract_addr, invoke_data)
        put_file(node, tag)

    batch_executor(nodes, perform, evm.__name__)


def _gen_vm_docker_compose(node, tag, idx, cmd, contract_addr, invoke_data):
    count = int(node.get("num_accounts", 50))
    nm = node["name"]
    if len(nm) > 30:
        nm = nm[:30]
    dc = {
        "version": "3.7",
        "services": {
            "batch": {
                "image": "{}:{}".format(node["registry"], tag),
                "container_name": nm + "-batch",
                "network_mode": "host",
                "environment": {
                    "CMD": cmd,
                    "URL": node.get("ws") or node.get("url"),
                    "IDX": idx * count,
                    "COUNT": count,
                    "NODEKEY": node["nodekey"],
                    "BLSKEY": node["blskey"],
                    "NODENAME": nm,
                    "STAKING_FLAG": "false",
                    "ONLY_CONSENSUS_FLAG": "false",
                    "DELEGATE_FLAG": 'true',
                    "R_ACCOUNTS": "true",
                    "RAND_COUNT": 200000,
                    "R_IDX": idx * 200000,
                    "CHAINID": 101,
                    "CONTRACT_ADDR": contract_addr,
                    "INVOKE_DATA": invoke_data,
                    "INTERVAL": 150,
                },
            }
        },
    }

    with open("{}/{}/docker-compose.yaml".format(DEPLOY_HOME, nm), "wb") as file:
        yaml = YAML()
        yaml.dump(dc, file)
        file.close()


def stop(cfg):
    def perform(node, i):
        c = _connection(node)
        c.sudo('bash -c "docker-compose -f {}/{}/docker-compose.yaml stop"'.format(
            node["path"], node["name"]), hide=True,)
    with open(cfg, "rb") as file:
        nodes = json.load(file)
        batch_executor(nodes, perform, stop.__name__)


def start(cfg):
    def perform(node, i):
        c = _connection(node)
        c.sudo('bash -c "docker-compose -f {}/{}/docker-compose.yaml start"'.format(
            node["path"], node["name"]), hide=True,)
    with open(cfg, "rb") as file:
        nodes = json.load(file)
        batch_executor(nodes, perform, start.__name__)


def remove(cfg):
    def perform(node, i):
        c = _connection(node)
        # stop batch
        c.sudo('bash -c "docker-compose -f {}/{}/docker-compose.yaml down"'.format(
            node["path"], node["name"]), hide=True)
        # remove directory
        c.sudo(
            "rm -f {}/{}/docker-compose.yaml".format(node["path"], node["name"]), hide=True)
    with open(cfg, "rb") as file:
        nodes = json.load(file)
        file.close()

        batch_executor(nodes, perform, remove.__name__)


def status(cfg):
    def perform(node):
        c = _connection(node)
        return c.sudo(
            'bash -c "cd {}/{} && docker-compose ps"'.format(
                node["path"], node["name"]
            ),
            hide=True,
        )

    file = open(cfg, "rb")
    nodes = json.load(file)
    file.close()

    with futures.ThreadPoolExecutor(max_workers=10) as executor:
        fs = []
        for node in nodes:
            if "boot" in node and node["boot"]:
                continue
            fs.append(
                (executor.submit(perform, node),
                    (node["name"], node["host"]))
            )
        for (f, n) in fs:
            e = f.exception()
            if e:
                print("{}@{:<10} exception<{}>".format(n[0], n[1], e))
            else:
                res = f.result().stdout
                ls = res.split("\n")
                r = "NotExist"
                if len(ls) > 2:
                    ll = ls[2].split()
                    r = ll[2]

                print("{}@{:<10} {}".format(n[0], n[1], r))


def prune(cfg):
    def perform(node, i):
        c = _connection(node)
        c.run('bash -c "docker container prune -f"')
    file = open(cfg, "rb")
    old_nodes = json.load(file)
    file.close()
    nodes = []

    def check_had(n):
        for nod in nodes:
            if n["host"] == nod["host"]:
                return True
        return False

    for node in old_nodes:
        if not check_had(node):
            nodes.append(node)
    batch_executor(nodes, perform, prune.__name__)


if __name__ == "__main__":

    def do(cmd):
        return {
            "transfer": transfer,
            # "from_transfer": from_transfer,
            # "to_transfer": to_transfer,
            # "pro_transfer": pro_transfer,
            "delegate": delegate,
            "remove": remove,
            "stop": stop,
            "start": start,
            "status": status,
            "staking": staking,
            "prune": prune,
            "wasm": wasm,
            "evm": evm,
        }[cmd]

    if len(sys.argv) > 2:
        do(sys.argv[1])(*sys.argv[2:])
    else:
        do(sys.argv[1])()
