package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// lscGrid is the number of points in a lens shading gain table: a 17x13
// grid spanning the image.
const lscGrid = 17 * 13

// lscTables are a lens shading region's gain tables, one per Bayer channel.
var lscTables = []Field{
	F("r_gain", TPacked(lscGrid, TFloat)),
	F("gr_gain", TPacked(lscGrid, TFloat)),
	F("gb_gain", TPacked(lscGrid, TFloat)),
	F("b_gain", TPacked(lscGrid, TFloat)),
}

// LSC34 is the lens shading correction (rolloff) module, lsc_3_4_0.xsd's
// chromatix_lsc34Type (lsc34_ife, and lsc35_tfe, which has the same
// layout): gain tables selected by lens position, DRC gain, HDR ratio,
// flash LED index, exposure and colour temperature, and golden module
// tables by colour temperature.
var LSC34 = TStruct(
	F("rolloff_enable", TInt),
	F("module_version", ModuleVersion),
	F("revision", TString),
	F("control_variables", TArray(6, ControlVar)),
	F("aec_exp_control", AECControl),
	F("aec_hdr_control", AECHDRControl),
	F("led_sensitivity_trigger", Trigger),
	F("core", Region("lens_position", Trigger,
		Region("drc_gain", Trigger,
			Region("hdr_aec", TriggerAECHDR,
				Region("led_idx", TriggerLEDIdx,
					Region("aec", TriggerAEC,
						TList(TStruct(append([]Field{
							F("cct_trigger", Trigger),
						}, lscTables...)...)))))))),
	F("golden", TList(TStruct(append([]Field{
		F("cct_trigger", Trigger),
	}, lscTables...)...))),
)
