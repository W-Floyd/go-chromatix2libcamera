package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// AWBTriggerRGB is a gain adjustment at a trigger range
// (awbglobalelements.xsd's awbTriggerDataRGBType).
var AWBTriggerRGB = TStruct(
	F("trigger", Trigger),
	F("red", TFloat), F("green", TFloat), F("blue", TFloat),
)

// TriglGAV1 is the AWB gain adjustment module (modtrigladjv1.xsd): a mesh of
// triangles over R/G, B/G whose vertices carry gain adjustments by lux
// index, and a table of gain adjustments by lux index and colour
// temperature (cctAdjGainData).
var TriglGAV1 = TStruct(
	F("module_id", AWBModuleID),
	F("module_version", ModuleVersion),
	F("enable", TInt),
	F("triangles", TList(TStruct(
		F("vertex_idx", TPacked(3, TInt)),
		F("side_idx", TPacked(3, TInt)),
	))),
	F("vertices", TList(TStruct(
		F("point", AWBPoint),
		F("gain_adj_trigger", Trigger),
		F("gain_adj", TList(AWBTriggerRGB)),
	))),
	F("num_start_positions", TInt),
	F("start_triangle_idx", TPacked(4, TInt)),
	F("cct_adj_trigger", Trigger),
	F("cct_adj", TList(TStruct(
		F("trigger", Trigger),
		F("data", TList(AWBTriggerRGB)),
	))),
)
