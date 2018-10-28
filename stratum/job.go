package stratum

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

// job is stratum job which implements mining.
type job struct {
	// Job params from mining.notify command.
	ID             string
	prevHash       []byte
	coinb1         []byte
	coinb2         []byte
	merkleBranches [][]byte
	version        []byte
	nbits          []byte
	ntime          []byte

	// Job params from mining.subscribe command.
	target      []byte
	extraNonce1 []byte
	// extraNonce2Length variable expected to always be 4.
	extraNonce2Length uint

	// hashFunc is proof of work hashing algrorithm: sha256d, scrypt, etc..
	hashFunc func([]byte) []byte

	// minersCount is a mining goroutines count
	minersCount uint

	// stopMining boolean atomic value required to init mining goroutines stop.
	stopMining atomic.Value

	// minersWg wait group for miners stop waiting.
	minersWg sync.WaitGroup

	// minersParams storing mainers params then miners are stopping, required
	// for ability to resume job after miners are stopped.
	minersParams sync.Map

	// shares is a channel with mining shares which can be submitted as
	// Share to the pool.
	shares chan Share
}

type jobParams struct {
	jobID          string
	prevHash       string
	coinb1         string
	coinb2         string
	merkleBranches []string
	version        string
	nbits          string
	ntime          string
	target         string
	extraNonce1    string
	// extraNonce2Length variable expected to always be 4.
	extraNonce2Length uint
	hashFunc          func([]byte) []byte
	minersCount       uint
}

func newJob(p jobParams) (*job, error) {
	var err error

	j := &job{}

	j.ID = p.jobID

	j.prevHash, err = hex.DecodeString(p.prevHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode prevHash: %v", err)
	}
	j.prevHash = reverseBytesCopy(restorePrevHashByteOrder(j.prevHash))

	j.coinb1, err = hex.DecodeString(p.coinb1)
	if err != nil {
		return nil, fmt.Errorf("failed to decode coinb1: %v", err)
	}

	j.coinb2, err = hex.DecodeString(p.coinb2)
	if err != nil {
		return nil, fmt.Errorf("failed to decode coinb2: %v", err)
	}

	for _, mbHex := range p.merkleBranches {
		mb, err := hex.DecodeString(mbHex)
		if err != nil {
			return nil, fmt.Errorf("failed to decode merkle branch: %v", err)
		}
		j.merkleBranches = append(j.merkleBranches, mb)
	}

	j.version, err = hex.DecodeString(p.version)
	if err != nil {
		return nil, fmt.Errorf("failed to decode version: %v", err)
	}
	reverseBytes(j.version)

	j.nbits, err = hex.DecodeString(p.nbits)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nbits: %v", err)
	}
	reverseBytes(j.nbits)

	j.ntime, err = hex.DecodeString(p.ntime)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ntime: %v", err)
	}
	reverseBytes(j.ntime)

	j.target, err = hex.DecodeString(p.target)
	if err != nil {
		return nil, fmt.Errorf("failed to decode target: %v", err)
	}

	j.extraNonce1, err = hex.DecodeString(p.extraNonce1)
	if err != nil {
		return nil, fmt.Errorf("failed to decode extraNonce1: %v", err)
	}

	j.extraNonce2Length = p.extraNonce2Length
	if j.extraNonce2Length != 4 {
		return nil, errors.New("extraNonce2Length expected to always be 4")
	}

	j.hashFunc = p.hashFunc
	j.minersCount = p.minersCount

	return j, nil
}

// merkleRoot forms merkle root.
func (j *job) merkleRoot(extraNonce2 []byte) []byte {
	coinbase := make([]byte, 0, len(j.coinb1)+len(j.extraNonce1)+
		len(extraNonce2)+len(j.coinb2))

	coinbase = append(coinbase, j.coinb1...)
	coinbase = append(coinbase, j.extraNonce1...)
	coinbase = append(coinbase, extraNonce2...)
	coinbase = append(coinbase, j.coinb2...)

	merkleRoot := j.hashFunc(coinbase)

	for _, branch := range j.merkleBranches {
		merkleRoot = append(merkleRoot, branch...)
		merkleRoot = j.hashFunc(merkleRoot)
	}

	return merkleRoot
}

// headerPrefix forms block header prefix.
func (j *job) headerPrefix(extraNonce2 []byte) []byte {
	merkleRoot := j.merkleRoot(extraNonce2)

	prefix := make([]byte, 0, len(j.version)+len(j.prevHash)+
		len(merkleRoot)+len(j.ntime)+len(j.nbits))

	prefix = append(prefix, j.version...)
	prefix = append(prefix, j.prevHash...)
	prefix = append(prefix, merkleRoot...)
	prefix = append(prefix, j.ntime...)
	prefix = append(prefix, j.nbits...)

	return prefix
}

// reachTarget computes if given block hash reached target.
func (j *job) reachTarget(blockHash []byte) bool {
	for i := 0; i < len(blockHash); i++ {
		switch {
		case blockHash[i] < j.target[i]:
			return true
		case blockHash[i] > j.target[i]:
			return false
		}
	}
	return false
}

// stop stops mining goroutines.
func (j *job) stop() {
	j.stopMining.Store(true)
	j.minersWg.Wait()
}

type minerParams struct {
	extraNonce2 uint32
	nonce       uint32
}

// miner starts mining goroutine.
//
// nonceStart and nonceStride are useful for multi-processing if you
// would like to assign each process a different starting nonce
// (0, 1, 2, ...) and a stride equal to the number of processes.
//
// For single processor you can use nonceStart=0 and nonceStride=1.
//
// TODO: implement metrics.
// TODO: implement variable extraNonce2Length
func (j *job) miner(nonceStart uint32, nonceStride uint) {
	minerID := fmt.Sprintf("%d:%d", nonceStart, nonceStride)

	var params minerParams

	paramsInt, exists := j.minersParams.Load(minerID)
	if exists {
		params = paramsInt.(minerParams)
	} else {
		params.nonce = nonceStart
	}

	j.minersWg.Add(1)

	go func() {
		defer j.minersWg.Done()

		for extraNonce2 := params.extraNonce2; extraNonce2 <=
			0xffffffff; extraNonce2++ {

			extraNonce2Bytes := uint32ToLeBytes(extraNonce2)
			headerPrefix := j.headerPrefix(extraNonce2Bytes)

			for nonce := params.nonce; nonce <= 0xffffffff; nonce +=
				uint32(nonceStride) {

				if j.stopMining.Load().(bool) {
					j.minersParams.Store(minerID, minerParams{
						extraNonce2: extraNonce2,
						nonce:       nonce,
					})
					return
				}

				nonceBytes := uint32ToLeBytes(nonce)
				header := append(headerPrefix, nonceBytes...)

				headerHash := j.hashFunc(header)

				if j.reachTarget(headerHash) {
					j.stopMining.Store(true)

					logrus.WithFields(logrus.Fields{
						"minerID":     minerID,
						"extraNonce2": extraNonce2,
						"nonce":       nonce,
					}).Debug("Miner found share")

					j.shares <- newShare(j.ID, extraNonce2Bytes, j.ntime,
						nonceBytes)

					nextNonce := nonce + uint32(nonceStride)
					if nextNonce < nonce {
						extraNonce2++
					}

					j.minersParams.Store(minerID, minerParams{
						extraNonce2: extraNonce2,
						nonce:       nextNonce,
					})

					return
				}
			}
		}
	}()
}

func (j *job) mine() {
	j.shares = make(chan Share)
	j.stopMining.Store(false)
	for i := uint(0); i < j.minersCount; i++ {
		go j.miner(uint32(i), j.minersCount)
	}
}

// Share is job mining result, stores data required for pool to submit.
type Share struct {
	JobID       string
	ExtraNonce2 string
	Nonce       string
	Ntime       string
}

// newShare forms new share from given binary data, converts them
// to hexadecimal representation.
func newShare(jobID string, extraNonce2 []byte, ntime []byte,
	nonce []byte) Share {
	r := Share{}
	r.JobID = jobID
	r.ExtraNonce2 = hex.EncodeToString(extraNonce2)
	r.Ntime = hex.EncodeToString(reverseBytesCopy(ntime))
	r.Nonce = hex.EncodeToString(nonce)
	return r
}
