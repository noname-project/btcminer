package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/scrypt"

	"github.com/btcsuite/btcutil/base58"
	"github.com/ybbus/jsonrpc"
)

const (
	btcRPCURL = "http://127.0.0.1:8332"
	ltcRPCURL = "http://127.0.0.1:12332"

	rpcUser     = "user"
	rpcPassword = "password"

	//btcAddress = "15PKyTs3jJ3Nyf3i6R7D9tfGCY1ZbtqWdv"
	btcAddress = "2N8uc47SFPvDanB66jaVaCUWA44353AEjr8"
	//ltcAddress = "QP9PLDCJQRGZ7HPHPSn45fQ1TETXXJf4L3"
	ltcAddress = "QbMcRaBtRrYyc4tKTt9KgfQ4Em1RgshhUx"

	btc = "btc"
	ltc = "ltc"

	// btc or ltc
	miningCurrency = btc
)

type Transaction struct {
	Hash    string `json:"hash"`
	TxID    string `json:"txid"`
	Weight  uint32 `json:"weight"`
	Fee     uint32 `json:"fee"`
	Data    string `json:"data"`
	SigOps  uint32 `json:"sigops"`
	Depends []uint `json:"depends"`
}

type Block struct {
	PreviousBlockHash string        `json:"previousblockhash"`
	Target            string        `json:"target"`
	NonceRange        string        `json:"noncerange"`
	Bits              string        `json:"bits"`
	LongPollID        string        `json:"longpollid"`
	MinTime           uint32        `json:"mintime"`
	SigOpLimit        uint32        `json:"sigoplimit"`
	CurTime           uint32        `json:"curtime"`
	Height            uint32        `json:"height"`
	Version           uint32        `json:"version"`
	CoinBaseValue     uint64        `json:"coinbasevalue"`
	SizeLimit         uint32        `json:"sizelimit"`
	Transactions      []Transaction `json:"transactions"`
	Capabilities      []string      `json:"capabilities"`
	Mutable           []string      `json:"mutable"`

	Hash       string `json:"-"`
	Nonce      uint32 `json:"-"`
	MerkleRoot []byte `json:"-"`
}

func rpc(method string, params ...interface{}) (
	*jsonrpc.RPCResponse, error) {
	var rpcURL string
	switch miningCurrency {
	case btc:
		rpcURL = btcRPCURL
	case ltc:
		rpcURL = ltcRPCURL
	default:
		panic("unsupported currency: " + miningCurrency)
	}

	client := jsonrpc.NewClientWithOpts(rpcURL, &jsonrpc.RPCClientOpts{
		CustomHeaders: map[string]string{
			"Authorization": "Basic " + base64.StdEncoding.EncodeToString(
				[]byte(rpcUser+":"+rpcPassword)),
		},
	})

	res, err := client.Call(method, params...)
	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}

	return res, nil
}

func rpcGetBlockTemplate() (Block, error) {
	var b Block

	res, err := rpc("getblocktemplate")
	if err != nil {
		return b, err
	}

	err = res.GetObject(&b)
	if err != nil {
		return b, err
	}

	return b, nil
}

func rpcSubmitBlock(block string) error {
	res, err := rpc("submitblock", block)
	if err != nil {
		fmt.Println(err)
	}
	if res.Result != nil {
		resStr, err := res.GetString()
		if err != nil {
			fmt.Println("Failed to get response string:", err)
		}
		fmt.Println("Response string:", resStr)
	} else {
		fmt.Println("Result is nil, submitted")
	}
	return err
}

func uintToLeHex(x, width uint64) string {
	var (
		i   uint64
		hex string
	)
	for i = 0; i < width; i++ {
		hex += fmt.Sprintf("%02x", uint8(x>>uint(8*i)))
	}
	return hex
}

func binToHex(bytes []byte) string {
	return hex.EncodeToString(bytes)
}

func decodeTargetBits(bits string) (target []byte) {
	a, err := hex.DecodeString(bits)
	if err != nil {
		return target
	}
	if len(a) < 2 || len(a) > 32 || a[0] > 32 {
		panic("invalid target hex string")
	}

	target = make([]byte, 32)

	// Bits: 1b0404cb
	// 1b -> right shift of (0x1b) bytes
	// 0404cb -> value

	//copy value to a target slice to position at '32 - shift'
	copy(target[32-a[0]:], a[1:])

	return
}

func encodeCoinbaseHeight(n uint32) []byte {
	const minSize = 1
	bytes := []byte{1}

	for n > 127 {
		bytes[0] += 1
		bytes = append(bytes, byte(n%256))
		n /= 256
	}
	bytes = append(bytes, byte(n))

	for len(bytes) < minSize+1 {
		bytes = append(bytes, 0)
		bytes[0] += 1
	}

	return bytes
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func addrToHash160(address string) string {
	hash := base58.Decode(address)
	hashHex := binToHex(hash)
	return hashHex[2 : len(hashHex)-8]
}

func uintToVarIntHex(x uint64) string {
	switch {
	case x < 0xfd:
		return fmt.Sprintf("%02x", x)
	case x <= 0xffff:
		return "fd" + uintToLeHex(x, 2)
	case x <= 0xffffffff:
		return "fe" + uintToLeHex(x, 4)
	default:
		return "ff" + uintToLeHex(x, 8)
	}
}

func makeCoinBaseTx(coinbaseExtraNonce string, address string, value uint64,
	height uint32) string {

	var coinbaseScript string
	if height == 0 {
		coinbaseScript = coinbaseExtraNonce
	} else {
		coinbaseScript = binToHex(encodeCoinbaseHeight(height)) + coinbaseExtraNonce
	}

	// Create a pubkey script
	// OP_DUP OP_HASH160 <len to push> <pubkey> OP_EQUALVERIFY OP_CHECKSIG
	pubkeyScript := "76a914" + addrToHash160(address) + "88ac"

	tx := ""
	// version
	tx += "01000000"
	// in-counter
	tx += "01"
	// input[0] prev hash
	tx += "0000000000000000000000000000000000000000000000000000000000000000"
	// input[0] prev seqnum
	tx += "ffffffff"
	// input[0] script len
	tx += uintToVarIntHex(uint64(len(coinbaseScript)) / 2)
	// input[0] script
	tx += coinbaseScript
	// input[0] seqnum
	tx += "ffffffff"
	// out-counter
	tx += "01"
	// output[0] value (little endian)
	tx += uintToLeHex(value, 8)
	// output[0] script len
	tx += uintToVarIntHex(uint64(len(pubkeyScript)) / 2)
	// output[0] script
	tx += pubkeyScript
	// lock-time
	tx += "00000000"

	return tx
}

func hexToBin(hexStr string) []byte {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return bytes
}

func computeBTCHash(data []byte) []byte {
	h1 := sha256.Sum256(data)
	h2 := sha256.Sum256(h1[:])
	return h2[:]
}

func computeLTCHash(data []byte) []byte {
	// https://litecoin.info/index.php/Scrypt
	// Litecoin uses the following values for the call to scrypt:
	//    N = 1024;
	//    r = 1;
	//    p = 1;
	//    salt is the same 80 bytes as the input
	//    output is 256 bits (32 bytes)
	hashBytes, err := scrypt.Key(data, data, 1024, 1, 1, 32)
	if err != nil {
		panic(err)
	}
	return hashBytes
}

func computeHash(data []byte) []byte {
	switch miningCurrency {
	case btc:
		return computeBTCHash(data)
	case ltc:
		return computeLTCHash(data)
	default:
		panic("unknown mining currency: " + miningCurrency)
	}
}

func computeHashString(data string) string {
	return binToHex(reverseBytes(computeHash(hexToBin(data))))
}

func reverseBytes(bytes []byte) []byte {
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
	return bytes
}

func computeMerkleRoot(txsHashesHex []string) []byte {
	fmt.Println(txsHashesHex)
	var txsHashes [][]byte
	for _, txHashHex := range txsHashesHex {
		// Reverse the hash from big endian to little endian
		txHash := reverseBytes(hexToBin(txHashHex))
		txsHashes = append(txsHashes, txHash)
	}
	for len(txsHashes) > 1 {
		var newTxsHashes [][]byte
		if len(txsHashes)%2 != 0 {
			txsHashes = append(txsHashes, txsHashes[len(txsHashes)-1])
		}
		for {
			h1 := txsHashes[0]
			h2 := txsHashes[1]
			concat := []byte{}
			concat = append(concat, h1...)
			concat = append(concat, h2...)
			concatHash := computeHash(concat)
			newTxsHashes = append(newTxsHashes, concatHash)
			if len(txsHashes) > 2 {
				txsHashes = txsHashes[2:]
			} else {
				break
			}
		}
		txsHashes = newTxsHashes
	}
	return txsHashes[0]
}

func makeHeader(b Block) []byte {
	var header []byte

	// Version
	versionBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(versionBytes, b.Version)
	header = append(header, versionBytes...)

	// Previous block hash
	header = append(header, reverseBytes(hexToBin(b.PreviousBlockHash))...)

	// Merkle root hash
	header = append(header, b.MerkleRoot...)

	// Time
	timeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(timeBytes, b.CurTime)
	header = append(header, timeBytes...)

	// Target bits
	header = append(header, reverseBytes(hexToBin(b.Bits))...)

	// Nonce
	nonceBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(nonceBytes, b.Nonce)
	header = append(header, nonceBytes...)

	return header
}

func computeBlockHeaderHash(header []byte) []byte {
	hash := computeHash(header)
	return reverseBytes(hash[:])
}

func checkBlockTarget(blockHash []byte, targetHash []byte) bool {
	for i := range blockHash {
		switch {
		case blockHash[i] == targetHash[i]:
			continue
		case blockHash[i] < targetHash[i]:
			return true
		default:
			return false
		}
	}
	return false
}

func computeHpsAverage(hps []float64) float64 {
	if len(hps) == 0 {
		return 0
	}
	var sum float64
	for _, x := range hps {
		sum += x
	}
	return sum / float64(len(hps))
}

func mineBlock(block Block) (Block, bool, float64) {
	var address string
	switch miningCurrency {
	case btc:
		address = btcAddress
	case ltc:
		address = ltcAddress
	default:
		panic("unsupported currency: " + miningCurrency)
	}

	// Unshift empty transaction to create place for coinbase transaction
	block.Transactions = append([]Transaction{{}}, block.Transactions...)

	targetHash := decodeTargetBits(block.Bits)

	startTime := time.Now()
	hps := []float64{}

	var extraNonce uint32 = 0
	for extraNonce <= 0xffffffff {
		var coinbaseTx Transaction

		// Update the coinbase transaction with the extra nonce
		coinbaseExtraNonce := uintToLeHex(uint64(extraNonce), 4)
		coinbaseTx.Data = makeCoinBaseTx(coinbaseExtraNonce, address,
			block.CoinBaseValue, block.Height)
		coinbaseTx.Hash = computeHashString(coinbaseTx.Data)

		block.Transactions[0] = coinbaseTx

		// Recompute the merkle root
		var txsHashesHex []string
		for _, tx := range block.Transactions {
			txsHashesHex = append(txsHashesHex, tx.Hash)
		}

		block.MerkleRoot = computeMerkleRoot(txsHashesHex)
		block.Nonce = 0

		blockHeader := makeHeader(block)

		var nonce uint32 = 0
		for nonce <= 0xffffffff {
			block.Nonce = nonce

			// Update the block header with the new 32-bit nonce
			binary.LittleEndian.PutUint32(blockHeader[76:], nonce)

			//blockHash := computeHash(blockHeader)
			blockHash := computeBlockHeaderHash(blockHeader)

			if checkBlockTarget(blockHash, targetHash) {
				block.Nonce = nonce
				block.Hash = binToHex(blockHash)
				return block, true, computeHpsAverage(hps)
			}

			if nonce > 0 && nonce%10000 == 0 {
				elapsed := time.Now().Sub(startTime)
				hps = append(hps, 10000/elapsed.Seconds())
				if time.Now().Sub(startTime).Seconds() > 60 {
					return block, false, computeHpsAverage(hps)
				}
				fmt.Printf("Average Khash/s: %.4f\n",
					computeHpsAverage(hps)/1000)
				startTime = time.Now()
			}

			nonce++
		}
		extraNonce++
	}

	return block, false, 0
}

func makeBlockSubmission(block Block) string {
	subm := ""

	// Block header
	subm += binToHex(makeHeader(block))

	// Number of transactions as varint
	subm += uintToVarIntHex(uint64(len(block.Transactions)))

	// Concatenated transactions data
	for _, tx := range block.Transactions {
		subm += tx.Data
	}

	return subm
}

func main() {
	for {
		fmt.Println("Mining new block template...")

		block, err := rpcGetBlockTemplate()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		minedBlock, mined, hps := mineBlock(block)

		fmt.Printf("Average Khash/s: %.4f\n", hps/1000)

		if mined {
			fmt.Println("Solved block! Block hash:", minedBlock.Hash)
			blockSubmission := makeBlockSubmission(minedBlock)
			fmt.Println("Submiting:", blockSubmission)
			rpcSubmitBlock(blockSubmission)
			os.Exit(0)
		}
	}
}
