package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// CC13 is the cc13 colour correction module (cc13_ope, cc13_ipe, ...): a
// 3x3 matrix and per-channel offsets, selected by DRC gain, HDR ratio,
// flash LED index, exposure and colour temperature.
var CC13 = TStruct(
	F("cc_enable", TInt),
	F("module_version", ModuleVersion),
	F("revision", TString),
	F("control_variables", TArray(5, ControlVar)),
	F("aec_exp_control", AECControl),
	F("aec_hdr_control", AECHDRControl),
	F("led_sensitivity_trigger", Trigger),
	F("q_factor", TInt),
	F("core", Region("drc_gain", Trigger,
		Region("hdr_aec", TriggerAECHDR,
			Region("led_idx", TriggerLEDIdx,
				Region("aec", TriggerAEC,
					TList(TStruct(
						F("cct_trigger", Trigger),
						F("c", TPacked(9, TFloat)),
						F("k", TPacked(3, TInt)),
					))))))),
)
