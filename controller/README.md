# platon-deploy

### 生成配置文件
1.必须ip.txt文件，文件格式如下
```
192.168.9.201
192.168.9.202
192.168.9.203
192.168.9.204
10.10.8.209
```
2.修改...文件中对应的信息
```
num_accounts = 40
num_consensus = 4
total = 7
# user = "username"
# passwd = "passwd"
user = "user"
passwd = "passwd"
registry = "shinnng/platon-test"
```
3.执行命令
```shell
python gen_config.py gen_deploy <ip.txt> <node_config.json>
```
那么，部署配置文件以及压测配置文件已经生成成功。

4.如果部署过程遇到问题，导致nodekey等信息未正常写入到配置文件中，那么可以执行
```shell
python gen_config.py update_key <node_config.json>
```

5.如果需要修改每个节点的压测账号数，可以执行
```shell
python gen_config.py update_account <node_config.json> <num>
```

### 部署platon到服务器中

1.批量安装docker

```shell
python deploy-docker-remote.py install_docker <node_config.json>
```
2.部署节点
```shell
python deploy-docker-remote.py install <node_config.json> <docker-registry-tag> <program_version>
```
那么节点部署成功,并且质押成功

3.检查节点出块情况
```shell 
python deploy-docker-remote.py block_number <node_config.json>
```
4.清除环境
```shell 
python deploy-docker-remote.py remove <node_config.json>
```
### 批量质押

### 执行压测
```shell
python deploy-batch.py deploy <node_config.json> 
```



### 压测工具现有的缺陷以及修改思路

#### 1.缺陷
+ platon进程的容器与压测程序的容器由一个docker-compose.yml控制，不方便随时启停压测程序

+ 压测程序启动后，开始质押，而验证人的切换大约是4小时左右开始，现在只能在工具中设置4个小时左右的延迟

+ 质押程序platon版本号写死，每个大版本需要修改版本号重现编译压测程序

+ 现在不适合做转账交易之后，继续在原有环境做委托交易

#### 2. 修改思路
+ 拆分docker-compose.yml,platon进程与压测进程docker-compose独立，方便随时控制压测，也方便压测程序用于其他环境

+ 拆分质押，质押与压测不在由同一个进程控制，部署节点后，另外调用工具执行批量质押，并生成质押文件，等质押成功一定时间后启动压测程序

+ 质押版本参数化

+ 独立之后，大约会有3个脚本用于控制各项任务，gen-config.py 用于生成部署所需要的配置文件和压测所需要的配置文件；deploy-docker-remote.py 用于控制环境（安装docker，部署节点，清空环境，检查状态等）；deploy-batch.py 用于执行压测、关闭压测和批量质押节点

#### 3.修改点
+ platon-deploy：需要拆分脚本，按修改思路，实现对应的脚本以及功能

+ platon-test-toolkits：增加批量质押功能，去除原有的每次启动压测时，判断是否需要质押的逻辑，压测程序不在参与质押，增加命令行参数programVersion
