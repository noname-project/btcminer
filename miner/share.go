package miner

import (
	"encoding/hex"
)

// Share is a miner mining result, stores data required for pool to submit.
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
