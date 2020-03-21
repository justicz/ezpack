package encoding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanEncodeDecodeStruct(t *testing.T) {
	type Child struct {
		Foo []byte `ezpack:"foo,5"`
		Bar string `ezpack:"bar,5"`
		Baz uint64 `ezpack:"baz"`
	}

	type Parent struct {
		Child Child `ezpack:"child"`
	}

	pt := Parent{
		Child: Child{
			Foo: []byte("bar"),
			Bar: "baz",
			Baz: (1 << 64) - 1,
		},
	}

	enc, err := Encode(pt)
	require.NoError(t, err)

	var res Parent
	err = Decode(enc, &res)
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
	err = Decode(enc, &foo)
	require.NoError(t, err)

	var bar Bar
	err = Decode(enc, &bar)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max is 3")
}
