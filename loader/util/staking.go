package util

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/crypto/bls"
	"github.com/PlatONnetwork/PlatON-Go/node"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/rlp"
)

type StakingStub struct {
	PrivateKey *ecdsa.PrivateKey
	NodeID     discover.NodeID
	Address    common.Address
}

func NewStakingStub(privateKey string) *StakingStub {
	pk, nodeId, addr, err := parsePrivateKey(privateKey)
	if err != nil {
		panic(err)
	}
	return &StakingStub{
		PrivateKey: pk,
		NodeID:     nodeId,
		Address:    addr,
	}
}

func parsePrivateKey(privateKey string) (*ecdsa.PrivateKey, discover.NodeID, common.Address, error) {
	nodePk, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, discover.NodeID{}, common.Address{}, err
	}
	nodePub := nodePk.PublicKey
	nodeID := discover.PubkeyID(&nodePub)
	nodeAddr := crypto.PubkeyToAddress(nodePub)
	return nodePk, nodeID, nodeAddr, nil
}

func (stub *StakingStub) Create(blsKey, nodeName string, proVersion uint32) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(1000))
	typ, _ := rlp.EncodeToBytes(uint16(0))
	benefitAddress, _ := rlp.EncodeToBytes(stub.Address)
	nodeId, _ := rlp.EncodeToBytes(stub.NodeID)
	externalId, _ := rlp.EncodeToBytes("xxxxxxxxxxxxxx")
	nodeNameEn, _ := rlp.EncodeToBytes(nodeName)
	website, _ := rlp.EncodeToBytes("http://www.platon.network")
	details, _ := rlp.EncodeToBytes(nodeName + " super node")
	st, _ := big.NewInt(0).SetString("5000000000000000000000000", 10)
	amount, _ := rlp.EncodeToBytes(st)
	programVersion, _ := rlp.EncodeToBytes(proVersion)
	rewardPer, _ := rlp.EncodeToBytes(uint64(1000))
	var versionSign common.VersionSign
	buf, err := crypto.Sign(node.RlpHash(proVersion).Bytes(), stub.PrivateKey)
	if err != nil {
		return nil, err
	}
	versionSign.SetBytes(buf)
	sign, _ := rlp.EncodeToBytes(versionSign)

	var sec bls.SecretKey
	key, err := hex.DecodeString(blsKey)
	if err != nil {
		return nil, err
	}
	err = sec.SetLittleEndian(key)
	if err != nil {
		return nil, err
	}

	var keyEntries bls.PublicKeyHex
	blsHex := hex.EncodeToString(sec.GetPublicKey().Serialize())
	keyEntries.UnmarshalText([]byte(blsHex))
	blsPub, _ := rlp.EncodeToBytes(keyEntries)

	proof, _ := sec.MakeSchnorrNIZKP()
	proofBytes, _ := proof.MarshalText()
	var proofHex bls.SchnorrProofHex
	proofHex.UnmarshalText(proofBytes)
	proofRlp, _ := rlp.EncodeToBytes(proofHex)

	transactionParams := make([][]byte, 0)
	transactionParams = append(transactionParams, fnType)
	transactionParams = append(transactionParams, typ)
	transactionParams = append(transactionParams, benefitAddress)
	transactionParams = append(transactionParams, nodeId)
	transactionParams = append(transactionParams, externalId)
	transactionParams = append(transactionParams, nodeNameEn)
	transactionParams = append(transactionParams, website)
	transactionParams = append(transactionParams, details)
	transactionParams = append(transactionParams, amount)
	transactionParams = append(transactionParams, rewardPer)
	transactionParams = append(transactionParams, programVersion)
	transactionParams = append(transactionParams, sign)
	transactionParams = append(transactionParams, blsPub)
	transactionParams = append(transactionParams, proofRlp)

	b := new(bytes.Buffer)
	err = rlp.Encode(b, transactionParams)
	return b.Bytes(), err
}

func (stub *StakingStub) Increase(amount string) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint64(1002))
	nodeId, _ := rlp.EncodeToBytes(stub.NodeID)
	typ, _ := rlp.EncodeToBytes(uint16(0))
	increase, _ := big.NewInt(0).SetString(amount, 10)
	amountBytes, _ := rlp.EncodeToBytes(increase)

	params := make([][]byte, 0)
	params = append(params, fnType)
	params = append(params, nodeId)
	params = append(params, typ)
	params = append(params, amountBytes)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (stub *StakingStub) Withdrew() ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(1003))
	nodeId, _ := rlp.EncodeToBytes(stub.NodeID)

	params := make([][]byte, 0)
	params = append(params, fnType)
	params = append(params, nodeId)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (stub *StakingStub) Delegate(amount string) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(1004))
	typ, _ := rlp.EncodeToBytes(uint16(0))
	nodeId, _ := rlp.EncodeToBytes(stub.NodeID)
	n, _ := big.NewInt(0).SetString(amount, 10)
	amountBytes, _ := rlp.EncodeToBytes(n)

	params := make([][]byte, 0)
	params = append(params, fnType)
	params = append(params, typ)
	params = append(params, nodeId)
	params = append(params, amountBytes)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Delegate(nodeID discover.NodeID, amount string) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(1004))
	typ, _ := rlp.EncodeToBytes(uint16(0))
	nodeId, _ := rlp.EncodeToBytes(nodeID)
	n, _ := big.NewInt(0).SetString(amount, 10)
	amountBytes, _ := rlp.EncodeToBytes(n)

	params := make([][]byte, 0)
	params = append(params, fnType)
	params = append(params, typ)
	params = append(params, nodeId)
	params = append(params, amountBytes)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (stub *StakingStub) WithdrewDelegate(stakingBlockNum uint64, amount string) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(1005))
	number, _ := rlp.EncodeToBytes(stakingBlockNum)
	nodeId, _ := rlp.EncodeToBytes(stub.NodeID)
	n, _ := big.NewInt(0).SetString(amount, 10)
	amountBytes, _ := rlp.EncodeToBytes(n)

	params := make([][]byte, 0)
	params = append(params, fnType)
	params = append(params, number)
	params = append(params, nodeId)
	params = append(params, amountBytes)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func WithdrewDelegate(nodeID discover.NodeID, stakingNum uint64, amount string) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(1005))
	number, _ := rlp.EncodeToBytes(stakingNum)
	nodeId, _ := rlp.EncodeToBytes(nodeID)
	n, _ := big.NewInt(0).SetString(amount, 10)
	amountBytes, _ := rlp.EncodeToBytes(n)

	params := make([][]byte, 0)
	params = append(params, fnType)
	params = append(params, number)
	params = append(params, nodeId)
	params = append(params, amountBytes)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (stub *StakingStub) Call(fn int) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(fn))

	params := make([][]byte, 0)
	params = append(params, fnType)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func GetCandiateInfo(nodeID discover.NodeID) ([]byte, error) {
	fnType, _ := rlp.EncodeToBytes(uint16(1105))
	nodeId, _ := rlp.EncodeToBytes(nodeID)

	params := make([][]byte, 0)
	params = append(params, fnType)
	params = append(params, nodeId)

	buf := new(bytes.Buffer)
	err := rlp.Encode(buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
