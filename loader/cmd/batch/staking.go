package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
	"github.com/shinnng/platon-test-toolkits/util"
)

type BatchStaking struct {
	stakingConf    *StakingConfig
	url            string
	programVersion uint32
	exit           chan struct{}
}

type StakingConfig struct {
	Nodekey    string
	Blskey     string
	NodeName   string
	PrivateKey string
}

func NewBatchStaking(nodekey, blskey, nodeName, privateKey, url string, programVersion uint32) *BatchStaking {
	fmt.Printf("program version: %d \n", programVersion)
	stakingConf := &StakingConfig{
		Nodekey:    nodekey,
		Blskey:     blskey,
		NodeName:   nodeName,
		PrivateKey: privateKey,
	}
	return &BatchStaking{
		stakingConf:    stakingConf,
		url:            url,
		programVersion: programVersion,
	}
}

func (bs *BatchStaking) Start() {
	var client *ethclient.Client
	var err error
	for {
		client, err = ethclient.Dial(bs.url)
		if err != nil {
			log.Printf("Failure to connect platon %s", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	defer client.Close()

	for {
		number := big.NewInt(100)
		block, err := client.BlockByNumber(context.Background(), number)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
		}
		if block != nil && err == nil {
			log.Println("Platon node mining now")
			break
		}
	}
	fmt.Println("start create staking")
	bs.createStaking(client)
	fmt.Println("end staking")
	os.Exit(0)
}

func (bs *BatchStaking) createStaking(client *ethclient.Client) {
	stub := util.NewStakingStub(bs.stakingConf.Nodekey)
	buf, _ := stub.Create(bs.stakingConf.Blskey, bs.stakingConf.NodeName, bs.programVersion)
	signer := types.NewEIP155Signer(big.NewInt(ChainId))
	accountPK, err := crypto.HexToECDSA(bs.stakingConf.PrivateKey)
	if err != nil {
		log.Printf("%s parse account privatekey error: %v", stub.NodeID.String(), err)
		return
	}
	address := crypto.PubkeyToAddress(accountPK.PublicKey)
	nonce, err := client.NonceAt(context.Background(), address, nil)
	if err != nil {
		log.Printf("%s get nonce error: %v", stub.NodeID.String(), err)
		return
	}
	tx, err := types.SignTx(
		types.NewTransaction(
			nonce,
			contractAddr,
			big.NewInt(1),
			103496,
			big.NewInt(500000000000),
			buf),
		signer, accountPK)
	if err != nil {
		log.Printf("sign tx error: %v", err)
		return
	}

	err = client.SendTransaction(context.Background(), tx)
	if err != nil {
		log.Printf("%s send create staking transaction error %v", stub.NodeID.String(), err)
		return
	}

	t := time.Now()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	fmt.Printf("transaction hash %s", tx.Hash().String())
	for {
		select {
		case <-timer.C:
			_, err = client.TransactionReceipt(context.Background(), tx.Hash())
			if err == nil {
				log.Printf("%s create staking success!!!\n", stub.NodeID.String())
				return
			}
			if time.Since(t) > 120*time.Second {
				log.Printf("%s Get transaction receipt timeout %s", stub.NodeID.String(), tx.Hash().String())
				return
			}
			timer.Reset(100 * time.Millisecond)
		case <-bs.exit:
			fmt.Println("exit")
			return
		}
	}
}

func (bs *BatchStaking) Stop()                           {}
func (bs *BatchStaking) Pause()                          {}
func (bs *BatchStaking) Resume()                         {}
func (bs *BatchStaking) SetSendInterval(d time.Duration) {}
