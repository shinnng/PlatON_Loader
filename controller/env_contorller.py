#!/usr/bin/env python2.7

"""
pip3 install paramiko
pip3 install futures
pip3 install scp
pip3 install ruamel.yaml
"""

import json
import os
import shutil
import sys
import time
from concurrent import futures

import paramiko
from ruamel.yaml import YAML
from scp import SCPClient

# import config_docker as config
need_point = False
if sys.platform == "linux":
    need_point = True
    PLATON_BIN = os.path.abspath("./bin/platon")
else:
    PLATON_BIN = os.path.abspath("./bin/platon.exe")


def connect(node):
    try:
        ssh = paramiko.SSHClient()
        ssh.load_system_host_keys()
        ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        ssh.connect(node["host"], node["port"], node["user"], node["passwd"])
        return ssh
    except Exception as e:
        raise e


def exec_cmd(ssh, cmd, passwd=None):
    try:
        print(cmd)
        stdin, stdout, stderr = ssh.exec_command(cmd)
        if passwd:
            stdin.write(passwd + "\n")
        for line in stdout.readlines():
            print(line)
    except Exception as e:
        print("exec {} expect: {}".format(cmd, e))
        return


def exec_cmd_return(ssh, cmd, passwd):
    try:
        stdin, stdout, stderr = ssh.exec_command(cmd)
        if passwd:
            stdin.write(passwd + "\n")
        lines = []
        for line in stdout.readlines():
            lines.append(line)
        for line in stderr.readlines():
            lines.append(line)
        return lines
    except Exception as e:
        print("exec {} expect: {}".format(cmd, e))
        return []


def upload_via_scp(ssh, local, remote):
    scp = SCPClient(ssh.get_transport())
    scp.put(local, recursive=True, remote_path=remote)
    scp.close()


def get_via_scp(ssh, remote, local):
    scp = SCPClient(ssh.get_transport())
    scp.get(remote, local)
    scp.close()


def upload(ssh, file, dst):
    sftp = paramiko.SFTPClient.from_transport(ssh.get_transport())
    sftp = ssh.open_sftp()

    sftp.put(file, dst)
    sftp.close()


def deploy_docker_and_docker_compose(nodes):
    deploy_node = []
    for node in nodes:
        found = False
        for dn in deploy_node:
            if dn["host"] == node["host"]:
                found = True
        if not found:
            deploy_node.append(node)

    def perform(node):
        ssh = connect(node)
        upload_via_scp(ssh, "./script/docker-install.sh",
                       "/tmp/docker-install.sh")
        # upload_via_scp(ssh, "./script/docker-compose", "/tmp/docker-compose")
        exec_cmd(ssh, "sudo -S -p '' bash /tmp/docker-install.sh",
                 node["passwd"])
        exec_cmd(
            ssh, "sudo -S -p '' gpasswd -a ${USER} docker", node["passwd"])
    with futures.ThreadPoolExecutor(max_workers=len(nodes)) as executor:
        for node in deploy_node:
            executor.submit(perform, node)


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


def ethkey_gen(nodes):
    if sys.platform == "linux":
        keytool = os.path.abspath("./bin/keytool")
    else:
        keytool = os.path.abspath("./bin/keytool.exe")

    def gen(node):
        path = "./deploy-docker/" + node["name"] + "/data"
        try:
            os.makedirs(path, 0o755)
        except os.error:
            pass

        if len(os.listdir(path)) > 0:
            print("node(%s) already gen key" % (node["name"]))
            with open("{}/{}/data/blskey".format("./deploy-docker", node["name"])) as f:
                node["blskey"] = f.read()
            with open("{}/{}/data/blspub".format("./deploy-docker", node["name"])) as f:
                node["blspub"] = f.read()
            with open("{}/{}/data/nodekey".format("./deploy-docker", node["name"])) as f:
                node["nodekey"] = f.read()
            with open("{}/{}/data/pub".format("./deploy-docker", node["name"])) as f:
                node["pubkey"] = f.read()
            return

        keypair = run("{} genkeypair".format(keytool))
        lines = keypair.split("\n")
        for l in lines:
            kv = l.split(":")
            if kv[0] == "PrivateKey":
                key = kv[1].strip()
                fp = open(path + "/nodekey", "w")
                fp.write(key)
                fp.close()
                node["nodekey"] = key
            elif kv[0] == "PublicKey ":
                key = kv[1].strip()
                fp = open(path + "/pub", "w")
                fp.write(key)
                fp.close()
                node["pubkey"] = key
            elif "Address" in kv[0]:
                key = kv[1].strip()
                fp = open(path + "/addr", "w")
                fp.write(key)
                fp.close()

        keypair = run("{} genblskeypair --json".format(keytool))
        objs = json.loads(keypair)
        fp = open("{}/blskey".format(path), "w")
        fp.write(objs["PrivateKey"])
        fp.close()
        fp = open("{}/blspub".format(path), "w")
        fp.write(objs["PublicKey"])
        fp.close()
        node["blskey"] = objs["PrivateKey"]
        node["blspub"] = objs["PublicKey"]

    for node in nodes:
        gen(node)


def gen_fluent_config(node):
    """
<filter platon*>
  @type parser
  key_name log
  <parse>
    @type regexp
    expression /(?!.*breakpoint_log.*)(?<log_json>{\".*})/g
  </parse>
</filter>
    """

    tpl = """
<system>
  log_level error
</system>

<source>
  @type forward
  port %s
  bind 0.0.0.0
</source>

<filter platon*>
  @type parser
  key_name log
  <parse>
    @type json
  </parse>
</filter>
"""
#     match_es = """
# <match platon*>
#   @type forward
#   send_timeout 60s
#   recover_wait 10s
#   hard_timeout 60s

#   <server>
#     name fluent-forward
#     host %s
#     port %s
#     weight 100
#   </server>
# </match>
# """

    match_file = """
<match platon*>
    @type file
    path /data/log/platon-cbft
    append true
    <format>
      @type json
    </format>
    <buffer>
      chunk_limit_size 10MB
    </buffer>
</match>
"""

    label = """
<label @ERROR>
  <match platon*>
    @type file
    path /data/log/platon
    append true
    <format>
      @type single_value
      message_key log
    </format>
    <buffer>
      chunk_limit_size 10MB
    </buffer>
  </match>
</label>
"""

    """
<match platon*>
  @type elasticsearch
  host %s
  port %s
  logstash_format true
  logstash_prefix ${tag}-cbft
</match>
  <buffer>
    flush_at_shutdown true
    flush_mode interval
    flush_interval 1s
  </buffer>

<label @ERROR>
  <match platon*>
    @type elasticsearch
    host %s
    port %s
    logstash_format true
    logstash_prefix ${tag}-log
  </match>
</label>
    """
    fluent_config = tpl % (node["fluent_port"]) + match_file + label

    fp = open("./deploy-docker/" + node["name"] + "/fluent.conf", "w")
    fp.write(fluent_config)
    fp.close()


def gen_logrotate_config(node):
    logrotate = """
%s/log/platon*.log
{
   compress
   size 200M
   rotate 500
   su root root

   postrotate
   endscript
}
"""

    lc = logrotate % (node["path"])
    fp = open("./deploy-docker/%s/%s.conf" % (node["name"], node["name"]), "w")
    fp.write(lc)
    fp.close()


def gen_docker_compose_config(
    node, tag, program_version, bootnode, log_to_fluent=False, idx=0
):
    nm = node["name"]
    if len(nm) > 30:
        nm = nm[:30]
    dc = {
        "version": "3.7",
        "services": {
            "platon": {
                "image": "%s:%s" % (node["registry"], tag),
                "container_name": node["name"],
                "volumes": ["%s:/data/platon" % (node["path"])],
                "network_mode": "host",
                "environment": {
                    "ENABLE_DEBUG": "true",
                    "ENABLE_PPROF": "true",
                    "ENABLE_WS": "true",
                    "WSAPI": "platon,debug,txpool,metrics,admin,net",
                    "ENABLE_RPC": "true",
                    "RPCAPI": "platon,debug,txpool,metrics,admin,net",
                    "VERBOSITY": 1,
                    "INIT": "true",
                    "NEW_ACCOUNT": "true",
                    "ENABLE_DISCOVER": "true",
                    "P2PPORT": node["p2p_port"],
                    "WSPORT": node["ws_port"],
                    "RPCPORT": node["rpc_port"],
                    "PPROFPORT": node["pprof_port"],
                    "DISABLEDBGC": "false",
                    "DBGCINTERVAL": 1000,
                    "DBGCTIMEOUT": "5m",
                    "DBGCMPT": "true",
                    "MAXPEERS": 50,
                    "HOST": "0.0.0.0",
                    # "TXCOUNT": 2500,
                    "CACHE_TRIEDB": 512,
                },
            }
        },
    }

    if "boot" not in node or not node["boot"]:
        obj = dc["services"]["platon"]["environment"]
        obj["BOOTNODES"] = bootnode
        dc["services"]["platon"]["environment"] = obj
    else:
        obj = dc["services"]["platon"]["environment"]
        obj["ENABLE_LIGHT_SRV"] = "true"
        obj["MAXPEERS"] = 100
        obj["BOOTNODES"] = bootnode
        dc["services"]["platon"]["environment"] = obj

    if log_to_fluent:
        gen_fluent_config(node)

        dc["services"]["platon"]["logging"] = {
            "driver": "fluentd",
            "options": {
                "fluentd-address": "%s:%s" % (node["host"], node["fluent_port"]),
                "tag": "platon",
                "fluentd-async-connect": "true",
            },
        }
        dc["services"]["platon"]["depends_on"] = ["fluent"]
        dc["services"]["fluent"] = {
            "network_mode": "host",
            "image": "fluentd",
            "container_name": "%s-fluent" % (node["name"]),
            "volumes": [
                "%s/fluent.conf:/fluentd/etc/fluent.conf" % node["path"],
                "%s/log:/data/log/" % (node["path"]),
            ],
        }
    if "staking" in node and node["staking"]:
        dc["services"]["platon"]["depends_on"].append("staking")
        dc["services"]["staking"] = {
            "network_mode": "host",
            "image": "%s:%s" % (node["registry"], "bech32"),
            "container_name": "{}-staking".format(nm),
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
    fp = open("./deploy-docker/" + node["name"] + "/docker-compose.yaml", "w")
    yaml = YAML()
    yaml.dump(dc, fp)
    fp.close()


def update_docker_compose_config(node, tag):
    fp = open("./deploy-docker/%s/docker-compose.yaml" % (node["name"]), "r")
    yaml = YAML()
    dc = yaml.load(fp)
    fp.close()
    try:
        dc["services"]["platon"]["environment"].pop("INIT")
        dc["services"]["platon"]["environment"].pop("NEW_ACCOUNT")
        dc["services"]["platon"]["depends_on"].pop("staking")
        del dc["services"]["staking"]
    except Exception as e:
        print(e)
    dc["services"]["platon"]["image"] = "%s:%s" % (node["registry"], tag)
    fp = open("./deploy-docker/%s/docker-compose.yaml" % (node["name"]), "w")
    yaml.dump(dc, fp)
    fp.close()


def gen_genesis(nodes, gstmpl, tag, program_version):
    fp = open(gstmpl, "r")
    tmpl = json.load(fp)
    fp.close()

    tmpl["config"]["cbft"]["initialNodes"] = []
    # tmpl["config"]["cbft"]["validatorMode"] = "inner"
    # tmpl["config"]["cbft"]["validatorMode"] = "static"

    newGs = open("./deploy-docker/genesis.json", "w")
    static_nodes_file = open("./deploy-docker/static-nodes.json", "w")
    static_nodes = []
    validators_file = open("./deploy-docker/validator-nodes.json", "w")
    validators = []
    # bootnodes = ""
    for node in nodes:
        if "boot" in node and node["boot"]:
            # with open("./deploy-docker/%s/data/pub" % (node["name"]), "r") as fp:
            #     pubkey = fp.read()
            #     fp.close()
            bootnode = "enode://%s@%s:%s" % (node["pubkey"],
                                             node["host"], node["p2p_port"])
            break

    i = 0
    for node in nodes:
        # path = "./deploy-docker/" + node["name"] + "/data/"
        pubkey = node["pubkey"]
        blspubkey = node["blspub"]
        nodeid = pubkey
        if "static" in node and node["static"]:
            nodeid = "enode://{}@{}:{}".format(pubkey,
                                               node["host"], node["p2p_port"])
            static_nodes.append(nodeid)

        staking = False
        if "staking" in node and node["staking"]:
            staking = True

        validators.append(
            {
                "index": i,
                "nodeID": pubkey,
                "name": node["name"],
                "host": "%s:%s" % (node["host"], node["rpc_port"]),
                "blsPubKey": blspubkey,
                "staking": staking,
            }
        )

        if node["consensus"]:
            tmpl["config"]["cbft"]["initialNodes"].append(
                {"node": nodeid, "blsPubKey": blspubkey}
            )

        gen_docker_compose_config(
            node, tag, program_version, bootnode, True, i)
        gen_logrotate_config(node)

        i = i + 1

    json.dump(tmpl, newGs, indent=2)
    newGs.close()

    json.dump(static_nodes, static_nodes_file, indent=2)
    static_nodes_file.close()

    vds = {"validateNodes": validators}
    json.dump(vds, validators_file, indent=2)
    validators_file.close()


def deploy_platon(nodes, tag, program_version):
    ethkey_gen(nodes)
    gen_genesis(nodes, "./tmpl/genesis.json", tag, program_version)

    def perform(node):
        path = "./deploy-docker/" + node["name"]
        shutil.copyfile("./deploy-docker/genesis.json", path + "/genesis.json")
        if os.path.isfile("./deploy-docker/static-nodes.json"):
            shutil.copyfile("./deploy-docker/static-nodes.json",
                            path + "/data/static-nodes.json")
        shutil.copyfile("shell/platon-log-cron.sh",
                        path + "/platon-log-cron.sh")

        ssh = connect(node)
        exec_cmd(ssh, "sudo -S -p '' rm -rf " +
                 node["path"] + "/data", node["passwd"])
        exec_cmd(ssh, "sudo -S -p '' mkdir -p " + node["path"], node["passwd"])
        exec_cmd(ssh, "sudo -S -p '' mkdir -p " +
                 node["path"] + "/log", node["passwd"])
        exec_cmd(ssh, "sudo -S -p '' chmod 777 %s/log" %
                 (node["path"]), node["passwd"])

        exec_cmd(ssh, "rm -rf /tmp/" + node["name"])
        upload_via_scp(ssh, local="./deploy-docker/" +
                       node["name"], remote="/tmp/" + node["name"])

        # copy files
        exec_cmd(ssh, "cd /tmp/" +
                 node["name"] + " && sudo -S -p '' cp -r * " + node["path"], node["passwd"],)
        exec_cmd(
            ssh, "sudo -S -p '' docker pull {}:{}".format(node["registry"], "bech32"), node["passwd"],)
        exec_cmd(
            ssh, "sudo -S -p '' docker pull {}:{}".format(node["registry"], tag), node["passwd"],)
        exec_cmd(ssh, "cd %s && sudo -S -p '' docker-compose up -d" %
                 (node["path"]), node["passwd"],)

        # logrotate
        exec_cmd(ssh, "sudo -S -p '' cp %s/%s.conf /etc/logrotate.d" %
                 (node["path"], node["name"]), node["passwd"],)
        exec_cmd(ssh, "sudo -S -p '' bash %s/platon-log-cron.sh %s" %
                 (node["path"], node["name"]), node["passwd"],)
        exec_cmd(ssh, "sudo -S -p service cron restart", node["passwd"])

    with futures.ThreadPoolExecutor(max_workers=10) as executor:
        fs = []
        for node in nodes:
            fs.append((executor.submit(perform, node),
                       (node["name"], node["host"])))

        for (f, n) in fs:
            e = f.exception()
            if e:
                print("deploy {}@{} exception: {}".format(n[0], n[1], e))
            else:
                print("deploy {}@{} success".format(n[0], n[1]))


def check(nodes):
    def perform(node):
        ssh = connect(node)
        lines = exec_cmd_return(
            ssh,
            "cd %s && sudo -S -p '' docker-compose ps" % (node["path"]),
            node["passwd"],
        )
        status = "Not Exist"
        printed = False
        for line in lines:
            if node["name"] in line:
                ls = line.split(" ")
                nl = []
                for s in ls:
                    if s == "" or s == " ":
                        continue
                    nl.append(s)
                if nl[0] == node["name"]:
                    print("%s@%s               \t\t%s" %
                          (nl[0], node["host"], nl[2]))
                else:
                    print("%s@%s               \t%s" %
                          (nl[0], node["host"], nl[5]))
                printed = True

        if not printed:
            print("%s@%s                \t%s" %
                  (node["name"], node["host"], status))

    with futures.ThreadPoolExecutor(max_workers=10) as executor:
        for node in nodes:
            executor.submit(perform, node)


def deploy(config_file, tag="latest", program_version=2816):
    with open(config_file, "r") as infile:
        nodes = json.load(infile)

        # deploy_docker_and_docker_compose(nodes)
        try:
            deploy_platon(nodes, tag, program_version)
            check(nodes)
            with open(config_file, "w") as outfile:
                json.dump(nodes, outfile, indent=2)
        except Exception as e:
            raise e


def install_docker(config_file):
    with open(config_file, "r") as infile:
        nodes = json.load(infile)
        deploy_docker_and_docker_compose(nodes)


def start(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def perform(node):
        ssh = connect(node)
        exec_cmd(ssh, "cd %s && sudo -S -p '' docker-compose start" %
                 (node["path"]), node["passwd"],)

    with futures.ThreadPoolExecutor(max_workers=len(nodes)) as executor:
        for node in nodes:
            executor.submit(perform, node)

    check(nodes)


def stop(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def perform(node):
        ssh = connect(node)
        exec_cmd(ssh, "cd %s && sudo -S -p '' docker-compose stop " %
                 (node["path"]), node["passwd"],)

    with futures.ThreadPoolExecutor(max_workers=len(nodes)) as executor:
        for node in nodes:
            executor.submit(perform, node)

    check(nodes)


def remove(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def perform(node):
        ssh = connect(node)
        exec_cmd(ssh, "cd %s && sudo -S -p '' docker-compose down" %
                 (node["path"]), node["passwd"],)
        exec_cmd(ssh, "sudo -S -p '' rm -rf " + node["path"], node["passwd"])
        # exec_cmd(ssh, "sudo -S -p '' cp /etc/crontab.bak /etc/crontab", node["passwd"])

    with futures.ThreadPoolExecutor(max_workers=20) as executor:
        for node in nodes:
            print("remove %s" % (node["name"]))
            executor.submit(perform, node)


def status(config_file):
    with open(config_file) as infile:
        nodes = json.load(infile)
        check(nodes)


def update(config_file, tag):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def perform(node):
        ssh = connect(node)
        update_docker_compose_config(node, tag)
        # backup
        exec_cmd(ssh, "sudo -S -p '' mv " + node["path"] + "/docker-compose.yaml " + node["path"] + "/docker-compose.yaml.bak",
                 node["passwd"],
                 )
        # upload platon
        upload(ssh, "./deploy-docker/%s/docker-compose.yaml" %
               (node["name"]), "/tmp/%s-docker-compose.yaml" % (node["name"]),)
        # replace
        exec_cmd(ssh,  "sudo -S -p '' cp /tmp/%s-docker-compose.yaml %s/docker-compose.yaml" %
                 (node["name"], node["path"]), node["passwd"],)
        exec_cmd(
            ssh, "sudo -S -p '' docker pull {}:{}".format(node["registry"], tag), node["passwd"],)
        # update
        exec_cmd(ssh, "cd %s && sudo -S -p '' docker-compose up -d --force-recreate" %
                 (node["path"]), node["passwd"],)

    with futures.ThreadPoolExecutor(max_workers=10) as executor:
        for node in nodes:
            print("update %s" % (node["name"]))
            executor.submit(perform, node)

    check(nodes)


def block_number(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def get_block_number(nodes):
        for node in nodes:
            try:
                if not need_point:
                    cmd = "%s attach ws://%s:%s --exec platon.blockNumber" % (
                        PLATON_BIN,
                        node["host"],
                        node["ws_port"],
                    )
                else:
                    cmd = "%s attach ws://%s:%s --exec 'platon.blockNumber'" % (
                        PLATON_BIN,
                        node["host"],
                        node["ws_port"],
                    )
                bn = run(cmd)
                print(
                    "node: {:<0}@{:<30} blockNumber: {:<0}".format(
                        node["name"], node["host"].strip(), bn.strip()
                    )
                )
            except Exception as e:
                print(e)

    try:
        while True:
            # print('---------------------------------------------------------')
            get_block_number(nodes)
            print("---------------------------------------------------------")
            time.sleep(5)
    except KeyboardInterrupt:
        pass


def block_number_r(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def get_block_number(node):
        # for node in nodes:
        ssh = connect(node)
        cmd = "sudo -S -p '' docker exec {} platon attach ws://127.0.0.1:{} --exec platon.blockNumber".format(
            node["name"], node["ws_port"]
        )
        lines = exec_cmd_return(ssh, cmd, node["passwd"])
        print(
            "node: {:<0}@{:<30} blockNumber: {:<0}".format(
                node["name"], node["host"].strip(), lines[0].strip()
            )
        )

    try:
        while True:
            # print('---------------------------------------------------------')
            with futures.ThreadPoolExecutor(max_workers=10) as executor:
                for node in nodes:
                    executor.submit(get_block_number, node)
            # get_block_number(nodes)
            print("---------------------------------------------------------")
            time.sleep(5)
    except KeyboardInterrupt:
        pass


def net_r(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def get_net(node):
        # for node in nodes:
        ssh = connect(node)
        cmd = "sudo -S -p '' docker exec {} platon attach ws://127.0.0.1:{} --exec net.peerCount".format(
            node["name"], node["ws_port"]
        )
        lines = exec_cmd_return(ssh, cmd, node["passwd"])
        print("node: {:<0}@{:<30} peers: {:<0}".format(
            node["name"], node["host"].strip(), lines[0].strip()))

    with futures.ThreadPoolExecutor(max_workers=10) as executor:
        for node in nodes:
            executor.submit(get_net, node)


def debug(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def status(nodes):
        ls = []
        for node in nodes:
            # try:
            if need_point:
                cmd = "%s attach http://%s:%s --exec 'debug.consensusStatus()'" % (
                    PLATON_BIN,
                    node["host"],
                    node["rpc_port"],
                )
            else:
                cmd = "%s attach http://%s:%s --exec debug.consensusStatus()" % (
                    PLATON_BIN,
                    node["host"],
                    node["rpc_port"],
                )
            bn = run(cmd)
            objs = json.loads(bn.strip())
            ls.append(
                {
                    "node": "{}:{}".format(node["host"], node["rpc_port"]),
                    "name": node["name"],
                    "status": json.loads(objs),
                }
            )
            # except:
            #    print('get debug info fail')
            #    pass
        fp = open("debug.log", "w")
        json.dump(ls, fp, indent=2)
        fp.close()
    status(nodes)


# def date():
#     def perform(node):
#         ssh = connect(node)
#         bn = exec_cmd_return(ssh, "date", None)
#         print("node: %s(%s)\t\t%s" % (node["name"], node["host"], bn[0].strip()))
#
#     with futures.ThreadPoolExecutor(max_workers=len(config.nodes)) as executor:
#         for node in config.nodes:
#             executor.submit(perform, node)


def block_interval(config_file, start_bn):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()
    node = nodes[0]
    map_nodes = dict()
    for node in nodes:
        path = "./deploy-docker/{}/data/addr".format(node["name"])
        fp = open(path)
        addr = fp.read()
        fp.close()

        map_nodes[addr.lower()] = node

    fp = open("./long_interval.txt", "w")

    def parse_timestamp(s):
        for line in s.splitlines():
            if "timestamp: " in line:
                line = line.strip("timestamp: ")
                tm = line.strip(",")
                return int(tm.strip())

    start = int(start_bn)
    if not need_point:
        pre_cmd = "%s attach http://%s:%s --exec platon.getBlock(%d)" % (
            PLATON_BIN, node["host"], node["rpc_port"], start)
    else:
        pre_cmd = "%s attach http://%s:%s --exec 'platon.getBlock(%d)'" % (
            PLATON_BIN, node["host"], node["rpc_port"], start)
    pre_block = run(pre_cmd)

    pre_timestamp = parse_timestamp(pre_block)

    start = start + 1
    while True:
        try:
            if not need_point:
                cmd = "%s --verbosity 0 attach http://%s:%s --exec platon.blockNumber" % (
                    PLATON_BIN, node["host"], node["rpc_port"])
            else:
                cmd = "%s --verbosity 0 attach http://%s:%s --exec 'platon.blockNumber'" % (
                    PLATON_BIN, node["host"], node["rpc_port"])
            block_number = run(cmd).strip()

            for i in range(start, int(block_number)):
                if not need_point:
                    next_cmd = "%s --verbosity 0 attach http://%s:%s --exec platon.getBlock(%s)" % (
                        PLATON_BIN, node["host"], node["rpc_port"], i)
                else:
                    next_cmd = "%s --verbosity 0 attach http://%s:%s --exec 'platon.getBlock(%s)'" % (
                        PLATON_BIN, node["host"], node["rpc_port"], i)
                next_block = run(next_cmd)

                def parse_miner(s):
                    for line in s.splitlines():
                        if "miner: " in line:
                            line = line.strip("miner: ")
                            line = line.lstrip('"')
                            line = line.strip(",")
                            line = line.rstrip('"')
                            return line.strip()

                try:
                    next_timestamp = parse_timestamp(next_block)
                    interval = next_timestamp - pre_timestamp
                    pre_timestamp = next_timestamp
                    miner = parse_miner(next_block)
                    n = map_nodes[miner]
                    print(
                        "block: {:<10} interval: {:<10} miner:{:<10}({}:{})".format(
                            i, interval, n["name"], n["host"], n["rpc_port"]
                        )
                    )
                    if interval > 2000:
                        fp.write(
                            "%d block: %s \t interval: %d\n"
                            % (time.time(), i, interval)
                        )
                except Exception as e:
                    print(e)

            start = int(block_number)
            fp.flush()
        except KeyboardInterrupt:
            fp.close()
            return


def block_interval_simple(config_file, start_bn):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    node = nodes[0]
    print(node)

    def parse_timestamp(s):
        for line in s.splitlines():
            if "timestamp: " in line:
                line = line.strip("timestamp: ")
                tm = line.strip(",")
                return int(tm.strip())

    start = int(start_bn)
    if not need_point:
        pre_cmd = "%s --verbosity 0 attach http://%s:%s --exec platon.getBlock(%d)" % (
            PLATON_BIN, node["host"], node["rpc_port"], start)
    else:
        pre_cmd = "%s --verbosity 0 attach http://%s:%s --exec 'platon.getBlock(%d)'" % (
            PLATON_BIN, node["host"], node["rpc_port"], start)
    pre_block = run(pre_cmd)
    pre_timestamp = parse_timestamp(pre_block)
    start = start + 1
    while True:
        try:
            if not need_point:
                cmd = "%s --verbosity 0 attach http://%s:%s --exec platon.blockNumber" % (
                    PLATON_BIN, node["host"], node["rpc_port"])
            else:
                cmd = "%s --verbosity 0 attach http://%s:%s --exec 'platon.blockNumber'" % (
                    PLATON_BIN, node["host"], node["rpc_port"])

            block_number = run(cmd).strip()

            for i in range(start, int(block_number)):
                if not need_point:
                    next_cmd = "%s --verbosity 0 attach http://%s:%s --exec platon.getBlock(%s)" % (
                        PLATON_BIN, node["host"], node["rpc_port"], i)
                else:
                    next_cmd = "%s --verbosity 0 attach http://%s:%s --exec 'platon.getBlock(%s)'" % (
                        PLATON_BIN, node["host"], node["rpc_port"], i)

                next_block = run(next_cmd)

                def parse_miner(s):
                    for line in s.splitlines():
                        if "miner: " in line:
                            line = line.strip("miner: ")
                            line = line.lstrip('"')
                            line = line.strip(",")
                            line = line.rstrip('"')
                            return line.strip()

                try:
                    next_timestamp = parse_timestamp(next_block)
                    interval = next_timestamp - pre_timestamp
                    pre_timestamp = next_timestamp
                    print("block: {:<10} interval: {:<10}".format(i, interval))
                except Exception as e:
                    print(e)

            start = int(block_number)
            time.sleep(1)
        except KeyboardInterrupt:
            return


def check_docker(arg):
    fp = open(arg)
    nodes = json.load(fp)
    fp.close()

    for node in nodes:
        ssh = connect(node)
        bn = exec_cmd_return(
            ssh, "sudo -S -p '' docker --version", node["passwd"])
        bn1 = exec_cmd_return(
            ssh, "sudo -S -p '' docker-compose --version", node["passwd"]
        )
        # exec_cmd(ssh, "sudo -S -p '' gpasswd -a ${USER} docker", node["passwd"])
        print("node(%s)      \t%s\t%s" % (node["host"], bn, bn1))


def net_status(config_file):
    with open(config_file) as infile:
        nodes = json.load(infile)
        for node in nodes:
            try:
                if not need_point:
                    cmd = "%s --verbosity 0 attach ws://%s:%s --exec net.peerCount" % (
                        PLATON_BIN, node["host"], node["ws_port"])
                else:
                    cmd = "%s --verbosity 0 attach ws://%s:%s --exec 'net.peerCount'" % (
                        PLATON_BIN, node["host"], node["ws_port"])
                bn = run(cmd)
                print(
                    "%s(%s:%s)               \t%s"
                    % (node["name"], node["host"], node["rpc_port"], bn.strip())
                )
            except Exception as e:
                print(e)


def get_accounts(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    map_nodes = dict()
    for n in nodes:
        try:
            if not need_point:
                cmd = "%s --verbosity 0 attach http://%s:%s --exec platon.accounts" % (
                    PLATON_BIN, n["host"], n["rpc_port"])
            else:
                cmd = "%s --verbosity 0 attach http://%s:%s --exec 'platon.accounts'" % (
                    PLATON_BIN, n["host"], n["rpc_port"])
            addr = (run(cmd)
                    .strip()
                    .strip("[")
                    .strip("]")
                    .strip('"')
                    )
            print(addr)
            map_nodes[addr] = n
        except Exception as e:
            print(e)
            continue

    fp = open("./accounts.txt", "w")
    json.dump(map_nodes, fp)
    fp.close()


def prepare_qc(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    fp = open("prepare.log", "w")

    for n in nodes:
        try:
            if not need_point:
                cmd = "%s --verbosity 0 attach http://%s:%s --exec platon.getPrepareQC(181137)" % (
                    PLATON_BIN, n["host"], n["rpc_port"])
            else:
                cmd = "%s --verbosity 0 attach http://%s:%s --exec 'platon.getPrepareQC(181137)'" % (
                    PLATON_BIN, n["host"], n["rpc_port"])
            bn = run(cmd).strip()
            print(bn)
            fp.write("{}@{}:{}\n".format(n["name"], n["host"], n["rpc_port"]))
            fp.write(bn)
            fp.write("\n")
        except Exception as e:
            print(e)
            continue

    fp.flush()
    fp.close()


def df(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    fp = open("./df_status.log", "w")

    for node in nodes:
        ssh = connect(node)
        out = exec_cmd_return(ssh, "df -h", None)
        fp.write(
            "-----------------------------------------------------------------\n")
        fp.write("{}:\n".format(node["host"]))
        fp.writelines(out)
        fp.flush()
    fp.close()


def restart_docker(config_file):
    fp = open(config_file)
    nodes = json.load(fp)
    fp.close()

    def perform(node):
        ssh = connect(node)
        exec_cmd(ssh, "sudo service docker restart", node["passwd"])

    with futures.ThreadPoolExecutor(max_workers=10) as executor:
        for node in nodes:
            executor.submit(perform, node)


# def stop_batch(config_file):
#     fp = open(config_file)
#     nodes = json.load(fp)
#     fp.close()

#     def perform(node):
#         ssh = connect(node)
#         exec_cmd(
#             ssh, "sudo -S -p '' docker stop {}-batch".format(node["name"]), node["passwd"],)

#     # with futures.ThreadPoolExecutor(max_workers=10) as executor:
#     for node in nodes:
#         perform(node)
        # executor.submit(perform, node)


if __name__ == "__main__":

    def do(cmd):
        return {
            "install": deploy,
            "install_docker": install_docker,
            "remove": remove,
            "start": start,
            "stop": stop,
            "status": status,
            "update": update,
            "block_number": block_number,
            "block_number_r": block_number_r,
            "debug": debug,
            # "date": date,
            "block_interval": block_interval,
            "block_interval_s": block_interval_simple,
            "check_docker": check_docker,
            "net": net_status,
            "net_r": net_r,
            "accounts": get_accounts,
            "prepare_qc": prepare_qc,
            "df": df,
            "restart_docker": restart_docker,
            # "stop_batch": stop_batch,
        }[cmd]

    if len(sys.argv) > 2:
        do(sys.argv[1])(*sys.argv[2:])
    else:
        do(sys.argv)()
