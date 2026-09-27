package isp

import (
	"strings"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
)

// schemas maps a module's base name (the part before the hardware block
// suffix, e.g. "gamma15" for gamma15_ope) and version to its schema.
var schemas = map[string]map[uint64]*chromatix.Type{
	"gamma15": {version(1, 5, 0): Gamma15},
	"cc13":    {version(1, 3, 0): CC13},
	"cv12":    {version(1, 2, 0): CV12},
	"lsc34":   {version(3, 4, 0): LSC34},
	"lsc35":   {version(3, 5, 0): LSC34},
	// No public XSD; see DemuxBlkLevel14.
	"demuxblklevel14": {version(1, 4, 0): DemuxBlkLevel14},
}

// moduleSchemas are the sensor module file's top-level elements and the
// tuned file's stats modules, which carry no hardware block suffix.
var moduleSchemas = map[string]*chromatix.Type{
	"actuatorDriver": ActuatorDriver,
	"refPtV1":        RefPtV1,
}

func version(major, minor, incr uint64) uint64 {
	return major | minor<<16 | incr<<32
}

// Schema returns the schema for a top-level module, if one is known.
func Schema(m chromatix.Symbol) (*chromatix.Type, bool) {
	if t, ok := moduleSchemas[m.Name]; ok {
		return t, true
	}
	base := m.Name
	if i := strings.LastIndexByte(base, '_'); i > 0 {
		base = base[:i]
	}
	t, ok := schemas[base][m.Version]
	return t, ok
}
