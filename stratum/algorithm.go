package stratum

import (
	"errors"
)

type Algorithm string

const (
	SHA256d Algorithm = "sha256d"
	Scrypt  Algorithm = "scrypt"
)

func (a Algorithm) String() string {
	return string(a)
}

func (a Algorithm) hashFunc() func([]byte) []byte {
	switch a {
	case SHA256d:
		return sha256dHash
	case Scrypt:
		return scryptHash
	}
	panic("algorithm hash function not defined in switch above")
}

func ParseAlgorithm(s string) (Algorithm, error) {
	switch s {
	case SHA256d.String():
		return SHA256d, nil
	case Scrypt.String():
		return Scrypt, nil
	}
	return Algorithm("unknown"), errors.New("unknown algorithm")
}
