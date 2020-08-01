package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
)

func TestAccount(t *testing.T) {
	var addrKeyList AddrKeyList
	for i := 0; i <= 15000; i++ {
		privateKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err.Error())
		}
		priByte := crypto.FromECDSA(privateKey)
		pri := common.Bytes2Hex(priByte)
		address := crypto.PubkeyToAddress(privateKey.PublicKey).String()
		addrKey := AddrKey{
			Address: address,
			Key:     pri,
		}

		addrKeyList = append(addrKeyList, addrKey)
	}
	file, err := os.Create("from_keys.json")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer file.Close()
	buf, err := json.MarshalIndent(addrKeyList, " ", " ")
	if err != nil {
		t.Fatal(err.Error())
	}

	_, err = file.Write(buf)
	if err != nil {
		t.Error(err.Error())
	}
}

func TestParpse(t *testing.T) {
	data := parseToAccountFile("./to_keys.json")
	fmt.Println(len(data))
	fmt.Println(data[len(data)-1])
	time.Sleep(time.Second * 20)
}

func TestVersion(t *testing.T) {
	VersionMajor := 0
	VersionMinor := 12
	VersionPatch := 0
	GenesisVersion := uint32(VersionMajor<<16 | VersionMinor<<8 | VersionPatch)
	fmt.Println(GenesisVersion)
}

func TestTimer(t *testing.T) {
	addr := common.MustBech32ToAddress("1")
	fmt.Println(addr)
	// fmt.Println("START")
	// do := func() {
	// 	fmt.Println("func")
	// }
	// time.AfterFunc(time.Second*3, do)
	// time.Sleep(5 * time.Second)
}
