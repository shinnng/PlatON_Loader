package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
	"github.com/shinnng/platon-test-toolkits/util"
)

type RandAccount struct {
	Address common.Address `json:"address"`
}

type BatchRandomProcess struct {
	accounts AccountList
	hosts    []string

	sendCh chan *Account
	waitCh chan *ReceiptTask

	exit chan struct{}

	sents    int32
	receipts int32

	sendInterval time.Duration

	BatchProcessor

	stub     *util.StakingStub
	delegate bool

	nextAddress  uint64
	randomAddres []*RandAccount

	maxSendTxPerAccount int
}

func NewBatchRandomProcess(accounts AccountList, hosts []string, nodeKey string, delegate bool, randmonCount int, accountFile string, randIdx int) BatchProcessor {
	bp := &BatchRandomProcess{
		accounts:            accounts,
		hosts:               hosts,
		sendCh:              make(chan *Account, 1000),
		waitCh:              make(chan *ReceiptTask, 1000),
		exit:                make(chan struct{}),
		sents:               0,
		receipts:            0,
		sendInterval:        50 * time.Millisecond,
		stub:                util.NewStakingStub(nodeKey),
		delegate:            delegate,
		nextAddress:         0,
		maxSendTxPerAccount: 1,
	}

	if accountFile == "" {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		for i := 0; i < randmonCount; i++ {
			bp.randomAddres = append(bp.randomAddres, &RandAccount{common.BigToAddress(big.NewInt(r.Int63()))})
		}
	} else {
		buf, err := ioutil.ReadFile(accountFile)
		if err != nil {
			panic(err)
		}

		if err := json.Unmarshal(buf, &bp.randomAddres); err != nil {
			panic(err)
		}

		total := len(bp.randomAddres)
		if randIdx+randmonCount <= total {
			bp.randomAddres = bp.randomAddres[randIdx : randIdx+randmonCount]
		} else {
			if randIdx < total {
				bp.randomAddres = bp.randomAddres[total-randmonCount:]
			} else {
				bp.randomAddres = bp.randomAddres[:randmonCount]
			}
		}

		log.Printf("nextAddress: %d\n", bp.nextAddress)
	}
	return bp
}

func (bp *BatchRandomProcess) Start() {
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

func (bp *BatchRandomProcess) Stop() {
	close(bp.exit)
}

func (bp *BatchRandomProcess) Pause() {}

func (bp *BatchRandomProcess) Resume() {}

func (bp *BatchRandomProcess) SetSendInterval(d time.Duration) {}

func (bp *BatchRandomProcess) report() {
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

func (bp *BatchRandomProcess) perform(host string) {
	client, err := ethclient.Dial(host)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	sentCh := make(chan *Account, 1000)
	receiptCh := make(chan *ReceiptTask, 1000)

	for {
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

func (bp *BatchRandomProcess) nonceAt(client *ethclient.Client, addr common.Address) uint64 {
	var blockNumber *big.Int
	nonce, err := client.NonceAt(context.Background(), addr, blockNumber)
	if err != nil {
		fmt.Printf("Get nonce error, addr: %s, err:%v\n", addr, err)
		return 0
	}
	return nonce

}

func (bp *BatchRandomProcess) randomAddress() common.Address {
	addr := bp.randomAddres[bp.nextAddress%uint64(len(bp.randomAddres))]
	bp.nextAddress++
	return addr.Address
}

func (bp *BatchRandomProcess) sendTransaction(client *ethclient.Client, account *Account) {
	to := bp.randomAddress()
	signer := types.NewEIP155Signer(big.NewInt(ChainId))
	nonce := bp.nonceAt(client, account.address)
	// if nonce < account.nonce {
	//	nonce = account.nonce
	// }
	for i := 0; i < bp.maxSendTxPerAccount; i++ {
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

		if i < bp.maxSendTxPerAccount-1 {
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

func (bp *BatchRandomProcess) sendDelegate(client *ethclient.Client, account *Account) {
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
			<-time.After(bp.sendInterval)
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

func (bp *BatchRandomProcess) getTransactionReceipt(client *ethclient.Client, task *ReceiptTask) {
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
		<-time.After(bp.sendInterval)
		task.account.sendCh <- task.account
	}()
}
