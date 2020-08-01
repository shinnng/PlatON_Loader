package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/ethclient"
	"github.com/PlatONnetwork/PlatON-Go/params"
)

var (
	accounts           = parseAccountFile(`all_addr_and_private_keys.json`, 0, 20, 500)
	hasMoneyAddress    = "lax196278ns22j23awdfj9f2d4vz0pedld8au6xelj"
	hasMoneyPrivateKey = "a689f0879f53710e9e0c1025af410a530d6381eebb5916773195326e123b822b"
)

func TestClient(t *testing.T) {
	client, err := ethclient.Dial("http://192.168.9.201:6789")
	if err != nil {
		panic(err.Error())
	}
	address, err := common.Bech32ToAddress(hasMoneyAddress)
	if err != nil {
		panic(err.Error())
	}
	nonce, err := client.NonceAt(context.Background(), address, nil)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println(nonce)
}

func TestParseAccountFile(t *testing.T) {
	bp := NewSideBatch(
		NewBatchProcess(accounts, []string{"ws://192.168.9.201:6790"}),
		accounts[0],
		"ws://192.168.9.201:6790",
		"a464a310005f6e4323b056c4ec4fe665f6799359bcb3cfbc0aaaa82a0771e166",
		"8b4757d841895db28c0669102c321a45ce698ec5c52d898e694166017b2a750b",
		"test",
		false,
		false)
	bp.Start()
	sigs := make(chan os.Signal, 1)
	done := make(chan struct{}, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		done <- struct{}{}
	}()
	<-done
}

func TestSendTransaction(t *testing.T) {
	client, err := ethclient.Dial("http://192.168.9.201:6789")
	if err != nil {
		panic(err.Error())
	}
	from, err := common.Bech32ToAddress(hasMoneyAddress)
	if err != nil {
		panic(err.Error())
	}
	signer := types.NewEIP155Signer(big.NewInt(ChainId))
	nonce, err := client.NonceAt(context.Background(), from, nil)
	if err != nil {
		panic(err.Error())
	}

	pk := crypto.HexMustToECDSA(hasMoneyPrivateKey)
	for _, account := range accounts {
		tx := types.NewTransaction(
			nonce,
			account.address,
			new(big.Int).Mul(big.NewInt(200), big.NewInt(params.LAT)),
			21000,
			big.NewInt(500000000000),
			nil)
		signedTx, err := types.SignTx(tx, signer, pk)
		if err != nil {
			fmt.Printf("sign tx error: %v\n", err)
			return
		}

		err = client.SendTransaction(context.Background(), signedTx)
		if err != nil {
			panic(err.Error())
		}
		nonce++
	}
}

func TestInit(t *testing.T) {
	fmt.Println(contractAddr.String())
}
