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
go-chromatix2libcamera convert  com.qti.tuned.<sensor>.bin > <sensor>.yaml
```

`convert` currently writes the front-end black level (`demuxblklevel14`) as
libcamera's `BlackLevel` and the colour correction matrices (`cc13`) as its
`Ccm`. Chromatix selects both by exposure (and DRC gain, flash, ...) as well as
colour temperature, while libcamera's software ISP indexes by colour
temperature only, so one exposure region is chosen: `-lux-idx` (Qualcomm lux index, higher
is darker; default 250, ordinary indoor light) and `-drc-gain` (default 1).

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
| `gamma15` | 1.5.0 | yes | (needs a curve in libcamera's software ISP) |
| `cc13` | 1.3.0 | yes | `Ccm` |
| `demuxblklevel14` (TFE) | 1.4.0 | yes (layout inferred, no public XSD) | `BlackLevel` |

## Tests

`go test ./...` uses synthetic files built in the tests; no vendor data is
needed or included.
