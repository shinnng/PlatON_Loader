package util

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/PlatONnetwork/PlatON-Go"
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
)

func TestStakingStub_Create(t *testing.T) {
	sub := NewStakingStub("a22d2767d847e539ef6cf554b0d2b139b08caeb3d395baca6641a0d922d644f6")
	stakBuf, err := sub.Create("2b3f963237f4c55d4920dc7ff95a9aa95302a35c676c0c6b9e330994651e565a", "node name", 1)
	if err != nil {
		t.Fatal(err.Error())
	}
	client, err := ethclient.Dial("http://192.168.9.201:6788")
	if err != nil {
		t.Fatal(err.Error())
	}
	to := common.HexToAddress("0x1000000000000000000000000000000000000002")
	from := common.HexToAddress("0x2e95E3ce0a54951eB9A99152A6d5827872dFB4FD")
	signer := types.NewEIP155Signer(big.NewInt(100))
	nonce, err := client.NonceAt(context.Background(), from, nil)
	pri, err := crypto.HexToECDSA("a689f0879f53710e9e0c1025af410a530d6381eebb5916773195326e123b822b")
	if err != nil {
		t.Fatal(err.Error())
	}
	tx := types.NewTransaction(
		nonce,
		to,
		big.NewInt(1),
		103496,
		big.NewInt(500000000000),
		stakBuf)
	signedTx, err := types.SignTx(tx, signer, pri)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		t.Fatal(err.Error())
	}
	time.Sleep(3 * time.Second)
	data, err := GetCandiateInfo(sub.NodeID)
	if err != nil {
		t.Fatal(err.Error())
	}
	msg := ethereum.CallMsg{
		From:     from,
		To:       &to,
		Gas:      103496,
		GasPrice: big.NewInt(500000000000),
		Data:     data,
	}
	hex, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		fmt.Printf("Get candidate info fail, err: %s\n", err)
		t.Fatal(err.Error())
	}
	fmt.Printf("Get candidate info: %s\n", hex)
}
