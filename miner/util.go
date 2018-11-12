package miner

import (
	"crypto/sha256"
	"encoding/binary"

	"gitlab.com/samli88/go-x11-hash"
	"golang.org/x/crypto/scrypt"
)

// sha256dHash is double sha256 hashing function
func sha256dHash(data []byte) []byte {
	h1 := sha256.Sum256(data)
	h2 := sha256.Sum256(h1[:])
	return h2[:]
}

// scryptHash is scrypt hashing function used in litecoin
func scryptHash(data []byte) []byte {
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

func x11Hash(data []byte) []byte {
	out := make([]byte, 32)
	x11.New().Hash(data, out)
	return out
}

func restorePrevHashByteOrder(prevHash []byte) []byte {
	restored := make([]byte, len(prevHash))

	for i := 0; i < len(prevHash); i = i + 4 {
		copy(restored[len(prevHash)-i-4:len(prevHash)-i], prevHash[i:i+4])
	}

	return restored
}

// reverseBytes reverse bytes order
func reverseBytes(bytes []byte) {
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
}

// reverseBytes reverse bytes order
func reverseBytesCopy(bytes []byte) []byte {
	bytes2 := make([]byte, len(bytes))
	copy(bytes2, bytes)
	reverseBytes(bytes2)
	return bytes2
}

// uint32ToLeBytes converts uint32 hex string to little-endian bytes
func uint32ToLeBytes(i uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, i)
	return bytes
}
