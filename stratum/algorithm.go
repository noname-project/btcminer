package stratum

type Algorithm int

const (
	SHA256d Algorithm = iota
	Scrypt
)

func (a Algorithm) hashFunc() func([]byte) []byte {
	switch a {
	case SHA256d:
		return sha256dHash
	case Scrypt:
		return scryptHash
	}
	panic("algorithm hash function not defined in switch above")
}
