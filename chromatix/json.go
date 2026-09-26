package chromatix

import (
	"bytes"
	"encoding/json"
)

// MarshalJSON writes the object's fields in schema order.
func (o *Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, n := range o.Names {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(n)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(o.Values[i])
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
