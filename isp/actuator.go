package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// Sensor module (com.qti.sensormodule.*.bin) types, from the structure of
// Qualcomm's camxactuatordriver.xsd and camxsensorcommon.xsd.

// Optional is how an element with minOccurs="0" is stored: like a list of
// zero or one element.
func Optional(t *Type) *Type { return TList(t) }

// i2cRegAddrDataType is a register address or data width, in bytes (1-4).
var i2cRegAddrDataType = TUint

var powerSetting = TStruct(
	F("configType", TUint), // MCLK, VANA, VDIG, VIO, VAF, RESET, STANDBY...
	F("configValue", TUint),
	F("delayMs", TUint),
)

// PowerSequenceInfo is a sequence of power settings.
var PowerSequenceInfo = TList(powerSetting)

var actuatorRegParam = TStruct(
	F("regAddrType", i2cRegAddrDataType),
	F("regDataType", i2cRegAddrDataType),
	F("registerAddr", TUint),
	F("registerData", TUint),
	F("operation", TEnum("WRITE_HW_DAMP", "WRITE_DAC_VALUE", "WRITE",
		"WRITE_DIR_REG", "POLL", "READ_WRITE")),
	F("delayUs", TUint),
	F("hwMask", TUint),
	F("hwShift", TUint),
	F("dataShift", TUint),
)

// RegSetting is one register write of a settings sequence.
var RegSetting = TStruct(
	F("slaveAddr", Optional(TUint)),
	F("registerAddr", TUint),
	F("registerData", TList(TUint)),
	F("regAddrType", i2cRegAddrDataType),
	F("regDataType", i2cRegAddrDataType),
	F("operation", TEnum("WRITE", "WRITE_BURST", "WRITE_SEQUENTIAL", "READ", "POLL")),
	F("delayUs", Optional(TUint)),
)

// ActuatorRegion maps lens steps to actuator codes.
var ActuatorRegion = TStruct(
	F("macroStepBoundary", TUint),
	F("infinityStepBoundary", TUint),
	F("codePerStep", TFloat),
	F("qValue", TUint),
)

var actuatorDamping = TStruct(
	F("ringingScenario", TUint),
	F("scenarioDampingParams", TList(TList(TStruct(
		F("dampingStep", TUint),
		F("dampingDelayUs", TUint),
		F("hwParams", TUint),
	)))),
)

// ActuatorDriver is the sensor module's actuatorDriver element.
//
// The fogona module's data has one slot more than the public XSD's
// ActuatorSlaveInfo, before dataBitWidth, and an optional readDacConfig in
// registerConfig; with those the schema covers the data exactly.
var ActuatorDriver = TStruct(
	F("module_version", ModuleVersion),
	F("actuatorName", TString),
	F("actuatorID", Optional(TUint)),
	F("slaveAddress", TUint),
	F("i2cFrequencyMode", TEnum("STANDARD", "FAST", "CUSTOM", "FAST_PLUS")),
	F("actuatorType", TEnum("VCM", "BIVCM")),
	F("unknown", TUint),
	F("dataBitWidth", TUint),
	F("powerUpSequence", PowerSequenceInfo),
	F("powerDownSequence", PowerSequenceInfo),
	F("registerParam", TList(actuatorRegParam)),
	F("readDacConfig", Optional(TUint)),
	F("initSettings", TList(RegSetting)),
	F("deInitSettings", Optional(TList(RegSetting))),
	F("initialCode", TInt),
	F("regionParams", TList(ActuatorRegion)),
	F("forwardDamping", actuatorDamping),
	F("backwardDamping", actuatorDamping),
)
