package main

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
)

const maxSendTxns = 5

type BatchProcess struct {
	accounts AccountList
	hosts    []string

	sendCh chan *Account
	waitCh chan *ReceiptTask

	signer types.EIP155Signer

	exit chan struct{}

	sents    int32
	receipts int32

	sendInterval atomic.Value // time.Duration

	paused bool
	lock   sync.Mutex
	cond   *sync.Cond

	BatchProcessor
}

func NewBatchProcess(accounts AccountList, hosts []string) *BatchProcess {
	bp := &BatchProcess{
		accounts: accounts,
		hosts:    hosts,
		sendCh:   make(chan *Account, 10000),
		waitCh:   make(chan *ReceiptTask, 10000),
		signer:   types.NewEIP155Signer(big.NewInt(ChainId)),
		exit:     make(chan struct{}),
		sents:    0,
		paused:   false,
	}
	bp.cond = sync.NewCond(&bp.lock)
	bp.sendInterval.Store(50 * time.Millisecond)
	return bp
}

func (bp *BatchProcess) Start() {
	go bp.report()

	for _, host := range bp.hosts {
		go bp.perform(host)
	}

	for _, act := range bp.accounts {
		bp.sendCh <- act
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println("start success")
}

func (bp *BatchProcess) Stop() {
	close(bp.exit)
}

func (bp *BatchProcess) Pause() {
	bp.cond.L.Lock()
	defer bp.cond.L.Unlock()
	bp.paused = true
}

func (bp *BatchProcess) Resume() {
	bp.cond.L.Lock()
	defer bp.cond.L.Unlock()
	if !bp.paused {
		return
	}
	bp.paused = false
	bp.cond.Signal()
}

func (bp *BatchProcess) SetSendInterval(d time.Duration) {
	bp.sendInterval.Store(d)
}

func (bp *BatchProcess) report() {
	timer := time.NewTimer(time.Second)
	for {
		select {
		case <-timer.C:
			cnt := atomic.SwapInt32(&bp.sents, 0)
			receipts := atomic.SwapInt32(&bp.receipts, 0)
			fmt.Printf("Send: %d/s, Receipts: %d/s\n", cnt, receipts)
			timer.Reset(time.Second)
		case <-bp.exit:
			return
		}
	}
}

func (bp *BatchProcess) perform(host string) {
	client, err := ethclient.Dial(host)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	sentCh := make(chan *Account, cap(bp.sendCh))
	receiptCh := make(chan *ReceiptTask, cap(bp.sendCh))

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
		case act := <-sentCh:
			bp.sendTransaction(client, act)
		case task := <-receiptCh:
			bp.getTransactionReceipt(client, task)
		case <-bp.exit:
			return
		}
	}
}

func (bp *BatchProcess) nonceAt(client *ethclient.Client, addr common.Address) uint64 {
	var blockNumber *big.Int
	nonce, err := client.NonceAt(context.Background(), addr, blockNumber)
	if err != nil {
		fmt.Printf("Get nonce error, addr: %s, err:%v\n", addr, err)
		return 0
	}
	return nonce

}

func (bp *BatchProcess) randomAccount(account *Account) *Account {
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

func randomToAddrKey() AddrKey {
	dataLen := len(toAccount)
	idx := rand.Intn(dataLen)
	return toAccount[idx]
}

func (bp *BatchProcess) sendTransaction(client *ethclient.Client, account *Account) {
	to := bp.randomAccount(account)
	// to := randomToAddrKey()
	// signer := types.NewEIP155Signer(big.NewInt(ChainId))
	nonce := bp.nonceAt(client, account.address)
	// if nonce < account.nonce {
	//	nonce = account.nonce
	// }
	for i := 0; i < maxSendTxns; i++ {
		tx := types.NewTransaction(
			nonce,
			to.address,
			big.NewInt(200),
			21000,
			big.NewInt(500000000000),
			nil)
		signedTx, err := types.SignTx(tx, bp.signer, account.privateKey)
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
				<-time.After(bp.sendInterval.Load().(time.Duration))
				account.sendCh <- account
			}()
			return
		}
		// account.nonce = nonce + 1
		atomic.AddInt32(&bp.sents, 1)

		nonce += 1

		if i < maxSendTxns-1 {
			continue
		}

		go func() {
			<-time.After(2 * time.Second)
			account.receiptCh <- &ReceiptTask{
				account: account,
				hash:    signedTx.Hash(),
			}
		}()
	}
}

func (bp *BatchProcess) getTransactionReceipt(client *ethclient.Client, task *ReceiptTask) {
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
