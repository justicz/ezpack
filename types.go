package encoding

const (
	PackMapID    = 0xDF
	PackUint64ID = 0xCF
	PackBytesID  = 0xC6
	PackStringID = 0xDB
)

type PackValue interface {
	Encode() ([]byte, error)
}

type PackUint64 struct {
	Value uint64
}

type PackBytes struct {
	Bytes []byte
}

type PackString struct {
	String string
}

type PackMapElement struct {
	Key   PackString
	Value PackValue
}

type PackMap struct {
	Elements []PackMapElement
}
