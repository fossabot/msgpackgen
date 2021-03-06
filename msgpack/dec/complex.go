package dec

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/shamaton/msgpack/def"
)

// todo : def
const complexType = -128

func (d *Decoder) AsComplex64(offset int) (complex64, int, error) {
	code, offset := d.readSize1(offset)

	switch code {
	case def.Fixext8:
		t, offset := d.readSize1(offset)
		if int8(t) != complexType {
			return 0, 0, fmt.Errorf("fixext8. complex type is diffrent %d, %d", t, complexType)
		}
		rb, offset := d.readSize4(offset)
		ib, offset := d.readSize4(offset)
		r := math.Float32frombits(binary.BigEndian.Uint32(rb))
		i := math.Float32frombits(binary.BigEndian.Uint32(ib))
		return complex(r, i), offset, nil
	}

	return 0, 0, d.errorTemplate(code, "AsComplex64")
}

func (d *Decoder) AsComplex128(offset int) (complex128, int, error) {
	code, offset := d.readSize1(offset)

	switch code {
	case def.Fixext16:
		t, offset := d.readSize1(offset)
		if int8(t) != complexType {
			return 0, 0, fmt.Errorf("fixext16. complex type is diffrent %d, %d", t, complexType)
		}
		rb, offset := d.readSize8(offset)
		ib, offset := d.readSize8(offset)
		r := math.Float64frombits(binary.BigEndian.Uint64(rb))
		i := math.Float64frombits(binary.BigEndian.Uint64(ib))
		return complex(r, i), offset, nil
	}

	return 0, 0, d.errorTemplate(code, "AsComplex128")
}
