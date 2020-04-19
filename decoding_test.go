package ezpack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanEncodeDecodeStruct(t *testing.T) {
	type Child struct {
		Foo []byte  `ezpack:"foo,5"`
		Bar string  `ezpack:"bar,5"`
		Baz uint64  `ezpack:"baz"`
		Boz [0]byte `ezpack:"boz,5"`
		Bam [5]byte `ezpack:"bam,5"`
	}

	type Parent struct {
		Child Child `ezpack:"child"`
	}

	pt := Parent{
		Child: Child{
			Foo: []byte("bar"),
			Bar: "baz",
			Baz: (1 << 64) - 1,
			Boz: [0]byte{},
			Bam: [5]byte{0x77},
		},
	}

	enc, err := Encode(pt)
	require.NoError(t, err)

	var res Parent
	err = DecodeBytes(enc, &res)
	require.NoError(t, err)

	require.Equal(t, res, pt)
}

func TestArrayDecodingLengthIsExact(t *testing.T) {
	type Struct struct {
		Bam [5]byte `ezpack:"bam,5"`
	}

	type OtherStruct struct {
		Bam [4]byte `ezpack:"bam,5"`
	}

	os := OtherStruct{
		Bam: [4]byte{0x77},
	}

	enc, err := Encode(os)
	require.NoError(t, err)

	var res Struct
	err = DecodeBytes(enc, &res)
	require.Error(t, err)
}

func TestCanEncodeDecodeStructSlice(t *testing.T) {
	type Child struct {
		Foo []byte `ezpack:"foo,5"`
		Bar string `ezpack:"bar,5"`
		Baz uint64 `ezpack:"baz"`
	}

	type Parent struct {
		Children []Child `ezpack:"children,3"`
	}

	pt := Parent{
		Children: []Child{
			Child{[]byte("hi0"), "xyz", 1234},
			Child{[]byte("hi1"), "abc", 5678},
			Child{[]byte("hi2"), "def", 9012},
		},
	}

	enc, err := Encode(pt)
	require.NoError(t, err)

	var res Parent
	err = DecodeBytes(enc, &res)
	require.NoError(t, err)

	require.Equal(t, res, pt)
}

func TestCannotDecodeBeyondMaxLen(t *testing.T) {
	type Foo struct {
		X string `ezpack:"bar,5"`
	}

	type Bar struct {
		X string `ezpack:"bar,3"`
	}

	f := Foo{
		X: "hello",
	}

	enc, err := Encode(f)
	require.NoError(t, err)

	var foo Foo
	err = DecodeBytes(enc, &foo)
	require.NoError(t, err)

	var bar Bar
	err = DecodeBytes(enc, &bar)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max is 3")
}
