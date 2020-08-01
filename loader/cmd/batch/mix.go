package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
	"github.com/shinnng/platon-test-toolkits/util"
)

type BatchMixProcess struct {
	accounts AccountList
	hosts    []string

	sendCh chan *Account
	waitCh chan *ReceiptTask

	exit chan struct{}

	sents    int32
	receipts int32

	sendInterval atomic.Value

	paused bool
	lock   sync.Mutex
	cond   *sync.Cond

	BatchProcessor

	stub     *util.StakingStub
	delegate bool
}

const maxSendTransferTxns = 3

func NewBatchMixProcess(accounts AccountList, hosts []string, nodeKey string, delegate bool) BatchProcessor {
	bp := &BatchMixProcess{
		accounts: accounts,
		hosts:    hosts,
		sendCh:   make(chan *Account, 1000),
		waitCh:   make(chan *ReceiptTask, 1000),
		exit:     make(chan struct{}),
		sents:    0,
		receipts: 0,
		paused:   false,
		stub:     util.NewStakingStub(nodeKey),
		delegate: delegate,
	}
	bp.cond = sync.NewCond(&bp.lock)
	bp.sendInterval.Store(50 * time.Millisecond)
	log.Printf("delegate: %v bp.delegate: %v\n", delegate, bp.delegate)
	return bp
}

func (bp *BatchMixProcess) Start() {
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

func (bp *BatchMixProcess) Stop() {
	close(bp.exit)
}

func (bp *BatchMixProcess) Pause() {
	bp.cond.L.Lock()
	defer bp.cond.L.Unlock()
	bp.paused = true
}

func (bp *BatchMixProcess) Resume() {
	bp.cond.L.Lock()
	defer bp.cond.L.Unlock()
	if !bp.paused {
		return
	}
	bp.paused = false
	bp.cond.Signal()
}

func (bp *BatchMixProcess) SetSendInterval(d time.Duration) {
	bp.sendInterval.Store(d)
}

func (bp *BatchMixProcess) report() {
	time := time.NewTicker(time.Second)
	for {
		select {
		case <-time.C:
			cnt := atomic.SwapInt32(&bp.sents, 0)
			receipts := atomic.SwapInt32(&bp.receipts, 0)
			log.Printf("Send: %d/s, Receipts: %d/s\n", cnt, receipts)
		case <-bp.exit:
			return
		}
	}
}

func (bp *BatchMixProcess) perform(host string) {
	client, err := ethclient.Dial(host)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	sentCh := make(chan *Account, 1000)
	receiptCh := make(chan *ReceiptTask, 1000)

	for {
		bp.cond.L.Lock()
		if bp.paused {
			bp.cond.Wait()
		}
		bp.cond.L.Unlock()

		select {
		case act := <-bp.sendCh:
			if act.sendCh == nil {
				act.sendCh = sentCh
				act.receiptCh = receiptCh
			}
			bp.sendTransaction(client, act)
			act.transfer = false
		case act := <-sentCh:
			if !act.transfer && bp.delegate {
				bp.sendDelegate(client, act)
				act.transfer = true
			} else {
				bp.sendTransaction(client, act)
				act.transfer = false
			}
		case task := <-receiptCh:
			bp.getTransactionReceipt(client, task)
		case <-bp.exit:
			return
		}
	}
}

func (bp *BatchMixProcess) nonceAt(client *ethclient.Client, addr common.Address) uint64 {
	var blockNumber *big.Int
	nonce, err := client.NonceAt(context.Background(), addr, blockNumber)
	if err != nil {
		fmt.Printf("Get nonce error, addr: %s, err:%v\n", addr, err)
		return 0
	}
	return nonce

}

func (bp *BatchMixProcess) randomAccount(account *Account) *Account {
	idx := 0
	for i, act := range bp.accounts {
		if act.address == account.address {
			idx = i
			break
		}
	}

	r := idx + 1
	if r == len(bp.accounts) {
		r = idx - 1
	}

	return bp.accounts[r]
}

func (bp *BatchMixProcess) sendTransaction(client *ethclient.Client, account *Account) {
	to := bp.randomAccount(account)
	signer := types.NewEIP155Signer(big.NewInt(ChainId))
	nonce := bp.nonceAt(client, account.address)
	// if nonce < account.nonce {
	//	nonce = account.nonce
	// }
	for i := 0; i < maxSendTransferTxns; i++ {
		tx := types.NewTransaction(
			nonce,
			to.address,
			big.NewInt(1),
			21000,
			big.NewInt(500000000000),
			nil)
		signedTx, err := types.SignTx(tx, signer, account.privateKey)
		if err != nil {
			fmt.Printf("sign tx error: %v\n", err)
			bp.sendCh <- account
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

		if i < maxSendTransferTxns-1 {
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

func (bp *BatchMixProcess) sendDelegate(client *ethclient.Client, account *Account) {
	buf, _ := bp.stub.Delegate("10000000000000000000")

	signer := types.NewEIP155Signer(big.NewInt(ChainId))
	nonce := bp.nonceAt(client, account.address)

	tx, err := types.SignTx(
		types.NewTransaction(
			nonce,
			contractAddr,
			big.NewInt(1),
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
	account.nonce = nonce
	atomic.AddInt32(&bp.sents, 1)
	go func() {
		<-time.After(600 * time.Millisecond)
		account.receiptCh <- &ReceiptTask{
			account: account,
			hash:    tx.Hash(),
		}
	}()
}

func (bp *BatchMixProcess) getTransactionReceipt(client *ethclient.Client, task *ReceiptTask) {
	_, err := client.TransactionReceipt(context.Background(), task.hash)
	if err != nil {
		if time.Since(task.account.lastSent) >= task.account.interval {
			fmt.Printf("get receipt timeout, address:%s, hash: %s, sendTime: %v, now: %v\n",
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
