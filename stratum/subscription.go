package stratum

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/boomstarternetwork/btcminer/miner"
)

// subscription is a stratum subscription.
type subscription struct {
	id                string
	extraNonce1       string
	extraNonce2Length uint
	target            string
	difficulty        float64

	// minersCount is a miner goroutines count
	minersCount uint

	miner Miner

	mutex sync.Mutex
}

// set sets new subscription's params
func (s *subscription) set(subID string, extraNonce1 string,
	extraNonce2Length uint) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.id = subID
	s.extraNonce1 = extraNonce1
	s.extraNonce2Length = extraNonce2Length
}

func bigFloatExp(f *big.Float, exp int) *big.Float {
	fexp := big.NewFloat(0).Copy(f)
	for i := 1; i < exp; i++ {
		fexp.Mul(fexp, f)
	}
	return fexp
}

// setDifficulty set mining difficulty and computes target.
func (s *subscription) setDifficulty(difficulty float64) error {
	if difficulty < 0 {
		return errors.New("Difficulty must be non-negative")
	}

	s.difficulty = difficulty

	var target *big.Int

	if difficulty == 0 {
		// python: 2 ** 256 - 1
		target = big.NewInt(0)
		target.Exp(big.NewInt(2), big.NewInt(256), nil)
		target.Sub(target, big.NewInt(1))
	} else {
		//python: (0xffff0000 * (2 ** (256 - 64)) + 1) / difficulty - 1 + 0.5)
		ftarget := bigFloatExp(big.NewFloat(2), 256-64)
		ftarget.Mul(ftarget, big.NewFloat(0xffff0000))
		ftarget.Add(ftarget, big.NewFloat(1))
		ftarget.Quo(ftarget, big.NewFloat(difficulty))
		ftarget.Sub(ftarget, big.NewFloat(0.5))

		target, _ = ftarget.Int(nil)
	}

	s.target = fmt.Sprintf("%064x", target)

	return nil
}

// newMiner creates new miner with given miner params, filled with
// subscription params, and mining goroutines count.
func (s *subscription) newMiner(p miner.Params) (chan miner.Share, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.miner != nil {
		s.miner.Stop()
	}

	p.Target = s.target
	p.ExtraNonce1 = s.extraNonce1
	p.ExtraNonce2Length = s.extraNonce2Length

	var err error

	s.miner, err = miner.NewBTCMiner(p)
	if err != nil {
		return nil, fmt.Errorf("failed to create new miner: %v", err)
	}

	s.miner.Mine()

	return s.miner.Shares(), nil
}

func (s *subscription) continueMine() chan miner.Share {
	s.miner.Mine()
	return s.miner.Shares()
}

func (s *subscription) noMiner() bool {
	return s.miner == nil
}
