package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// CV12 is the colour conversion module (cv_1_2_0.xsd, cv12_ope): RGB to
// YCbCr after gamma, with chroma enhancement. By mm-camera's chromatix.h,
//
//	Cb = a * ((B - G) + b * (R - G)) + KCb
//	Cr = c * (d * (B - G) + (R - G)) + KCr
//
// with a, b, c and d each taken from its p(ositive) or m(inus) value by
// the sign of the result. a = c = 0.5, b = -0.338, d = -0.162 are BT.601's
// Cb and Cr.
var CV12 = TStruct(
	F("enable", TInt),
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
					TList(TStruct(
						F("cct_trigger", Trigger),
						F("r_to_y", TFloat), F("g_to_y", TFloat), F("b_to_y", TFloat),
						F("y_offset", TFloat),
						F("ap", TFloat), F("am", TFloat),
						F("bp", TFloat), F("bm", TFloat),
						F("cp", TFloat), F("cm", TFloat),
						F("dp", TFloat), F("dm", TFloat),
						F("kcr", TFloat), F("kcb", TFloat),
					))))))),
)

// CV12 BT.601 values: a, c (chroma scale) and b, d (shear).
const (
	CVScaleNeutral = 0.5
	CVShearB       = -0.338
	CVShearD       = -0.162
)
