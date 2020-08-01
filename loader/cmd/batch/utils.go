package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math/big"

	ethereum "github.com/PlatONnetwork/PlatON-Go"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
)

type candidateInfo struct {
	StakingBlockNum uint64 `json:"StakingBlockNum"`
}

func getStakingBlockNum(url string, nodeID discover.NodeID, act *Account) (uint64, error) {
	buf, _ := makeCall(1105, nodeID)

	client, err := ethclient.Dial(url)
	if err != nil {
		log.Printf("dial error: %s\n", err)
		return 0, err
	}
	defer client.Close()

	msg := ethereum.CallMsg{
		From:     act.address,
		To:       &contractAddr,
		Gas:      103496,
		GasPrice: big.NewInt(500000000000),
		Data:     buf,
	}

	var blockNumber *big.Int
	hex, err := client.CallContract(context.Background(), msg, blockNumber)
	if err != nil {
		log.Printf("Get candidate info fail, err: %s\n", err)
		return 0, err
	}
	log.Printf("Get candidate info: %s\n", hex)

	var info candidateInfo
	err = json.Unmarshal(hex, &info)
	if err != nil {
		return 0, err
	}
	return info.StakingBlockNum, nil
}

func makeCall(fn int, nodeId discover.NodeID) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(fn))
	nodeID, _ := rlp.EncodeToBytes(nodeId)

	params := make([][]byte, 0)
	params = append(params, fnType)
	params = append(params, nodeID)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
