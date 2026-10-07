package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// BPCABF41 is the bpcabf41 module (bpcabf41_ope): CamX's bad pixel
// correction and adaptive Bayer filter (ABF), 4.1.0.
//
// No public XSD covers this version. The layout is inferred and accounts for
// every byte of all 371 records of the S5KJNS tuning (536 bytes each). The
// record follows the ABF 3.4 core struct of the older mm-camera Chromatix
// headers (chromatix_abf34_core_type), whose per-field comments give the
// names; the 8 trailing values look like a bad pixel correction set, and
// their names are guesses.
//
// The noise table is std = sqrt(k * bin) over 64 bins, bin = signal level /
// 64 on a 12-bit scale after black level, std in 12-bit counts, flat above
// bin 48: a shot noise model, which measures within 15% of the sensor's
// (10-bit variance = k / 256 * level, k / gain about 9.5 measured against
// 11 in the table). The filter strength anchors are levels, and the
// strengths between them are smaller for a stronger filter.
var BPCABF41 = TStruct(
	F("enable_section", TArray(3, TInt)),
	F("module_version", ModuleVersion),
	F("revision", TString),
	F("control_variables", TArray(3, ControlVar)),
	F("aec_exp_control", AECControl),
	F("aec_hdr_control", AECHDRControl),
	F("dynamic_enable_triggers", TArray(2, TStruct(
		F("enable", TInt),
		F("hyst_control_var", ControlVar),
		F("hyst_mode", TInt),
		F("hyst_trigger", TStruct(
			F("start1", TFloat), F("end1", TFloat),
			F("start2", TFloat), F("end2", TFloat),
		)),
	))),
	F("cross_channel_process_en", TInt),
	F("radial_anchor_table", TPacked(5, TFloat)),
	F("core", Region("drc_gain", Trigger,
		Region("hdr_aec", TriggerAECHDR,
			TList(BPCABF41Region)))),
)

// BPCABF41Region is one AEC region's parameters of bpcabf41.
var BPCABF41Region = TStruct(
	F("aec_trigger", TriggerAEC),
	F("bpc_fmax", TFloat),
	F("bpc_fmin", TFloat),
	F("bpc_maxshft", TFloat),
	F("bpc_minshft", TFloat),
	F("bpc_offset", TFloat),
	F("blk_pix_matching_rb", TFloat),
	F("blk_pix_matching_g", TFloat),
	F("noise_std_lut_level", TPacked(65, TFloat)),
	F("distance_ker", TPacked(6, TFloat)),
	F("curve_offset", TPacked(4, TFloat)),
	F("filter_strength_anchor_pt_lo", TFloat),
	F("filter_strength_anchor_pt_hi", TFloat),
	F("filter_strength_lo", TPacked(2, TFloat)),
	F("filter_strength_hi", TPacked(2, TFloat)),
	F("radial_adjust", TPacked(10, TFloat)),
	F("edge_softness", TFloat),
	F("hot_pixel_correction_disable", TFloat),
	F("cold_pixel_correction_disable", TFloat),
	F("same_channel_recovery", TFloat),
	F("bpc_fmax_q6", TFloat),
	F("bpc_fmin_q6", TFloat),
	F("bpc_offset_q6", TFloat),
	F("bcc_offset", TFloat),
	F("correction_threshold", TFloat),
)
