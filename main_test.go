package main

import (
	"fmt"
	"reflect"
	"testing"
)

func Test_uintToLeHex(t *testing.T) {
	tests := []struct {
		x     uint64
		width uint64
		want  string
	}{
		{0x1a, 1, "1a"},
		{0x1a2b, 2, "2b1a"},
		{0x1a2b3c4d, 4, "4d3c2b1a"},
		{0x1a2b3c4d5e6f7a8b, 8, "8b7a6f5e4d3c2b1a"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("_%d", i), func(t *testing.T) {
			if got := uintToLeHex(tt.x, tt.width); got != tt.want {
				t.Errorf("uintToLeHex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_binToHex(t *testing.T) {
	tests := []struct {
		bytes []byte
		want  string
	}{
		{[]byte{0, 1, 0xab, 0xcd, 'A'}, "0001abcd41"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("_%d", i), func(t *testing.T) {
			if got := binToHex(tt.bytes); got != tt.want {
				t.Errorf("binToHex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_decodeTargetBits(t *testing.T) {
	tests := []struct {
		bits       string
		wantTarget []byte
	}{
		{"1a01aa3d", hexToBin(
			"00000000000001aa3d0000000000000000000000000000000000000000000000")},
		{"207fffff", hexToBin(
			"7fffff0000000000000000000000000000000000000000000000000000000000")},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("_%d", i), func(t *testing.T) {
			if gotTarget := decodeTargetBits(tt.bits); !reflect.DeepEqual(gotTarget, tt.wantTarget) {
				t.Errorf("decodeTargetBits() = %v, want %v", gotTarget, tt.wantTarget)
			}
		})
	}
}

func Test_uintToVarIntHex(t *testing.T) {
	tests := []struct {
		x    uint64
		want string
	}{
		{0x1a, "1a"},
		{0x1a2b, "fd2b1a"},
		{0x1a2b3c, "fe3c2b1a00"},
		{0x1a2b3c4d, "fe4d3c2b1a"},
		{0x1a2b3c4d5e, "ff5e4d3c2b1a000000"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("_%d", i), func(t *testing.T) {
			if got := uintToVarIntHex(tt.x); got != tt.want {
				t.Errorf("uintToVarIntHex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_makeCoinBaseTx(t *testing.T) {
	want := "01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff2503ef98030400001059124d696e656420627920425443204775696c640800000037000011caffffffff01a0635c95000000001976a91427a1f12771de5cc3b73941664b2537c15316be4388ac00000000"

	coinbaseScript := "03ef98030400001059124d696e656420627920425443204775696c640800000037000011ca"
	address := "14cZMQk89mRYQkDEj8Rn25AnGoBi5H6uer"
	value := uint64(2505860000)

	got := makeCoinBaseTx(coinbaseScript, address, value, 0)

	if want != got {
		t.Log("want:", want)
		t.Log(" got:", got)
		t.Fatal("want not equal to got")
	}
}

func Test_computeMerkleRoot(t *testing.T) {
	type args struct {
		txHashes []string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{ // block testnet 00000000000001624cce24d8b32cb09ad5432a7173b905a06d048547f241a0b0
			name: "even hashes count",
			args: args{
				txHashes: []string{
					"decb39cfcc1d6b8e0155d14b8923dfc1b6cfe65bcc19a9f9e136b7a65c2ffb9d",
					"3e52c0e53c2c12b8d51cae5700273273f3968e7d9edc2c6e1093a74ba6fe7865",
					"0f7035239f5bd6759b5bfdf8f7cbfce0e7b382e105fa55af20a3994f439524c0",
					"d07fb5461d4a455270c06a6370708f6282256c2608a1937e54cd5db1f272657d",
				},
			},
			want: "07ecfbfa6214b1261daf058dbd226091d26acf8511c88f21b660a901cbc8179b",
		},
		{ // block testnet 00000000000000be13b52a46edd0e959e7785d569feb1b42ffc6eee7ae7caafa
			name: "odd hashes count",
			args: args{
				txHashes: []string{
					"1f4f9136b20069249115d55c843ec18acc1889b862cde134386e14396de7bdb3",
					"d731233d670af128e0a77dae8aa2b998f7515d0febe9aeb3ddfca0fff256e5b1",
					"874afe1c0e0dd76fcd4cd4f4a923a803d119e8612c70e6599c4e9cd98bb54084",
				},
			},
			want: "1489b66849671c4758d2b90d411e64b8e7ea5681ace1f60b9e053b94c7aef231",
		},
		{ // block regtest 63b2a02cce7888f359c82413f66de5cd4ad109fb91be7a19493df62551491975
			name: "single transaction",
			args: args{
				txHashes: []string{
					"aef812ae5d301be4bad82cd1881a7e0d735e081cb91f725c1fbc1ece83aba23c",
				},
			},
			want: "aef812ae5d301be4bad82cd1881a7e0d735e081cb91f725c1fbc1ece83aba23c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeMerkleRoot(tt.args.txHashes); binToHex(reverseBytes(got)) != tt.want {
				t.Errorf("computeMerkleRoot() = %v, want %v", binToHex(reverseBytes(got)), tt.want)
			}
		})
	}
}
