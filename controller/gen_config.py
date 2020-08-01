#!/usr/bin/env python3

import sys
import json

num_accounts = 40
num_consensus = 4
total = 8
user = "platon"
passwd = "Platon123!"
registry = "shinnng/platon-go"

staking = True
boot = True
base_name = "stress"
base_path = "/data/platon-stress/node"
p2p_port_range = (16789, 16889)
rpc_port_range = (6789, 6889)
ws_port_range = (7789, 7889)
pprof_port_range = (8789, 8889)
fluent_port_range = (9789, 9889)


def gen(nodes_file, config_file):
    key_fp = open("./tmpl/all_addr_and_private_keys.json")
    keys = json.load(key_fp)
    key_fp.close()
    with open(nodes_file) as file:
        nodes = file.readlines()
        result_nodes = []
        count = total
        if boot:
            count = count + 1
        if len(nodes) < count:
            for node in nodes:
                result_nodes.append(
                    (
                        node.strip(),
                        p2p_port_range[0],
                        rpc_port_range[0],
                        ws_port_range[0],
                        pprof_port_range[0],
                        fluent_port_range[0],
                    )
                )
            result_nodes = extend_nodes(result_nodes, count - len(result_nodes))
        else:
            for node in nodes[:count]:
                result_nodes.append(
                    (
                        node.strip(),
                        p2p_port_range[0],
                        rpc_port_range[0],
                        ws_port_range[0],
                        pprof_port_range[0],
                        fluent_port_range[0],
                    )
                )
        print(f'There are {len(result_nodes)} nodes are generated, as follows: {result_nodes}')

        static = True
        if boot:
            static = False

        config_nodes = []
        num = 1
        for (
                host,
                p2p_port,
                rpc_port,
                ws_port,
                pprof_port,
                fluent_port,
        ) in result_nodes[:num_consensus]:
            config_nodes.append(
                {
                    "host": host,
                    "port": 22,
                    "user": user,
                    "passwd": passwd,
                    "ws": "ws://{}:{}".format("127.0.0.1", ws_port),
                    "url": "http://{}:{}".format("127.0.0.1", rpc_port),
                    "p2p_port": "{}".format(p2p_port),
                    "rpc_port": "{}".format(rpc_port),
                    "ws_port": "{}".format(ws_port),
                    "pprof_port": "{}".format(pprof_port),
                    "fluent_port": "{}".format(fluent_port),
                    "name": "{}{}".format(base_name, num),
                    "path": "{}{}".format(base_path, num),
                    "registry": registry,
                    "consensus": True,
                    "static": static,
                    "num_accounts": num_accounts,
                }
            )
            num = num + 1

        for (
                host,
                p2p_port,
                rpc_port,
                ws_port,
                pprof_port,
                fluent_port,
        ) in result_nodes[num_consensus:]:
            config_nodes.append(
                {
                    "host": host,
                    "port": 22,
                    "user": user,
                    "passwd": passwd,
                    "ws": "ws://{}:{}".format("127.0.0.1", ws_port),
                    "url": "http://{}:{}".format("127.0.0.1", rpc_port),
                    "p2p_port": "{}".format(p2p_port),
                    "rpc_port": "{}".format(rpc_port),
                    "ws_port": "{}".format(ws_port),
                    "pprof_port": "{}".format(pprof_port),
                    "fluent_port": "{}".format(fluent_port),
                    "name": "{}{}".format(base_name, num),
                    "path": "{}{}".format(base_path, num),
                    "registry": registry,
                    "consensus": False,
                    "static": static,
                    "staking": staking,
                    "num_accounts": num_accounts,
                    "private_key": keys[-num]["private_key"]
                }
            )
            num = num + 1

        if boot:
            doc = config_nodes[-1]
            doc["name"] = base_name + "boot"
            doc["boot"] = boot
            doc["staking"] = False
            del doc["private_key"]
            config_nodes[-1] = doc

        fp = open(config_file, "w")
        json.dump(config_nodes, fp, indent=2)
        fp.close()


def extend_nodes(result_nodes, remaining):
    full_nodes = result_nodes[:]
    nodes = result_nodes[:]
    if remaining < len(result_nodes):
        nodes = result_nodes[:remaining]
    completion_nodes = []
    while True:
        for (host, p2p_port, rpc_port, ws_port, pprof_port, fluent_port) in nodes:
            completion_nodes.append(
                (
                    host,
                    p2p_port + 1,
                    rpc_port + 1,
                    ws_port + 1,
                    pprof_port + 1,
                    fluent_port + 1,
                )
            )
            full_nodes.append(
                (
                    host,
                    p2p_port + 1,
                    rpc_port + 1,
                    ws_port + 1,
                    pprof_port + 1,
                    fluent_port + 1,
                )
            )

            if remaining == len(completion_nodes):
                break

        if remaining == len(completion_nodes):
            break

        nodes = completion_nodes[:]

    return full_nodes


def update_key(node_config):
    def update(node):
        blskey = ""
        with open("{}/{}/data/blskey".format("./deploy-docker", node["name"])) as f:
            blskey = f.read()
            f.close()
            node["blskey"] = blskey

        nodekey = ""
        with open("{}/{}/data/nodekey".format("./deploy-docker", node["name"])) as f:
            nodekey = f.read()
            f.close()
            node["nodekey"] = nodekey
        blspub = ""
        with open("{}/{}/data/blspub".format("./deploy-docker", node["name"])) as f:
            blspub = f.read()
            f.close()
            node["blspub"] = blspub

        pubkey = ""
        with open("{}/{}/data/pub".format("./deploy-docker", node["name"])) as f:
            pubkey = f.read()
            f.close()
            node["pubkey"] = pubkey

    with open(node_config, "r") as infile:
        nodes = json.load(infile)
    for node in nodes:
        update(node)
    with open(node_config, "w") as outfile:
        json.dump(nodes, outfile, indent=2)


def update_account(node_config, num):
    with open(node_config, "r") as infile:
        nodes = json.load(infile)
    for node in nodes:
        node["num_accounts"] = num
    with open(node_config, "w") as outfile:
        json.dump(nodes, outfile, indent=2)


if __name__ == "__main__":
    def do(cmd):
        return {
            "gen_deploy": gen,
            "update_key": update_key,
            "update_account": update_account,
        }[cmd]

    if len(sys.argv) < 3:
        print("python {} gen_deploy <ips.txt> <node_config.json>".format(
            sys.argv[0]))
    do(sys.argv[1])(*sys.argv[2:])
