package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// GammaTableLen is the number of points in a gamma15 table: evenly spaced
// over the input range, first and last included.
const GammaTableLen = 257

// Gamma15 is the gamma15 module (gamma15_ope, gamma15_ipe): per-channel
// 257-point output curves, selected by DRC gain, HDR ratio, flash LED
// index, exposure and colour temperature.
var Gamma15 = TStruct(
	F("gamma_enable", TInt),
	F("module_version", ModuleVersion),
	F("revision", TString),
	F("control_variables", TArray(5, ControlVar)),
	F("aec_exp_control", AECControl),
	F("aec_hdr_control", AECHDRControl),
	F("led_sensitivity_trigger", Trigger),
	F("core", Region("drc_gain", Trigger,
		Region("hdr_aec", TriggerAECHDR,
			Region("led_idx", TriggerLEDIdx,
				Region("aec", TriggerAEC,
					Region("cct", Trigger,
						TList(TStruct(
							F("channel", Channel),
							F("table", TPacked(GammaTableLen, TFloat)),
						)))))))),
)
