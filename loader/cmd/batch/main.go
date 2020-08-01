package main

import (
	"crypto/ecdsa"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
)

const (
	SIGN_PASSPHRASE = `88888888`
)

var ChainId int64 = 101
var toAccount AddrKeyList

type AddrKey struct {
	Address string `json:"address"`
	Key     string `json:"private_key"`
}
type AddrKeyList []AddrKey

type Node struct {
	Host    string `json:"host"`
	WSPort  string `json:"ws_port"`
	RpcPort string `json:"rpc_port"`
}
type NodeList []Node

type Account struct {
	address    common.Address
	privateKey *ecdsa.PrivateKey

	lastSent time.Time
	interval time.Duration
	nonce    uint64
	transfer bool

	sendCh    chan *Account
	receiptCh chan *ReceiptTask

	transfers         int
	delegates         int
	withdrewDelegates int
	delegate          bool
	withdrewDelegate  bool
}
type AccountList []*Account

type ReceiptTask struct {
	account *Account
	hash    common.Hash
	lastGot time.Time

	sendCh chan *Account
}

type BatchProcessor interface {
	Start()
	Stop()
	Pause()
	Resume()
	SetSendInterval(d time.Duration)
}

func main() {
	log.SetFlags(0)
	runtime.GOMAXPROCS(runtime.NumCPU())
	cmdFlag := flag.String("cmd", "transfer", "Batch send transaction type(transfer,staking,side_transfer,side_delegate,side_mix,side_random,rally3)")
	accountsFlag := flag.String("accounts", "", "A json file store account's private key and address")
	nodeCfg := flag.String("node_cfg", "", "Node list config file")
	intervalMs := flag.Int("interval_ms", 100, "Send transaction interval")
	count := flag.Int("count", 1, "How many accounts to send transactions")
	idxFlag := flag.Int("idx", 0, "Index of accounts")
	urlFlag := flag.String("url", "ws://127.0.0.1:8806", "platon node's RPC endpoint")
	nodeKeyFlag := flag.String("nodekey", "", "The platon node's private key, for create staking")
	blsKeyFlag := flag.String("blskey", "", "The platon node's bls public key, for create staking")
	nodeNameFlag := flag.String("nm", "", "The platon node's name")
	onlyConsensusFlag := flag.Bool("only-consensus", false, "Only send transaction to consensus node")
	stakingFlag := flag.Bool("staking", false, "Should create staking")
	delegateFlag := flag.Bool("delegate", false, "Should delegate")
	randCountFlag := flag.Int("rand_count", 10000, "The maximum generate random addresses count")
	chanIdFlag := flag.Int64("chain_id", 101, "The identity of chain")
	randAccountsFlag := flag.String("rand_accounts", "", "A file store account's address")
	randIdxFlag := flag.Int("rand_idx", 0, "Index of random accounts")
	delegateNodes := flag.String("delegate_nodes", "", "A file store a list of node ID for delegate")
	programVersionFlag := flag.Int64("program_version", 2562, "create staking program version")
	privateKeyFlag := flag.String("private_key", "", "create staking address private key")
	// toAccountFileFlag := flag.String("to_account", "/data/to_keys.json", "addr for random transfer")
	flag.Parse()

	ChainId = *chanIdFlag
	fmt.Printf("The identify of chain is %d\n", ChainId)

	var bp BatchProcessor

	accounts := parseAccountFile(*accountsFlag, *idxFlag, *count, *intervalMs)
	// toAccount = parseToAccountFile(*toAccountFileFlag)

	switch *cmdFlag {
	case "transfer":
		hosts := parseHosts(*nodeCfg)
		bp = NewBatchProcess(accounts, hosts)
	case "staking":
		hosts := parseHosts(*nodeCfg)
		bp = NewStakingBatchProcess(accounts, hosts, *nodeKeyFlag)
	case "side_transfer":
		bp = NewSideBatch(
			NewBatchProcess(accounts, []string{*urlFlag}),
			accounts[0],
			*urlFlag,
			*nodeKeyFlag,
			*blsKeyFlag,
			*nodeNameFlag,
			*onlyConsensusFlag,
			*stakingFlag)
	case "side_delegate":
		bp = NewSideBatch(
			NewStakingBatchProcess(accounts, []string{*urlFlag}, *nodeKeyFlag),
			accounts[0],
			*urlFlag,
			*nodeKeyFlag,
			*blsKeyFlag,
			*nodeNameFlag,
			*onlyConsensusFlag,
			*stakingFlag)
	case "side_mix":
		bp = NewSideBatch(
			NewBatchMixProcess(accounts, []string{*urlFlag}, *nodeKeyFlag, *delegateFlag),
			accounts[0],
			*urlFlag,
			*nodeKeyFlag,
			*blsKeyFlag,
			*nodeNameFlag,
			*onlyConsensusFlag,
			*stakingFlag)

	case "side_random":
		bp = NewSideBatch(
			NewBatchRandomProcess(accounts, []string{*urlFlag}, *nodeKeyFlag, *delegateFlag, *randCountFlag, *randAccountsFlag, *randIdxFlag),
			accounts[0],
			*urlFlag,
			*nodeKeyFlag,
			*blsKeyFlag,
			*nodeNameFlag,
			*onlyConsensusFlag,
			*stakingFlag)
	case "rally3":
		nodes := parseNodes(*delegateNodes)
		bp = NewSideBatch(
			NewRally3Process(accounts,
				[]string{*urlFlag},
				nodes,
				*randCountFlag,
				*randAccountsFlag,
				*randIdxFlag),
			accounts[0],
			*urlFlag,
			*nodeKeyFlag,
			*blsKeyFlag,
			*nodeNameFlag,
			*onlyConsensusFlag,
			*stakingFlag)
	case "batch_staking":
		bp = NewBatchStaking(*nodeKeyFlag, *blsKeyFlag, *nodeNameFlag, *privateKeyFlag, *urlFlag, uint32(*programVersionFlag))
	default:
		log.Fatalf("Unexpected cmd %s", *cmdFlag)
		return
	}

	bp.Start()

	sigs := make(chan os.Signal, 1)
	done := make(chan struct{}, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		done <- struct{}{}
	}()
	<-done

	bp.Stop()
}

func parseAccountFile(accountFile string, idx, count, interval int) AccountList {
	var addrKeyList AddrKeyList
	b, err := ioutil.ReadFile(accountFile)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, &addrKeyList)
	if err != nil {
		panic(err)
	}

	if len(addrKeyList) > count {
		addrKeyList = addrKeyList[idx : idx+count]
	}
	accounts := make(AccountList, 0)
	for _, ak := range addrKeyList {
		pk, err := crypto.HexToECDSA(ak.Key)
		if err != nil {
			fmt.Printf("error private key: %s, err: %v\n", ak.Key, pk)
			continue
		}
		bech32Addr, err := common.Bech32ToAddress(ak.Address)
		if err != nil {
			panic(err.Error())
		}
		accounts = append(accounts, &Account{
			address:    bech32Addr,
			privateKey: pk,
			lastSent:   time.Now(),
			interval:   time.Duration(interval) * time.Millisecond,
			nonce:      0,
		})
	}
	return accounts
}

func parseToAccountFile(accountFile string) AddrKeyList {
	var addrKeyList AddrKeyList
	b, err := ioutil.ReadFile(accountFile)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, &addrKeyList)
	if err != nil {
		panic(err)
	}

	return addrKeyList
}

func parseHosts(nodeCfg string) []string {
	buf, err := ioutil.ReadFile(nodeCfg)
	if err != nil {
		panic(err)
	}

	var nl NodeList
	if err = json.Unmarshal(buf, &nl); err != nil {
		panic(err)
	}

	nodeList := make([]string, 0)
	for _, n := range nl {
		nodeList = append(nodeList, fmt.Sprintf("ws://%s:%s", n.Host, n.WSPort))
	}
	return nodeList
}

type ValidateNode struct {
	NodeID  discover.NodeID `json:"nodeID"`
	Staking bool            `json:"staking"`
}

type Validators struct {
	ValidateNodes []*ValidateNode `json:"validateNodes"`
}

func parseValidatorCfg(cfg string) []discover.NodeID {
	buf, err := ioutil.ReadFile(cfg)
	if err != nil {
		panic(err)
	}

	var vds Validators
	if err = json.Unmarshal(buf, &vds); err != nil {
		panic(err)
	}

	nodes := make([]discover.NodeID, 0)
	for _, node := range vds.ValidateNodes {
		if node.Staking {
			nodes = append(nodes, node.NodeID)
		}
	}
	return nodes
}

func parseNodes(file string) []discover.NodeID {
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}

	nl := strings.Split(string(buf), "\n")
	nodes := make([]discover.NodeID, 0)
	for _, n := range nl {
		log.Printf("node: %s\n", n)
		if len(n) == 0 {
			continue
		}
		var node discover.NodeID
		err := node.UnmarshalText([]byte(n))
		if err != nil {
			panic(err)
		}
		nodes = append(nodes, node)
	}
	return nodes
}
