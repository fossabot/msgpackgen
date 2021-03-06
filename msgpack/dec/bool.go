package dec

import (
	"github.com/shamaton/msgpack/def"
)

func (d *Decoder) AsBool(offset int) (bool, int, error) {
	code := d.data[offset]
	offset++

	switch code {
	case def.True:
		return true, offset, nil
	case def.False:
		return false, offset, nil
	}
	return false, 0, d.errorTemplate(code, "AsBool")
}
