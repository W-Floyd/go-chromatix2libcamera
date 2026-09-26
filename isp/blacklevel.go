package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// DemuxBlkLevel14 is the demuxblklevel14 module (demuxblklevel14_tfe) of the
// TFE ("thin front end") found on SM6225-class SoCs: Bayer demux channel
// gains and the per-channel black level subtracted at the front end.
//
// No public XSD covers this module. The layout is inferred: it follows the
// common Chromatix module template (enable, version, revision, control
// variables and method, LED trigger, trigger-region core), with four floats
// before the core and four per-channel black levels in each colour
// temperature region, which accounts for the data exactly. The black levels
// are in 12-bit units: on sensors with a 10-bit pedestal of 64 they read
// ~252.
var DemuxBlkLevel14 = TStruct(
	F("enable", TInt),
	F("module_version", ModuleVersion),
	F("revision", TString),
	F("control_variables", TArray(5, ControlVar)),
	F("aec_exp_control", AECControl),
	F("aec_hdr_control", AECHDRControl),
	F("led_sensitivity_trigger", Trigger),
	F("gain", TArray(4, TFloat)),
	F("core", Region("drc_gain", Trigger,
		Region("hdr_aec", TriggerAECHDR,
			Region("led_idx", TriggerLEDIdx,
				Region("aec", TriggerAEC,
					TList(TStruct(
						F("cct_trigger", Trigger),
						F("black_level", TArray(4, TFloat)), // R, Gr, Gb, B
					))))))),
)

// DemuxBlkLevelBits is the bit depth of DemuxBlkLevel14's black levels.
const DemuxBlkLevelBits = 12
