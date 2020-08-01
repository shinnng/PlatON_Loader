package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"sync/atomic"
	"time"

	ethereum "github.com/PlatONnetwork/PlatON-Go"
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/shinnng/platon-test-toolkits/util"
)

type Rally3Process struct {
	accounts AccountList
	hosts    []string
	nodes    []*DelegateNode

	sendCh chan *Account
	waitCh chan *ReceiptTask

	exit chan struct{}

	sents             int32
	delegates         int32
	withdrewDelegates int32
	receipts          int32

	sendInterval atomic.Value

	nextAddress uint64
	randomAddrs []*RandAccount

	maxSendTxns int

	BatchProcessor
}

type DelegateNode struct {
	NodeID     discover.NodeID
	StakingNum uint64
}

func NewRally3Process(
	accounts AccountList,
	hosts []string,
	nodes []discover.NodeID,
	rndCnt int,
	actFile string,
	rndIdx int) *Rally3Process {
	bp := &Rally3Process{
		accounts:          accounts,
		hosts:             hosts,
		sendCh:            make(chan *Account, 1000),
		waitCh:            make(chan *ReceiptTask, 1000),
		exit:              make(chan struct{}),
		sents:             0,
		delegates:         0,
		withdrewDelegates: 0,
		maxSendTxns:       10,
	}
	bp.sendInterval.Store(50 * time.Millisecond)

	for _, node := range nodes {
		bp.nodes = append(bp.nodes, &DelegateNode{
			NodeID:     node,
			StakingNum: 0,
		})
	}

	buf, err := ioutil.ReadFile(actFile)
	if err != nil {
		panic(err)
	}

	if err := json.Unmarshal(buf, &bp.randomAddrs); err != nil {
		panic(err)
	}

	total := len(bp.randomAddrs)
	if rndIdx+rndCnt <= total {
		bp.randomAddrs = bp.randomAddrs[rndIdx : rndIdx+rndCnt]
	} else {
		if rndIdx < total {
			bp.randomAddrs = bp.randomAddrs[total-rndCnt:]
		} else {
			bp.randomAddrs = bp.randomAddrs[:rndCnt]
		}
	}
	return bp
}

func (bp *Rally3Process) Start() {
	go bp.report()

	for _, host := range bp.hosts {
		go bp.perform(host)
	}

	for _, act := range bp.accounts {
		bp.sendCh <- act
		time.Sleep(100 * time.Millisecond)
	}
	log.Println("start success")
}

func (bp *Rally3Process) Stop() {
	close(bp.exit)
}

func (bp *Rally3Process) Pause() {
	panic("Not implement")
}

func (bp *Rally3Process) Resume() {
	panic("Not implement")
}

func (bp *Rally3Process) SetSendInterval(time.Duration) {
	panic("Not implement")
}

func (bp *Rally3Process) report() {
	timer := time.NewTicker(time.Second)
	for {
		select {
		case <-timer.C:
			log.Printf("Send %d/s Delegate %d/s WithdrewDelegate %d/s Receipt %d/s",
				atomic.SwapInt32(&bp.sents, 0),
				atomic.SwapInt32(&bp.delegates, 0),
				atomic.SwapInt32(&bp.withdrewDelegates, 0),
				atomic.SwapInt32(&bp.receipts, 0),
			)
		case <-bp.exit:
			return
		}
	}
}

func (bp *Rally3Process) perform(host string) {
	client, err := ethclient.Dial(host)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	bp.getAllCandidateInfo(client)

	sentCh := make(chan *Account, 1000)
	receiptCh := make(chan *ReceiptTask, 1000)

	for {
		select {
		case act := <-bp.sendCh:
			if act.sendCh == nil {
				act.sendCh = sentCh
				act.receiptCh = receiptCh
				act.delegate = true
				act.transfer = true
			}

			bp.sendTransaction(client, act)
		case act := <-sentCh:
			if act.transfers < bp.maxSendTxns {
				bp.sendTransaction(client, act)
			} else if act.delegate {
				act.transfers = 0
				bp.sendDelegate(client, act)
				if act.delegates == len(bp.nodes) {
					act.delegates = 0
					act.delegate = false
					act.withdrewDelegates = 0
					act.withdrewDelegate = true
				}
			} else {
				act.transfers = 0
				bp.sendWithdrewDelegate(client, act)
				if act.withdrewDelegates == len(bp.nodes) {
					act.withdrewDelegates = 0
					act.withdrewDelegate = false
					act.delegates = 0
					act.delegate = true
				}
			}
		case task := <-receiptCh:
			bp.getTransactionReceipt(client, task)
		case <-bp.exit:
			return
		}
	}
}

func (bp *Rally3Process) nonceAt(client *ethclient.Client, addr common.Address) uint64 {
	var blockNumber *big.Int
	nonce, err := client.NonceAt(context.Background(), addr, blockNumber)
	if err != nil {
		log.Printf("Get nonce error, addr: %s, err:%v\n", addr, err)
		return 0
	}
	return nonce
}

func (bp *Rally3Process) randomAddress() common.Address {
	addr := bp.randomAddrs[bp.nextAddress%uint64(len(bp.randomAddrs))]
	bp.nextAddress++
	return addr.Address
}

func (bp *Rally3Process) sendTransaction(client *ethclient.Client, account *Account) {
	cnt := 2
	to := bp.randomAddress()
	signer := types.NewEIP155Signer(big.NewInt(ChainId))
	nonce := bp.nonceAt(client, account.address)
	for i := 0; i < cnt; i++ {
		tx := types.NewTransaction(
			nonce,
			to,
			big.NewInt(1),
			21000,
			big.NewInt(500000000000),
			nil)
		signedTx, err := types.SignTx(tx, signer, account.privateKey)
		if err != nil {
			fmt.Printf("sign tx error: %v\n", err)
			go func() {
				<-time.After(account.interval)
				bp.sendCh <- account
			}()
			return
		}

		err = client.SendTransaction(context.Background(), signedTx)
		account.lastSent = time.Now()
		if err != nil {
			fmt.Printf("send transaction error: %v\n", err)
			go func() {
				<-time.After(account.interval)
				account.sendCh <- account
			}()
			return
		}
		account.nonce = nonce
		atomic.AddInt32(&bp.sents, 1)

		nonce += 1
		account.transfers += 1

		if i < cnt-1 {
			continue
		}

		go func() {
			<-time.After(600 * time.Millisecond)
			account.receiptCh <- &ReceiptTask{
				account: account,
				hash:    signedTx.Hash(),
			}
		}()
	}
}

func (bp *Rally3Process) getNode(idx int) *DelegateNode {
	return bp.nodes[idx%len(bp.nodes)]
}

func (bp *Rally3Process) sendDelegate(client *ethclient.Client, account *Account) {
	node := bp.getNode(account.delegates)
	buf, _ := util.Delegate(node.NodeID, "10000000000000000000")

	signer := types.NewEIP155Signer(big.NewInt(ChainId))
	nonce := bp.nonceAt(client, account.address)

	tx, err := types.SignTx(
		types.NewTransaction(
			nonce,
			contractAddr,
			big.NewInt(0),
			103496,
			big.NewInt(500000000000),
			buf),
		signer,
		account.privateKey)
	if err != nil {
		fmt.Printf("sign tx error %v\n", err)
		bp.sendCh <- account
		return
	}

	err = client.SendTransaction(context.Background(), tx)
	account.lastSent = time.Now()
	if err != nil {
		fmt.Printf("Send delegate transaction error %v\n", err)
		go func() {
			<-time.After(bp.sendInterval.Load().(time.Duration))
			account.sendCh <- account
		}()
		return
	}
	account.delegates += 1
	atomic.AddInt32(&bp.delegates, 1)
	go func() {
		<-time.After(600 * time.Millisecond)
		account.receiptCh <- &ReceiptTask{
			account: account,
			hash:    tx.Hash(),
		}
	}()
}

func (bp *Rally3Process) sendWithdrewDelegate(client *ethclient.Client, account *Account) {
	node := bp.getNode(account.withdrewDelegates)
	buf, _ := util.WithdrewDelegate(node.NodeID, node.StakingNum, "10000000000000000000")

	signer := types.NewEIP155Signer(big.NewInt(ChainId))
	nonce := bp.nonceAt(client, account.address)

	tx, err := types.SignTx(
		types.NewTransaction(
			nonce,
			contractAddr,
			big.NewInt(0),
			103496,
			big.NewInt(500000000000),
			buf),
		signer,
		account.privateKey)
	if err != nil {
		fmt.Printf("sign tx error %v\n", err)
		bp.sendCh <- account
		return
	}

	err = client.SendTransaction(context.Background(), tx)
	account.lastSent = time.Now()
	if err != nil {
		fmt.Printf("Send delegate transaction error %v\n", err)
		go func() {
			<-time.After(bp.sendInterval.Load().(time.Duration))
			account.sendCh <- account
		}()
		return
	}
	account.withdrewDelegates += 1
	atomic.AddInt32(&bp.withdrewDelegates, 1)
	go func() {
		<-time.After(600 * time.Millisecond)
		account.receiptCh <- &ReceiptTask{
			account: account,
			hash:    tx.Hash(),
		}
	}()
}

func (bp *Rally3Process) getCandidateInfo(client *ethclient.Client, nodeID discover.NodeID, account *Account) uint64 {
	buf, _ := util.GetCandiateInfo(nodeID)

	msg := ethereum.CallMsg{
		From:     account.address,
		To:       &contractAddr,
		Gas:      103496,
		GasPrice: big.NewInt(500000000000),
		Data:     buf,
	}

	var blockNumber *big.Int
	res, err := client.CallContract(context.Background(), msg, blockNumber)
	if err != nil {
		log.Printf("Get candiate info error: %s", err)
		return 0
	}
	log.Printf("can: %s\n", res)
	type Can struct {
		StakingBlockNum uint64 `json:"StakingBlockNum"`
	}

	type CanRes struct {
		Code int `json:"Code"`
		Ret  Can `json:"Ret"`
	}

	var can CanRes
	if err := json.Unmarshal(res, &can); err != nil {
		log.Printf("Parse candiate result error: %s", err)
		return 0
	}
	log.Printf("Node: %s StakingBlockNum: %d\n", nodeID, can.Ret.StakingBlockNum)
	return can.Ret.StakingBlockNum
}

func (bp *Rally3Process) getAllCandidateInfo(client *ethclient.Client) {
	for _, node := range bp.nodes {
		node.StakingNum = bp.getCandidateInfo(client, node.NodeID, bp.accounts[0])
	}
}

func (bp *Rally3Process) getTransactionReceipt(client *ethclient.Client, task *ReceiptTask) {
	_, err := client.TransactionReceipt(context.Background(), task.hash)
	if err != nil {
		if time.Since(task.account.lastSent) >= task.account.interval {
			log.Printf("get receipt timeout, address:%s, hash: %s, sendTime: %v, now: %v\n",
				task.account.address.String(), task.hash.String(), task.account.lastSent, time.Now())
			task.account.sendCh <- task.account
			return
		}
		go func() {
			<-time.After(300 * time.Millisecond)
			task.account.receiptCh <- task
		}()
		return
	}

	atomic.AddInt32(&bp.receipts, 1)

	go func() {
		<-time.After(bp.sendInterval.Load().(time.Duration))
		task.account.sendCh <- task.account
	}()
}
