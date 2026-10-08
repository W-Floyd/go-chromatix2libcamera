# go-chromatix2libcamera

Reads Qualcomm CamX Chromatix tuning files (`com.qti.tuned.*.bin`,
`com.qti.sensormodule.*.bin`, under `/vendor/lib64/camera` on Qualcomm Android
devices) and writes what libcamera can use as libcamera tuning files for the
software ISP.

It is meant to run **on the device**, against the device's own vendor
partition (for example at boot), so that no vendor calibration is ever
distributed: the output is derived from, and stays with, the hardware it
describes. Generated files say so in their header.

## Usage

```sh
go-chromatix2libcamera modules  com.qti.tuned.<sensor>.bin   # list tuning modules
go-chromatix2libcamera symbols  -name 'cc13' <file>          # list parameter elements
go-chromatix2libcamera hex      <file> <id|name>             # raw data, as 32-bit words
go-chromatix2libcamera decode   <file> gamma15_ope           # decode a module, as JSON
go-chromatix2libcamera convert  -module com.qti.sensormodule.<module>.bin \
                                com.qti.tuned.<sensor>.bin > <sensor>.yaml
```

`convert` writes the front-end black level (`demuxblklevel14`) as libcamera's
`BlackLevel`, the AWB white points (`refPtV1`) as `Awb`'s `colourGains`, the
colour correction matrices (`cc13`) as `Ccm`, lens shading (`lsc34`/`lsc35`)
as `Lsc`, the output curve (`gamma15`) as `Adjust`'s `toneCurve`, the AEC
luma targets (`Metering`) as `Agc`'s `meanTarget` and `histogram` (the brightest
zone) and, by lux index, `meanTargetLux` and `histogramLux`, the AWB
illuminant weights (`SFIlluWVV1`) as `Awb`'s `ctPrior` by lux index (not
yet read by libcamera), and with
`-module` the lens actuator's travel as `Af`'s `map`. Chromatix selects most tables by exposure
(and DRC gain, flash, ...) as well as colour temperature, while libcamera's
software ISP indexes by colour temperature only, so one exposure region is
chosen: `-lux-idx` (Qualcomm lux index, higher is darker; default 250,
ordinary indoor light) and `-drc-gain` (default 1).

The colour correction matrices of every lux region are also written, as
`Ccm`'s `luxSets`, with a lux index model for `Agc` (`lux`): the lux index is
the exposure index of CamX's exposure table, which the tuning's
`expKneeEntries` gives at nine knees of gain and exposure time, and which is
23.45 index steps (a step is 3% of sensitivity) per stop of gain x exposure
time. `-no-lux` leaves both out; `-lux-ref-product` and `-lux-per-stop`
override the table's. The soft ISP needs the `pdaf` branch's lux selection
("ipa: soft: Select the colour correction by lux index") to use them.

`toneCurve` is not in upstream libcamera: it needs the software ISP change on
the `softisp-af` branch of github.com/W-Floyd/libcamera ("software_isp: Apply
the tone curve from a lookup table"). Without it, libcamera ignores the key.

## Generating a device's tuning (`generate`)

On the device, typically at boot once the vendor partition is mounted,
`generate` writes the tuning file of every camera a small per-device JSON
config describes (default `/etc/go-chromatix2libcamera.json`) to
`/etc/libcamera/ipa/soft/<sensor>.yaml`:

```json
{
  "vendor": "/run/msm-firmware-loader/mnt/vendor*/lib64/camera",
  "cameras": [
    { "sensor": "s5kjn1",
      "tuned": "com.qti.tuned.mot_fogona_s5kjns.bin",
      "module": "com.qti.sensormodule.mot_fogona_s5kjns_qtech.bin",
      "eeprom": "i2c-bus@0/eeprom@50" },
    { "sensor": "ov08f10", "tuned": "com.qti.tuned.mot_ov08f.bin" }
  ]
}
```

`sensor` is libcamera's name for the sensor, which the tuning file is named
after. `vendor` is a glob; the first directory matching it that holds the
tuned file is used. `eeprom` is the module EEPROM's device tree node, matched
against the end of each nvmem device's `of_node` path, because the I2C bus
numbers, and so the nvmem devices' names, change between boots. Without the
EEPROM, or when it does not decode, the camera still gets the vendor's
golden-module tuning, with a note on stderr.

A file is written, atomically, only when its content changes, so that
running `generate` on every boot costs nothing, and a vendor update flows
through. `-n` reports what would change; `-output` writes elsewhere.

Reading the EEPROM is slow (2.5 s over I2C on fogona), so each file has a
stamp beside it (`.<sensor>.stamp`): a hash of the camera's config, the
Chromatix files' sizes and times, and the program itself. When the stamp
matches and the file exists, the run stops there without touching the EEPROM
(`up to date`). The EEPROM is not in the stamp: a module swapped for another
needs `-f`.

## Per-unit calibration (module EEPROM)

Camera modules carry factory calibration in an EEPROM, and CamX uses it in
place of parts of the tuning: notably, the actuator tuning's step table is a
placeholder that CamX rescales to the EEPROM's infinity and macro codes. With
`-eeprom <image>` (and `-module`), `convert` reads the image and builds `Af`'s
map from the calibration: calibration points at known distances where there
are some (a least-squares line through them, codes against dioptres, also
places the macro code), extended at the macro end by the module's margin.

EEPROM layouts are vendor-specific, so they sit behind the `calibration.Format`
interface (`Detect`, `Parse`): vendor formats are tried first, and CamX's
generic layout, driven by the sensor module's `EEPROMDriverData` memory map, is
the fallback.

| format | detected by | provides |
|---|---|---|
| `motorola-gt24p256c` | EEPROM name `mot_gt24p256c_s5kjns_eeprom`, size, CRC-16/BUYPASS of the module information block | AF infinity and macro codes; four AF calibration points (distance, code); PDAF gain maps and DCC; white balance (unit and golden R/G, B/G under D50, which scale `Awb`'s white points); lens shading (a 17×13 flat field per channel under D50: `Lsc`'s gains are scaled by the tuning's golden module levels over it, as CamX does). Each block is CRC-checked, and a bad block drops only that block's data |
| `camx-generic` | `EEPROMDriverData` marks AF data available | AF infinity and macro codes at the memory map's offsets |

## The format

A Chromatix parameter file is self-describing: a header, a section table, a
symbol table naming every parameter element (id, name, module version and
group, data offset and size), the data, and an index of the top-level modules.
In the data, each leaf value takes an 8-byte slot, nested structures and
fixed-length sequences are inline, strings and variable-length sequences are
`(count, symbol id)` references, and fixed-length number arrays are packed at 4
bytes per value. See `chromatix/chromatix.go` and `chromatix/schema.go`.

What a module's inline fields mean is defined by its schema. The schemas in
`isp/` are written from the structure of Qualcomm's published Chromatix XSDs
and checked against real files: a schema that does not account for a module's
data exactly fails to decode.

| module | version | decoded | converted to |
|---|---|---|---|
| `gamma15` | 1.5.0 | yes | `Adjust: toneCurve` (see above) |
| `cc13` | 1.3.0 | yes | `Ccm` |
| `lsc34`, `lsc35` | 3.4.0, 3.5.0 | yes | `Lsc`: the lens position nearest infinity, one set per colour temperature, the greens averaged |
| `demuxblklevel14` (TFE) | 1.4.0 | yes (layout inferred, no public XSD) | `BlackLevel` |
| `refPtV1` (AWB) | 1.0.0 | yes | `Awb: colourGains`: the points on CamX's daylight locus, at the colour temperatures its AWB gives them (below) |
| `Metering` (AEC) | 8.0.1 | the luma target zones only | `Agc: meanTarget`, `histogram` (brightest zone), `meanTargetLux`, `histogramLux` |
| `actuatorDriver` (sensor module) | 1.0.0 | yes (one slot more than the public XSD) | `Af: map`; the macro end's distance is `-macro-dioptres` (default 10, i.e. 10 cm), since the module gives codes, not distances |

### AWB reference points

`refPtV1` holds 15 white points (R/G, B/G of grey as the AWB statistics see
it), indexed by CamX's `StatsIlluminantType`: High, D75, D65, D50, CW,
Fluorescent, TL84, Incandescent, Horizon, Low, then unused. The tuning gives
them no colour temperature; CamX's AWB (`com.qti.stats.awb.so`) does, in its
illuminant and locus segment tables: points 0-1-2-3-5-7-8-9 form the daylight
locus at 12000, 7500, 6500, 5000, 4000, 2850, 2300 and 1500 K, and CW (4100 K)
and TL84 (3800 K) a fluorescent segment off it. `colourGains` has the locus;
the fluorescent points go in a comment.

Using `colourGains` for the colour temperature needs the soft IPA change on
the `pdaf` branch of github.com/W-Floyd/libcamera ("ipa: soft: awb: Take the
colour temperature from the sensor's locus"); upstream's soft IPA ignores it.

## Tests

`go test ./...` uses synthetic files built in the tests; no vendor data is
needed or included.
