package ezpack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanEncodeUint64Fields(t *testing.T) {
	type Struct struct {
		Foo uint64 `ezpack:"foo"`
		Bar uint64 `ezpack:"bar"`
		Baz uint64 `ezpack:"baz"`
		Biz uint64 `ezpack:"biz"`
	}

	// Struct containing uint64 should be encodable
	s := Struct{
		Foo: (1 << 64) - 1,
		Bar: (1 << 32) - 1,
		Baz: 0,
		Biz: 1234,
	}

	_, err := Encode(s)
	require.NoError(t, err)
}

func TestCanEncodeStringFields(t *testing.T) {
	type Struct struct {
		Foo string `ezpack:"foo"`
	}

	// Struct containing string should be encodable
	s := Struct{
		Foo: "bar",
	}

	_, err := Encode(s)
	require.NoError(t, err)
}

func TestCanEncodeByteSliceFields(t *testing.T) {
	type Struct struct {
		Foo []byte `ezpack:"foo"`
	}

	// Struct containing byte slice should be encodable
	s := Struct{
		Foo: []byte("bar"),
	}

	_, err := Encode(s)
	require.NoError(t, err)
}

func TestCanEncodedNestedStructs(t *testing.T) {
	type Child struct {
		Foo []byte `ezpack:"foo"`
	}

	type Parent struct {
		Child Child `ezpack:"child"`
	}

	// Struct containing struct should be encodable
	pt := Parent{
		Child: Child{
			Foo: []byte("bar"),
		},
	}

	_, err := Encode(pt)
	require.NoError(t, err)
}

func TestCannotEncodeWithBadStructTags(t *testing.T) {
	type NoTag struct {
		Foo []byte
	}

	// Struct with no struct tag on field should not be encodable
	nt := NoTag{}
	_, err := Encode(nt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "struct tag")

	type BadTag struct {
		Foo []byte `ezpack:noquotes`
	}

	// Struct with bad struct tag on field should not be encodable
	bt := BadTag{}
	_, err = Encode(bt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "struct tag")

	type DupTag struct {
		Foo []byte `ezpack:"dup"`
		Bar []byte `ezpack:"dup"`
	}

	// Struct with duplicate keys should not be encodable
	dt := DupTag{}
	_, err = Encode(dt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate key")
}
