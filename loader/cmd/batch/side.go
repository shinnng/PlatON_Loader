package main

import (
	"context"
	"log"
	"math/big"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/ethclient"
	"github.com/PlatONnetwork/PlatON-Go/rpc"
)

var (
	stopNumber            = 40000
	consensusSendInterval = 50 * time.Millisecond
	defaultSendInterval   = 1 * time.Second
)

type SideBatch struct {
	process BatchProcessor

	url string

	exit chan struct{}

	account  *Account
	nodeKey  string
	blsKey   string
	nodeName string

	onlyConsensus bool
	staking       bool
}

func NewSideBatch(
	process BatchProcessor,
	account *Account,
	url, nodeKey, blsKey, nodeName string,
	onlyConsensus, staking bool) *SideBatch {
	return &SideBatch{
		process:       process,
		url:           url,
		exit:          make(chan struct{}),
		account:       account,
		nodeKey:       nodeKey,
		blsKey:        blsKey,
		nodeName:      nodeName,
		onlyConsensus: onlyConsensus,
		staking:       staking,
	}
}

func (sb *SideBatch) Start() {
	sb.checkMining()
	sb.process.Start()

	if sb.onlyConsensus {
		go sb.loop()
	}
}

func (sb *SideBatch) Stop() {
	close(sb.exit)
	sb.process.Stop()
}

func (sb *SideBatch) Pause()  {}
func (sb *SideBatch) Resume() {}

func (sb *SideBatch) SetSendInterval(d time.Duration) {
}

func (sb *SideBatch) checkMining() {
	var client *ethclient.Client
	var err error
	for {
		client, err = ethclient.Dial(sb.url)
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
}

func (sb *SideBatch) loop() {
	// Check if mining block has arrived the stop number.
	//
	var client *rpc.Client
	var err error
	for {
		client, err = rpc.Dial(sb.url)
		if err != nil {
			log.Printf("Failure to connect platon %s", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	defer client.Close()

	timer := time.NewTicker(1 * time.Second)

	consensus := func() bool {
		var c bool
		err := client.Call(&c, "debug_isConsensusNode")
		if err != nil {
			log.Printf("Failure call debug_isConsensusNode%s", err)
			return false
		}
		// log.Printf("Current node is proposer? %v", proposer)
		return c

	}

	for {
		select {
		case <-sb.exit:
			return
		case <-timer.C:
			if consensus() {
				// sb.process.SetSendInterval(consensusSendInterval)
				sb.process.Resume()
			} else {
				// sb.process.SetSendInterval(defaultSendInterval)
				sb.process.Pause()
			}
		}
	}
}
