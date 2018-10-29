package stratum

import "github.com/boomstarternetwork/btcminer/miner"

type Miner interface {
	// Shares return shares channel
	Shares() chan miner.Share

	// Mine starts mining, continue to mine if miner was stopped.
	Mine()

	// Stop initiate mining goroutines stop and wait them to stop.
	Stop()
}
