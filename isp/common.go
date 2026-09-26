// Package isp holds schemas for Chromatix ISP tuning modules, written from
// the structure of Qualcomm's published Chromatix XSDs and checked against
// real parameter files: every schema must account for its module's data
// exactly, or decoding fails.
package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// ControlVar is the control variable enumeration used by trigger regions.
var ControlVar = TEnum(
	"lux_idx", "gain", "drc_gain", "exp_time_ratio",
	"aec_exp_sensitivity_ratio", "exp_gain_ratio", "cct",
	"total_scale_ratio", "lens_position", "led_idx", "led_sensitivity",
	"post_scale_ratio", "effective_gain", "lens_zoom", "pre_scale_ratio",
	"pass",
)

// Channel is the colour channel enumeration.
var Channel = TEnum("G", "B", "R", "RGB")

// Trigger is a [start, end] region of one control variable, with
// interpolation between neighbouring regions' data in the gaps.
var Trigger = TStruct(F("start", TFloat), F("end", TFloat))

// TriggerAEC is a region in both gain and lux index; which one applies is
// the module's aec_exp_control.
var TriggerAEC = TStruct(
	F("lux_idx_start", TFloat), F("lux_idx_end", TFloat),
	F("gain_start", TFloat), F("gain_end", TFloat),
)

// TriggerAECHDR is a region in the three HDR ratios; which one applies is
// the module's aec_hdr_control.
var TriggerAECHDR = TStruct(
	F("exp_time_start", TFloat), F("exp_time_end", TFloat),
	F("aec_sensitivity_start", TFloat), F("aec_sensitivity_end", TFloat),
	F("exp_gain_start", TFloat), F("exp_gain_end", TFloat),
)

// TriggerLEDIdx selects data by flash LED index.
var TriggerLEDIdx = TInt

// ModuleVersion is the major/minor/incr revision every module starts with.
var ModuleVersion = TStruct(F("major", TInt), F("minor", TInt), F("incr", TInt))

// AECControl and AECHDRControl name the control variable an AEC or HDR
// trigger region is evaluated in.
var (
	AECControl    = TEnum("lux_idx", "gain")
	AECHDRControl = TEnum("exp_time_ratio", "aec_exp_sensitivity_ratio", "exp_gain_ratio")
)

// Region nests one trigger level: a list of (trigger, next level) pairs.
func Region(name string, trigger, next *Type) *Type {
	return TList(TStruct(F(name+"_trigger", trigger), F(name+"_data", next)))
}
