package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// The asf32 schema registers itself here, so that this file holds the whole
// of it.
func init() {
	schemas["asf32"] = map[uint64]*Type{version(3, 2, 0): ASF32}
}

// ASF32 is the asf32 module (asf32_ope): CamX's adaptive spatial filter,
// 3.2.0, the OPE's luma sharpening after bpcabf41 and wnr24.
//
// No public XSD covers this version. The layout is inferred. It accounts for
// every byte of all 71 modules and 802 region records of the S5KJNS and
// OV08F tunings (200-byte header, 2024-byte records), and the one pad word
// is zero in every record. The record is the symmetric-filter-only 7x7
// subset of the mm-camera CPP's 9x9 ASF (chromatix_asf_9_9_core_type in the
// 0x0310 chromatix_cpp.h). That struct gives the field names, and its
// per-field comments give the ranges and effects. The fields keep its order,
// minus the directional filters, the activity filters and the dynamic clamp.
// The 7x7 kernels are the last 10 of the 9x9's 15 coefficients, and the
// tuning's second HPF and its LPF are exactly the 9x9 sample tuning's
// coefficients with the outer ring dropped.
//
// The model, by the CPP comments: detail = HPF(Y), with a sum of 0. Activity
// is |band-pass(Y)| (L1), clamped, then scaled by activity_normalization_lut
// at the pixel's level. The gain is gain_positive_lut or gain_negative_lut
// at the level (by the detail's sign), times gain_weight_lut at the
// activity, times the radial factor, capped by gain_cap. The result is added
// to Y within [clamp_ll, clamp_ul]. Level is 8-bit gamma-corrected luma in
// 64 bins.
var ASF32 = TStruct(
	F("enable", TInt),
	F("module_version", ModuleVersion),
	F("revision", TString),
	F("control_variables", TArray(4, ControlVar)),
	F("aec_exp_control", AECControl),
	F("aec_hdr_control", AECHDRControl),
	// Same 7-slot block as bpcabf41's dynamic_enable_triggers; disabled.
	F("dynamic_enable_trigger", TStruct(
		F("enable", TInt),
		F("hyst_control_var", ControlVar),
		F("hyst_mode", TInt),
		F("hyst_trigger", TStruct(
			F("start1", TFloat), F("end1", TFloat),
			F("start2", TFloat), F("end2", TFloat),
		)),
	)),
	// The radius fractions of the 4 radial adjust points: 0.4, 0.6, 0.8,
	// 1.0 (corner).
	F("radial_pt_table", TPacked(4, TFloat)),
	// 1 and 1 everywhere. The names are a guess: the two flags of the 9x9
	// ASF, which are both 1 in its sample tuning.
	F("use_only_symmetric_filter", TInt),
	F("radial_enable", TInt),
	F("core", Region("total_scale_ratio", Trigger,
		Region("drc_gain", Trigger,
			Region("hdr_aec", TriggerAECHDR,
				Region("aec", TriggerAEC, ASF32Region))))),
)

// ASF32Region is one AEC region's parameters of asf32 (249 slots).
var ASF32Region = TStruct(
	// The 7x7 sharpening HPF, octant-symmetric, Q10, sum 0. The coefficients
	// are the upper-left triangle in raster order: (dx,dy) = (3,3) (2,3)
	// (1,3) (0,3) (2,2) (1,2) (0,2) (1,1) (0,1), and the centre last.
	F("hpf_symmetric_coeff", TPacked(10, TFloat)),
	// The 7x7 LPF in the same order, Q10, sum 1024: the separable tent
	// [1 2 3 4 3 2 1] / 16 in each direction.
	F("lpf_coeff", TPacked(10, TFloat)),
	// Level-based, 0..0.996: scales activity per luma bin. 0 makes the bin
	// "flat", so it gets no sharpening there.
	F("activity_normalization_lut", TPacked(64, TFloat)),
	// Activity-based, 0..0.996; 0.996 (neutral) in every record.
	F("weight_modulation_lut", TPacked(64, TFloat)),
	// Level-based, 0..255, where a larger value lets more of the detail
	// through; 255 (no coring) in every record.
	F("soft_threshold_lut", TPacked(64, TFloat)),
	// Level-based sharpening gains (0..7.96) for positive and negative
	// detail, i.e. overshoot and undershoot. Low in the shadows, rising to a
	// plateau by bin ~24.
	F("gain_positive_lut", TPacked(64, TFloat)),
	F("gain_negative_lut", TPacked(64, TFloat)),
	// Activity-based gain weight, 0..0.996: a sigmoid from a floor (0..0.5)
	// up to 0.996. This is the coring against noise: its knee moves to
	// higher activity as the sensor gain rises.
	F("gain_weight_lut", TPacked(64, TFloat)),
	F("gamma_corrected_luma_target", TFloat), // 128 everywhere
	F("gain_cap", TFloat),                    // upper cap of the gain
	F("clamp_ul", TFloat),                    // max overshoot, 8-bit luma
	F("clamp_ll", TFloat),                    // max undershoot (negative)
	F("activity_clamp_threshold", TFloat),    // 255 everywhere
	// 0 everywhere. The names are a guess (L2_norm_en is the other
	// candidate).
	F("median_blend_upper_offset", TFloat),
	F("median_blend_lower_offset", TFloat),
	// The 3x3 median smoothing percentage, 0..100. It is 0, or 5-50 at high
	// gain.
	F("sp", TFloat),
	// Factors at radial_pt_table's points, 1 at the centre and falling
	// outward (e.g. 1, 0.951, 0.905, 0.861).
	F("radial_activity_adj_factor", TPacked(4, TFloat)),
	F("radial_gain_adj_factor", TPacked(4, TFloat)),
	// Unknown. Two identical 32-entry halves, 1.0 in 777 of 802 records,
	// otherwise falling from 1 to 0.25: a gain attenuation by some 32-bin
	// index.
	F("unknown_lut", TPacked(64, TFloat)),
	// Unknown. Five equal values, 153 (or 255 in some OV08F regions).
	F("unknown_tail", TPacked(5, TFloat)),
)
