package isp

import . "github.com/W-Floyd/go-chromatix2libcamera/chromatix"

// MemoryInfo locates a value in the module EEPROM.
var MemoryInfo = TStruct(
	F("offset", TUint),
	F("mask", TUint),
	F("signedness", TEnum("SIGNED", "UNSIGNED")),
)

// EEPROMDriverDataPrefix is the start of the sensor module's
// EEPROMDriverData element (camxeepromdriver.xsd): the EEPROM's slave
// information and its auto focus memory map, which is all that is needed
// from it so far. Decode it with File.DecodePrefix.
var EEPROMDriverDataPrefix = TStruct(
	F("module_version", ModuleVersion),
	F("EEPROMName", TString),
	F("slaveAddress", TUint),
	F("regAddrType", i2cRegAddrDataType),
	F("regDataType", i2cRegAddrDataType),
	F("i2cFrequencyMode", TEnum("STANDARD", "FAST", "CUSTOM", "FAST_PLUS")),
	F("powerUpSequence", PowerSequenceInfo),
	F("powerDownSequence", PowerSequenceInfo),
	F("settings", TList(RegSetting)),
	// AFInfo
	F("afAvailable", TUint),
	F("afEndianness", TEnum("BIG", "LITTLE")),
	F("macro", MemoryInfo),
	F("infinity", MemoryInfo),
	F("calibrationInfo", TList(TStruct(
		F("chartDistanceCM", MemoryInfo),
		F("DACValue", MemoryInfo),
	))),
	F("hall", MemoryInfo),
	F("hallBias", MemoryInfo),
	F("hallRegisterAddr", TUint),
	F("verticalMacro", MemoryInfo),
	F("verticalInfinity", MemoryInfo),
	F("horizontalMacro", MemoryInfo),
	F("horizontalInfinity", MemoryInfo),
	F("macroMargin", TFloat),
	F("infinityMargin", TFloat),
)
