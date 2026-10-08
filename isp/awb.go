package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// AWBModuleID is the moduleID/subModuleID pair the AWB stats modules start
// with (awbglobalelements.xsd's moduleIDType).
var AWBModuleID = TStruct(F("module_id", TInt), F("sub_module_id", TInt))

// AWBPoint is a white point as the AWB statistics see it: the R/G and B/G
// ratios of a grey surface (awbglobalelements.xsd's awbPointDataType).
var AWBPoint = TStruct(F("rg", TFloat), F("bg", TFloat))

// RefPtNum is the number of AWB reference points, one per illuminant.
const RefPtNum = 15

// RefPtV1 is the AWB reference point module (modrefptv1.xsd): the white
// point of each of the algorithm's illuminants, in the order of CamX's
// StatsIlluminantType (see RefPtIlluminants). Unused points are zero.
var RefPtV1 = TStruct(
	F("module_id", AWBModuleID),
	F("module_version", ModuleVersion),
	F("enable", TInt),
	F("ref_pt", TArray(RefPtNum, AWBPoint)),
)

// RefPtIlluminants names the reference points, from CamX's
// StatsIlluminantType (chistatsinterfacedefs.h).
var RefPtIlluminants = []string{
	"High", "D75", "D65", "D50", "CW", "Fluorescent", "TL84",
	"Incandescent", "Horizon", "Low",
}

// RefPtCT is the colour temperature CamX's AWB (com.qti.stats.awb.so)
// gives each reference point on its daylight locus, by index into
// RefPtV1's points: its tables of illuminant CCTs and of locus segments
// chain 0-1-2-3-5-7-8-9, and join CW (4) and TL84 (6) in a separate
// fluorescent segment (4100 K to 3800 K), off the locus.
var RefPtCT = map[int]int{
	0: 12000, 1: 7500, 2: 6500, 3: 5000, 5: 4000, 7: 2850, 8: 2300, 9: 1500,
}

// SFIlluWVV1 is the AWB illuminant weight vector (no public XSD; layout from
// fogona's tuned files): for each lux index region, a weight on each range of
// colour temperature, how plausible an illuminant there is at that scene
// brightness. Ranges whose start equals their end are points. The two fields
// after enable are 0 in every file seen.
var SFIlluWVV1 = TStruct(
	F("module_id", AWBModuleID),
	F("module_version", ModuleVersion),
	F("enable", TInt),
	F("reserved", TArray(2, TInt)),
	F("lux_weights", TList(TStruct(
		F("trigger", Trigger),
		F("cct_weights", TList(TStruct(
			F("cct", Trigger),
			F("weight", TFloat),
		))),
	))),
)
