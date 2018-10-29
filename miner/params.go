package miner

type Params struct {
	JobID          string
	PrevHash       string
	Coinb1         string
	Coinb2         string
	MerkleBranches []string
	Version        string
	Nbits          string
	Ntime          string
	Target         string
	ExtraNonce1    string
	// ExtraNonce2Length variable expected to always be 4.
	ExtraNonce2Length uint
	Algorithm         Algorithm
	MinersCount       uint
}
