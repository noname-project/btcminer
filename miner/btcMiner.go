package miner

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// BTCMiner is stratum job which implements mining for bitcoin like coins.
type BTCMiner struct {
	// Miner params from mining.notify command.
	jobID          string
	prevHash       []byte
	coinb1         []byte
	coinb2         []byte
	merkleBranches [][]byte
	version        []byte
	nbits          []byte
	ntime          []byte

	// Miner params from mining.subscribe command.
	target      []byte
	extraNonce1 []byte
	// ExtraNonce2Length variable expected to always be 4.
	extraNonce2Length uint

	// HashFunc is proof of work hashing algrorithm: sha256d, scrypt, etc..
	hashFunc func([]byte) []byte

	// MinersCount is a mining goroutines count
	minersCount uint

	// stopMining boolean atomic value required to init mining goroutines stop.
	stopMining atomic.Value

	// minersWg wait group for miners stop waiting.
	minersWg sync.WaitGroup

	// minersParams storing miners params then miners are stopping, required
	// for ability to resume BTCMiner after miners are stopped.
	minersParams sync.Map

	// shares is a channel with mining shares which can be submitted as
	// share to the pool.
	shares chan Share

	// metrics data
	metricsLoggerRunning  bool
	metricsStartTime      time.Time
	metricsHashesCounters []uint64
}

func NewBTCMiner(p Params) (*BTCMiner, error) {
	var err error

	j := &BTCMiner{}

	j.jobID = p.JobID

	j.prevHash, err = hex.DecodeString(p.PrevHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PrevHash: %v", err)
	}
	j.prevHash = reverseBytesCopy(restorePrevHashByteOrder(j.prevHash))

	j.coinb1, err = hex.DecodeString(p.Coinb1)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Coinb1: %v", err)
	}

	j.coinb2, err = hex.DecodeString(p.Coinb2)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Coinb2: %v", err)
	}

	for _, mbHex := range p.MerkleBranches {
		mb, err := hex.DecodeString(mbHex)
		if err != nil {
			return nil, fmt.Errorf("failed to decode merkle branch: %v", err)
		}
		j.merkleBranches = append(j.merkleBranches, mb)
	}

	j.version, err = hex.DecodeString(p.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Version: %v", err)
	}
	reverseBytes(j.version)

	j.nbits, err = hex.DecodeString(p.Nbits)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Nbits: %v", err)
	}
	reverseBytes(j.nbits)

	j.ntime, err = hex.DecodeString(p.Ntime)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Ntime: %v", err)
	}
	reverseBytes(j.ntime)

	j.target, err = hex.DecodeString(p.Target)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Target: %v", err)
	}

	j.extraNonce1, err = hex.DecodeString(p.ExtraNonce1)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ExtraNonce1: %v", err)
	}

	j.extraNonce2Length = p.ExtraNonce2Length
	if j.extraNonce2Length != 4 {
		return nil, errors.New("ExtraNonce2Length expected to always be 4")
	}

	j.hashFunc = p.Algorithm.hashFunc()
	j.minersCount = p.MinersCount

	return j, nil
}

// merkleRoot forms merkle root.
func (m *BTCMiner) merkleRoot(extraNonce2 []byte) []byte {
	coinbase := make([]byte, 0, len(m.coinb1)+len(m.extraNonce1)+
		len(extraNonce2)+len(m.coinb2))

	coinbase = append(coinbase, m.coinb1...)
	coinbase = append(coinbase, m.extraNonce1...)
	coinbase = append(coinbase, extraNonce2...)
	coinbase = append(coinbase, m.coinb2...)

	merkleRoot := m.hashFunc(coinbase)

	for _, branch := range m.merkleBranches {
		merkleRoot = append(merkleRoot, branch...)
		merkleRoot = m.hashFunc(merkleRoot)
	}

	return merkleRoot
}

// headerPrefix forms block header prefix.
func (m *BTCMiner) headerPrefix(extraNonce2 []byte) []byte {
	merkleRoot := m.merkleRoot(extraNonce2)

	prefix := make([]byte, 0, len(m.version)+len(m.prevHash)+
		len(merkleRoot)+len(m.ntime)+len(m.nbits))

	prefix = append(prefix, m.version...)
	prefix = append(prefix, m.prevHash...)
	prefix = append(prefix, merkleRoot...)
	prefix = append(prefix, m.ntime...)
	prefix = append(prefix, m.nbits...)

	return prefix
}

// reachTarget computes if given block hash reached Target.
func (m *BTCMiner) reachTarget(blockHash []byte) bool {
	for i := 0; i < len(blockHash); i++ {
		switch {
		case blockHash[i] < m.target[i]:
			return true
		case blockHash[i] > m.target[i]:
			return false
		}
	}
	return false
}

type minerParams struct {
	extraNonce2 uint32
	nonce       uint32
}

// miner is a miner goroutine function, intended to be run as goroutine.
//
// nonceStart and nonceStride are useful for multi-processing if you
// would like to assign each process a different starting nonce
// (0, 1, 2, ...) and a stride equal to the number of processes.
//
// For single processor you can use nonceStart=0 and nonceStride=1.
//
// TODO: variable ExtraNonce2Length.
func (m *BTCMiner) miner(nonceStart uint32, nonceStride uint) {
	minerID := fmt.Sprintf("%d:%d", nonceStart, nonceStride)

	var params minerParams

	paramsInt, exists := m.minersParams.Load(minerID)
	if exists {
		params = paramsInt.(minerParams)
	} else {
		params.nonce = nonceStart
	}

	m.minersWg.Add(1)

	go func() {
		defer m.minersWg.Done()

		for extraNonce2 := params.extraNonce2; extraNonce2 <=
			0xffffffff; extraNonce2++ {

			extraNonce2Bytes := uint32ToLeBytes(extraNonce2)
			headerPrefix := m.headerPrefix(extraNonce2Bytes)

			for nonce := params.nonce; nonce <= 0xffffffff; nonce +=
				uint32(nonceStride) {

				if m.stopMining.Load().(bool) {
					m.minersParams.Store(minerID, minerParams{
						extraNonce2: extraNonce2,
						nonce:       nonce,
					})
					return
				}

				nonceBytes := uint32ToLeBytes(nonce)
				header := append(headerPrefix, nonceBytes...)

				headerHash := m.hashFunc(header)

				atomic.AddUint64(&m.metricsHashesCounters[nonceStart], 1)

				if m.reachTarget(headerHash) {
					m.stopMining.Store(true)

					m.shares <- newShare(m.jobID, extraNonce2Bytes, m.ntime,
						nonceBytes)

					nextNonce := nonce + uint32(nonceStride)
					if nextNonce < nonce {
						extraNonce2++
					}

					m.minersParams.Store(minerID, minerParams{
						extraNonce2: extraNonce2,
						nonce:       nextNonce,
					})

					return
				}
			}
		}
	}()
}

func (m *BTCMiner) metricsLogger() {
	m.metricsLoggerRunning = true

	for {
		time.Sleep(10 * time.Second)

		if m.stopMining.Load().(bool) {
			break
		}

		elapsed := time.Now().Sub(m.metricsStartTime)

		var hashes uint64
		for i := uint(0); i < m.minersCount; i++ {
			hashes += atomic.LoadUint64(&m.metricsHashesCounters[i])
			atomic.StoreUint64(&m.metricsHashesCounters[i], 0)
		}

		m.metricsStartTime = time.Now()

		hashRate := float64(hashes) / elapsed.Seconds()
		valueStr := "H/s"

		if hashRate >= 100 {
			hashRate /= 1000
			valueStr = "KH/s"
		}

		if hashRate >= 100 {
			hashRate /= 1000
			valueStr = "MH/s"
		}

		logrus.Infof("Hash rate is %0.2g %s", hashRate, valueStr)
	}

	m.metricsLoggerRunning = false
}

// Shares return shares channel
func (m *BTCMiner) Shares() chan Share {
	return m.shares
}

// Mine starts miner, runs mining goroutines.
func (m *BTCMiner) Mine() {
	m.shares = make(chan Share)

	m.stopMining.Store(false)

	if !m.metricsLoggerRunning {
		m.metricsHashesCounters = make([]uint64, m.minersCount)
		m.metricsStartTime = time.Now()
		go m.metricsLogger()
	}

	for i := uint32(0); i < uint32(m.minersCount); i++ {
		go m.miner(i, m.minersCount)
	}
}

// Stop initiate mining goroutines stop and wait them to stop.
func (m *BTCMiner) Stop() {
	m.stopMining.Store(true)
	m.minersWg.Wait()
}
