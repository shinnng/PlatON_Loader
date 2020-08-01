package main

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
	"github.com/shinnng/platon-test-toolkits/util"
)

// var contractAddr = common.Bech32ToAddress("lax1zqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqzlh5ge3")
var (
	contractAddr common.Address
	err          error
)

func init() {
	contractAddr, err = common.Bech32ToAddress("lax1zqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqzlh5ge3")
}

type StakingBatchProcess struct {
	accounts AccountList
	hosts    []string

	sendCh chan *Account
	waitCh chan *ReceiptTask

	exit chan struct{}

	sents int32

	stub *util.StakingStub

	BatchProcessor
}

func NewStakingBatchProcess(accounts AccountList, hosts []string, nodeKey string) BatchProcessor {
	return &StakingBatchProcess{
		accounts: accounts,
		hosts:    hosts,
		sendCh:   make(chan *Account, 4096),
		waitCh:   make(chan *ReceiptTask, 4096),
		exit:     make(chan struct{}),
		sents:    0,
		stub:     util.NewStakingStub(nodeKey),
	}
}

func (bp *StakingBatchProcess) Start() {
	// client, err := ethclient.Dial(bp.hosts[0])
	// if err != nil {
	// 	panic(err.Error())
	// }
	// for {
	// 	if block, err := client.BlockByNumber(context.Background(), big.NewInt(10000)); err == nil && block != nil {
	// 		break
	// 	}
	// 	time.Sleep(time.Second * 1)
	// }
	// time.Sleep(time.Hour * 4)
	go bp.report()

	for _, host := range bp.hosts {
		go bp.perform(host)
	}

	for _, act := range bp.accounts {
		bp.sendCh <- act
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Println("start success")
}

func (bp *StakingBatchProcess) Stop() {
	close(bp.exit)
}

func (bp *StakingBatchProcess) Pause()  {}
func (bp *StakingBatchProcess) Resume() {}

func (bp *StakingBatchProcess) SetSendInterval(d time.Duration) {}

func (bp *StakingBatchProcess) report() {
	timer := time.NewTimer(time.Second)
	for {
		select {
		case <-timer.C:
			cnt := atomic.SwapInt32(&bp.sents, 0)
			fmt.Printf("Send: %d/s\n", cnt)
			timer.Reset(time.Second)
		case <-bp.exit:
			return
		}
	}
}

func (bp *StakingBatchProcess) perform(host string) {
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
			bp.delegate(client, act)
		case act := <-sentCh:
			bp.delegate(client, act)
		case task := <-receiptCh:
			bp.getTransactionReceipt(client, task)
		case <-bp.exit:
			return
		}
	}
}

func (bp *StakingBatchProcess) nonceAt(client *ethclient.Client, addr common.Address) uint64 {
	var blockNumber *big.Int
	nonce, err := client.NonceAt(context.Background(), addr, blockNumber)
	if err != nil {
		fmt.Printf("Get nonce error, addr: %s, err:%v\n", addr, err)
		return 0
	}
	return nonce

}

func (bp *StakingBatchProcess) delegate(client *ethclient.Client, account *Account) {
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
			<-time.After(50 * time.Millisecond)
			account.sendCh <- account
		}()
		return
	}
	atomic.AddInt32(&bp.sents, 1)
	go func() {
		<-time.After(600 * time.Millisecond)
		account.receiptCh <- &ReceiptTask{
			account: account,
			hash:    tx.Hash(),
		}
	}()
}

func (bp *StakingBatchProcess) getTransactionReceipt(client *ethclient.Client, task *ReceiptTask) {
	receipt, err := client.TransactionReceipt(context.Background(), task.hash)
	if err != nil {
		if time.Since(task.account.lastSent) >= task.account.interval {
			fmt.Printf("get receipt timeout, address:%s, hash: %s sendTime: %v, now: %v\n",
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

	if len(receipt.Logs) > 0 {
		result := string(receipt.Logs[0].Data[2:])
		if strings.Contains(result, "Delegate failed: Account of Candidate(Validator)") {
			fmt.Printf("%s\n", result)
			return
		}
		// fmt.Printf("Staking txHash: %s, result: %s\n", task.hash.String(), receipt.Logs[0].Data[2:])
	}

	go func() {
		<-time.After(50 * time.Millisecond)
		task.account.sendCh <- task.account
	}()
}
