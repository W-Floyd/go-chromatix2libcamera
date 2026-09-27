package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// TMC12Knees is the number of TMC 1.2 knee points (TMC12_KNEE_POINTS_NUM).
const TMC12Knees = 5

// TMC12 is the software tone mapping control module (tmc_1_2_0.xsd,
// tmc12_sw): CamX's ADRC. From the DRC gain AEC exposed the frame under by,
// it makes knee points, from tone anchors and from the frame's histogram,
// through which a curve (Bezier or PCHIP, curve_model) is the gain applied
// by GTM (gtm_percentage of it) and LTM (ltm_percentage); CamX's
// tmc12interpolation.cpp.
var TMC12 = TStruct(
	F("enable", TInt),
	F("module_version", ModuleVersion),
	F("revision", TString),
	F("control_variables", TArray(3, ControlVar)),
	F("aec_exp_control", AECControl),
	F("aec_hdr_control", AECHDRControl),
	F("use_gtm", TInt),
	F("use_ltm", TInt),
	F("curve_model", TInt),
	F("curve_order", TInt),
	F("tone_max_ratio", TFloat),
	F("reserve_rsv_para", TArray(5, TFloat)),
	F("core", Region("drc_gain", Trigger,
		Region("hdr_aec", TriggerAECHDR,
			Region("aec", TriggerAEC, TMC12Region)))),
)

// TMC12Region is tmc12_rgn_dataType.
var TMC12Region = TStruct(
	F("gtm_percentage", TFloat),
	F("ltm_percentage", TFloat),
	F("dark_boost_ratio", TFloat),
	F("dark_boost_offset", TFloat),
	F("tone_target", TPacked(TMC12Knees, TFloat)),
	F("tone_anchors", TPacked(TMC12Knees, TFloat)),
	F("tone_bright_adj", TFloat),
	F("tone_dark_adj", TFloat),
	F("stretch_bright_str", TFloat),
	F("stretch_dark_str", TFloat),
	F("hist_supr_range_start", TFloat),
	F("hist_supr_range_end", TFloat),
	F("hist_boost_range_start", TFloat),
	F("hist_boost_range_end", TFloat),
	F("hist_avg_range_start", TFloat),
	F("hist_avg_range_end", TFloat),
	F("hist_conv_kernel", TPacked(TMC12Knees, TFloat)),
	F("hist_clip_slope", TFloat),
	F("hist_enhance_clamp", TFloat),
	F("hist_enhance_ratio", TPacked(TMC12Knees, TFloat)),
	F("contrast_bright_clip", TFloat),
	F("contrast_dark_adj", TFloat),
	F("contrast_he_bright", TFloat),
	F("contrast_he_dark", TFloat),
	F("hist_smoothing_str", TFloat),
	F("hist_curve_smoothing_str", TFloat),
	F("scene_change_smoothing_str", TFloat),
	F("scene_change_curve_smoothing_str", TFloat),
	F("scene_change_hist_delta_th1", TFloat),
	F("scene_change_hist_delta_th2", TFloat),
	F("scene_change_lux_idx_delta_th1", TFloat),
	F("scene_change_lux_idx_delta_th2", TFloat),
	F("core_rsv_para", TArray(5, TFloat)),
)
