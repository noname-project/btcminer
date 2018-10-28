package stratum

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
)

// subscription is a stratum subscription.
type subscription struct {
	id                string
	extraNonce1       string
	extraNonce2Length uint
	target            string
	difficulty        float64

	minersCount uint

	job *job

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

// newJob creates new job with given job params, filled with
// subscription params, and mining goroutines count.
func (s *subscription) newJob(p jobParams) (chan Share, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.job != nil {
		s.job.stop()
	}

	p.target = s.target
	p.extraNonce1 = s.extraNonce1
	p.extraNonce2Length = s.extraNonce2Length

	var err error

	s.job, err = newJob(p)
	if err != nil {
		return nil, fmt.Errorf("failed to create new job: %v", err)
	}

	s.job.mine()

	return s.job.shares, nil
}

func (s *subscription) stopJob() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.job = nil
}

func (s *subscription) continueJob() chan Share {
	s.job.mine()
	return s.job.shares
}

func (s *subscription) noJob() bool {
	return s.job == nil
}
