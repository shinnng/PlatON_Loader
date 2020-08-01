package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/PlatONnetwork/PlatON-Go/common"
	ctypes "github.com/PlatONnetwork/PlatON-Go/consensus/cbft/types"
	"github.com/PlatONnetwork/PlatON-Go/core/cbfttypes"
	"github.com/PlatONnetwork/PlatON-Go/core/rawdb"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/ethdb"

	"github.com/elastic/go-elasticsearch"
)

type BlockInfo struct {
	Type        string         `json:"type"`
	Timestamp   time.Time      `json:"timestamp"`
	Number      uint64         `json:"number"`
	Hash        common.Hash    `json:"hash"`
	Epoch       uint64         `json:"epoch"`
	View        uint64         `json:"view"`
	BlockIndex  uint32         `json:"block_index"`
	Txs         int            `json:"txs"`
	Miner       common.Address `json:"miner"`
	Interval    uint64         `json:"interval"`
	TimestampMs uint64         `json:"timestamp_ms"`
	NodeID      string         `json:"node_id,omitempty"`
	NodeName    string         `json:"node_name,omitempty"`
	Host        string         `json:"host,omitempty"`
}

type BlockList []*BlockInfo

type Validator struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeID"`
	Host   string `json:"host"`
}

type ValidatorList struct {
	Validators []*Validator `json:"validateNodes"`
}

type EpochValidator struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	NodeID string `json:"nodeID"`
	Host   string `json:"host"`
	Index  uint32 `json:"index"`
	Epoch  string `json:"epoch"`
}

type EpochValidatorList []*EpochValidator

func main() {
	log.SetFlags(0)

	cmdFlag := flag.String("cmd", "dump", "Dump block info from chaindb or import block info to elasticsearch(dump/import)")

	// dump flags
	datadirFlag := flag.String("datadir", "", "platon node's data directory")
	outputFlag := flag.String("output", "", "output file(json format)")

	// import flags
	urlFlag := flag.String("url", "http://127.0.0.1:9200", "Elasticsearch host")
	indexNameFlag := flag.String("index_name", "block-static", "Index name")
	dumpFileFlag := flag.String("dump_file", "", "Dump file")
	nodesCfgFlag := flag.String("nodes_cfg", "", "Nodes config file")

	// dump validator
	logdirFlag := flag.String("logdir", "", "platon node's log directory")

	flag.Parse()

	switch *cmdFlag {
	case "dump":
		dump(*datadirFlag, *outputFlag, *nodesCfgFlag)
	case "import":
		importToEs(*urlFlag, *indexNameFlag, *dumpFileFlag)
	case "dump_vd":
		dumpValidators(*logdirFlag, *outputFlag, *nodesCfgFlag)
	case "import_vd":
		importValidator(*urlFlag, *indexNameFlag, *dumpFileFlag)
	default:
		log.Fatalf("Unexpected cmd %s", *cmdFlag)
	}
}

func dump(datadir, output, nodesCfg string) {
	buf, err := ioutil.ReadFile(nodesCfg)
	if err != nil {
		log.Fatalf("Read nodes config file error %v", err)
	}

	var vds ValidatorList
	err = json.Unmarshal(buf, &vds)
	if err != nil {
		log.Fatalf("Unmarshal nodes config error %v", err)
	}

	validators := make(map[string]*Validator, 0)
	for _, v := range vds.Validators {
		validators[v.NodeID] = v
	}
	log.Printf("validators %d", len(validators))

	chainDb, err := ethdb.NewLDBDatabase(datadir+"/platon/chaindata", 0, 0)
	if err != nil {
		panic(err)
	}

	headHash := rawdb.ReadHeadHeaderHash(chainDb)
	if headHash == (common.Hash{}) {
		panic("not found head")
	}

	currentNumber := rawdb.ReadHeaderNumber(chainDb, headHash)
	if currentNumber == nil {
		panic("not found head number")
	}

	blockList := make(BlockList, 0)
	var (
		i        uint64 = 0
		interval uint64 = 0
		preBlock *types.Block
	)
	for i = 0; i <= *currentNumber; i++ {
		hash := rawdb.ReadCanonicalHash(chainDb, i)
		block := rawdb.ReadBlock(chainDb, hash, i)
		if block == nil {
			break
		}

		blockInfo := &BlockInfo{
			Type:        "block",
			Timestamp:   common.MillisToTime(block.Time().Int64()),
			Number:      block.NumberU64(),
			Hash:        block.Hash(),
			Txs:         len(block.Transactions()),
			Miner:       block.Coinbase(),
			Interval:    interval,
			TimestampMs: block.Time().Uint64(),
		}

		if pub, err := crypto.Ecrecover(block.Header().SealHash().Bytes(), block.Header().Signature()); err == nil {
			pubHex := fmt.Sprintf("%x", pub[1:])

			log.Println(pubHex)
			if v, ok := validators[pubHex]; ok {
				blockInfo.NodeID = v.NodeID
				blockInfo.NodeName = v.Name
				blockInfo.Host = v.Host
			}
		}

		_, qc, _ := ctypes.DecodeExtra(block.ExtraData())
		if qc != nil {
			blockInfo.Epoch = qc.Epoch
			blockInfo.View = qc.ViewNumber
			blockInfo.BlockIndex = qc.BlockIndex
		}

		if preBlock != nil {
			blockInfo.Interval = block.Time().Uint64() - preBlock.Time().Uint64()
		}
		preBlock = block
		blockList = append(blockList, blockInfo)
	}

	buf, err = json.MarshalIndent(&blockList, "", " ")
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile(output, buf, 0644)
}

func importToEs(url, indexName, dumpFile string) {
	log.Println("Start import block info to elasticsearch")

	es, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{url}})
	if err != nil {
		log.Fatalf("New elasticsearch client error %v", err)
	}

	dumpData, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		log.Fatalf("Read dump file error %v", err)
	}

	var blockList BlockList
	err = json.Unmarshal(dumpData, &blockList)
	if err != nil {
		log.Fatalf("Parse dump file error %v", err)
	}

	// Re-create the index
	if _, err = es.Indices.Delete([]string{indexName}); err != nil {
		log.Fatalf("Cannot delete index: %s", err)
	}
	res, err := es.Indices.Create(indexName)
	if err != nil {
		log.Fatalf("Cannot create index: %s", err)
	}
	if res.IsError() {
		log.Fatalf("Cannot create index: %s", res)
	}

	batch := 255
	count := len(blockList)
	numBatches := 1
	currBatch := 0

	if count%batch == 0 {
		numBatches = (count / batch)
	} else {
		numBatches = (count / batch) + 1
	}

	var buf bytes.Buffer
	var raw map[string]interface{}
	for i, b := range blockList {
		currBatch = i / batch
		if i == count-1 {
			currBatch++
		}

		meta := []byte(fmt.Sprintf(`{ "index" : { "_id" : "block_%d" } }%s`, b.Number, "\n"))

		data, err := json.Marshal(b)
		if err != nil {
			log.Fatalf("Cannot encode BlockInfo %d: %s", b.Number, err)
		}

		data = append(data, "\n"...)

		buf.Grow(len(meta) + len(data))
		buf.Write(meta)
		buf.Write(data)

		if i > 0 && (i%batch == 0 || i == count-1) {
			log.Printf("> Batch %-2d of %d", currBatch, numBatches)

			res, err = es.Bulk(bytes.NewReader(buf.Bytes()), es.Bulk.WithIndex(indexName))
			if err != nil {
				log.Fatalf("Failure indexing batch %d: %s", currBatch, err)
			}

			if res.IsError() {
				if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
					log.Fatalf("Failure to parse response body: %s", err)
				} else {
					log.Printf("  Error: [%d] %s: %s",
						res.StatusCode,
						raw["error"].(map[string]interface{})["type"],
						raw["error"].(map[string]interface{})["reason"])
				}
			}
			buf.Reset()
		}
	}
}

func dumpValidators(logdir, output, nodesCfg string) {
	buf, err := ioutil.ReadFile(nodesCfg)
	if err != nil {
		log.Fatalf("Read nodes config file error %v", err)
	}

	var vds ValidatorList
	err = json.Unmarshal(buf, &vds)
	if err != nil {
		log.Fatalf("Unmarshal nodes config error %v", err)
	}

	validators := make(map[string]*Validator, 0)
	for _, v := range vds.Validators {
		validators[v.NodeID] = v
	}
	log.Printf("validators %d", len(validators))

	vdFile := "/tmp/validators.log"
	if err := os.Remove(vdFile); err != nil {
		log.Printf("Remove %s error %s", vdFile, err)
	}

	cmd := exec.Command("./filter.sh", logdir, vdFile)

	if out, err := cmd.Output(); err != nil {
		log.Fatalf("Filter validators error %s", err)
	} else {
		log.Printf("Filter validators %s", out)
	}

	file, err := os.Open(vdFile)
	if err != nil {
		log.Fatal("Open file %s error %s", vdFile, err)
	}

	epochVl := make(EpochValidatorList, 0)

	r := bufio.NewReader(file)
	for {
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			panic(err)
		}

		if line == "" || err == io.EOF {
			log.Printf("Finish read")
			break
		}

		log.Println(line)
		spaceSlices := strings.Split(string(line), " ")

		vdsStr := ""
		epochStr := ""
		for _, s := range spaceSlices {
			s := strings.TrimSpace(s)
			if strings.Contains(s, "validators") {
				vdsStr = s
				log.Println("Find validators")
				continue
			}

			if strings.Contains(s, "epoch") {
				epochStr = s
				log.Println("Find epoch")
				continue
			}
		}

		if vdsStr == "" || epochStr == "" {
			log.Println("Not found validators or epoch")
			continue
		}

		vdsSlices := strings.Split(vdsStr, "=")
		var vl cbfttypes.Validators

		ss := strings.TrimLeft(vdsSlices[1], "\"")
		ss = strings.TrimRight(ss, "\"")
		ss = strings.ReplaceAll(ss, "\\", "")

		err = json.Unmarshal([]byte(ss), &vl)
		if err != nil {
			log.Fatalf("Parse validator `%s` error %s", ss, err)
		}

		epochSlices := strings.Split(epochStr, "=")
		epoch := epochSlices[1]

		for k, v := range vl.Nodes {
			vd := validators[k.String()]
			epochVl = append(epochVl, &EpochValidator{
				Type:   "validator",
				Name:   vd.Name,
				NodeID: vd.NodeID,
				Host:   vd.Host,
				Index:  v.Index,
				Epoch:  epoch,
			})
		}
	}

	buf, err = json.MarshalIndent(&epochVl, "", " ")
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile(output, buf, 0644)
}

func importValidator(url, indexName, dumpFile string) {
	log.Println("Start import validator to elasticsearch")

	es, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{url}})
	if err != nil {
		log.Fatalf("New elasticsearch client error %v", err)
	}

	dumpData, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		log.Fatalf("Read dump file error %v", err)
	}

	var vl EpochValidatorList
	err = json.Unmarshal(dumpData, &vl)
	if err != nil {
		log.Fatalf("Parse dump file error %v", err)
	}

	batch := 255
	count := len(vl)
	numBatches := 1
	currBatch := 0

	if count%batch == 0 {
		numBatches = (count / batch)
	} else {
		numBatches = (count / batch) + 1
	}

	var buf bytes.Buffer
	var raw map[string]interface{}
	for i, b := range vl {
		currBatch = i / batch
		if i == count-1 {
			currBatch++
		}

		meta := []byte(fmt.Sprintf(`{ "index" : { "_id" : "validator_%s_%d" } }%s`, b.Epoch, b.Index, "\n"))

		data, err := json.Marshal(b)
		if err != nil {
			log.Fatalf("Cannot encode Validator %s-%d: %s", b.Epoch, b.Index, err)
		}

		data = append(data, "\n"...)

		buf.Grow(len(meta) + len(data))
		buf.Write(meta)
		buf.Write(data)

		if i > 0 && (i%batch == 0 || i == count-1) {
			log.Printf("> Batch %-2d of %d", currBatch, numBatches)

			res, err := es.Bulk(bytes.NewReader(buf.Bytes()), es.Bulk.WithIndex(indexName))
			if err != nil {
				log.Fatalf("Failure indexing batch %d: %s", currBatch, err)
			}

			if res.IsError() {
				if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
					log.Fatalf("Failure to parse response body: %s", err)
				} else {
					log.Printf("  Error: [%d] %s: %s",
						res.StatusCode,
						raw["error"].(map[string]interface{})["type"],
						raw["error"].(map[string]interface{})["reason"])
				}
			}
			buf.Reset()
		}
	}
}
