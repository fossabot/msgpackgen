package enc

import (
	"math"

	"github.com/shamaton/msgpack/def"
)

func (e *Encoder) CalcString(v string) int {
	l := len(v)
	if l < 32 {
		return def.Byte1 + l
	} else if l <= math.MaxUint8 {
		return def.Byte1 + def.Byte1 + l
	} else if l <= math.MaxUint16 {
		return def.Byte1 + def.Byte2 + l
	}
	return def.Byte1 + def.Byte4 + l
	// NOTE : length over uint32
}

func (e *Encoder) WriteString(str string, offset int) int {
	l := len(str)
	if l < 32 {
		offset = e.setByte1Int(def.FixStr+l, offset)
	} else if l <= math.MaxUint8 {
		offset = e.setByte1Int(def.Str8, offset)
		offset = e.setByte1Int(l, offset)
	} else if l <= math.MaxUint16 {
		offset = e.setByte1Int(def.Str16, offset)
		offset = e.setByte2Int(l, offset)
	} else {
		offset = e.setByte1Int(def.Str32, offset)
		offset = e.setByte4Int(l, offset)
	}
	offset += copy(e.d[offset:], str)
	return offset
}
