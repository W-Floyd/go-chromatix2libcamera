// Command go-chromatix2libcamera inspects Qualcomm CamX Chromatix parameter
// files and converts what libcamera can use into libcamera tuning files.
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/W-Floyd/go-chromatix2libcamera/calibration"
	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/convert"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

const usage = `usage: go-chromatix2libcamera <command> [flags] <file.bin> [args]

Commands:
  modules <file>            list the top-level modules
  symbols [-name RE] <file> list symbols, optionally filtered by name
  hex <file> <id|name>...   dump a symbol's data as 32-bit words, naming
                            any word that matches a symbol id
  decode <file> <id|name>   decode a module with its schema, as JSON
  convert [-lux-idx N] [-drc-gain G] [-module sensormodule.bin
          [-eeprom image] [-macro-dioptres D]] <tuned.bin>
                            write a libcamera software ISP tuning file
  calibration <sensormodule.bin> <eeprom>
                            decode a module EEPROM image (AF, PDAF), as JSON
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "modules":
		err = cmdModules(args)
	case "symbols":
		err = cmdSymbols(args)
	case "hex":
		err = cmdHex(args)
	case "decode":
		err = cmdDecode(args)
	case "convert":
		err = cmdConvert(args)
	case "calibration":
		err = cmdCalibration(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-chromatix2libcamera:", err)
		os.Exit(1)
	}
}

func load(path string) (*chromatix.File, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := chromatix.Parse(buf)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func cmdModules(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("modules takes one file")
	}
	f, err := load(args[0])
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tVERSION\tGROUP\tSIZE")
	for _, m := range f.Modules() {
		fmt.Fprintf(w, "%#x\t%s\t%s\t%#x\t%#x\n", m.ID, m.Name, m.VersionString(), m.Group, m.Size)
	}
	return w.Flush()
}

func cmdSymbols(args []string) error {
	fs := flag.NewFlagSet("symbols", flag.ContinueOnError)
	name := fs.String("name", "", "only symbols whose name matches this regular expression")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("symbols takes one file")
	}
	var re *regexp.Regexp
	if *name != "" {
		var err error
		if re, err = regexp.Compile(*name); err != nil {
			return err
		}
	}
	f, err := load(fs.Arg(0))
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tOFFSET\tSIZE")
	for _, s := range f.Symbols {
		if re != nil && !re.MatchString(s.Name) {
			continue
		}
		fmt.Fprintf(w, "%#x\t%s\t%#x\t%#x\n", s.ID, s.Name, s.Offset, s.Size)
	}
	return w.Flush()
}

func cmdHex(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("hex takes a file and one or more symbol ids or names")
	}
	f, err := load(args[0])
	if err != nil {
		return err
	}
	for _, key := range args[1:] {
		syms, err := lookup(f, key)
		if err != nil {
			return err
		}
		for _, s := range syms {
			dumpWords(f, s)
		}
	}
	return nil
}

func cmdDecode(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("decode takes a file and a module id or name")
	}
	f, err := load(args[0])
	if err != nil {
		return err
	}
	syms, err := lookup(f, args[1])
	if err != nil {
		return err
	}
	var out []any
	for _, s := range syms {
		t, ok := isp.Schema(s)
		if !ok {
			return fmt.Errorf("no schema for %s %s", s.Name, s.VersionString())
		}
		v, err := f.Decode(s, t)
		if err != nil {
			return err
		}
		out = append(out, map[string]any{
			"id": s.ID, "name": s.Name, "version": s.VersionString(),
			"group": s.Group, "value": v,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	return enc.Encode(out)
}

func cmdConvert(args []string) error {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	o := convert.DefaultOptions
	fs.Float64Var(&o.LuxIdx, "lux-idx", o.LuxIdx, "Qualcomm lux index to take the exposure region from (higher is darker)")
	fs.Float64Var(&o.DRCGain, "drc-gain", o.DRCGain, "DRC gain to take the DRC region from (1 is none)")
	module := fs.String("module", "", "sensor module file (com.qti.sensormodule.*.bin), for the lens actuator")
	macro := fs.Float64("macro-dioptres", convert.DefaultMacroDioptres,
		"distance, in dioptres (1/m), the actuator's macro end focuses at, when the calibration gives no distances")
	eeprom := fs.String("eeprom", "", "the module's EEPROM image, for per-unit calibration (needs -module)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("convert takes one tuned.bin file")
	}
	var af *convert.AFMap
	var pdaf *convert.PDAF
	var awbCal *calibration.AWB
	var lscCal *calibration.LSC
	var sources []string
	if *module != "" {
		mf, err := load(*module)
		if err != nil {
			return err
		}
		sources = append(sources, filepath.Base(*module))
		act, err := convert.ActuatorFromFile(mf)
		if err != nil {
			return fmt.Errorf("%s: %w", *module, err)
		}
		var cal *calibration.AF
		if *eeprom != "" {
			c, err := readCalibration(mf, *eeprom)
			if err != nil {
				return err
			}
			sources = append(sources, fmt.Sprintf("%s (%s, module %s)",
				filepath.Base(*eeprom), c.Format, c.ModuleID))
			cal = c.AF
			awbCal, lscCal = c.AWB, c.LSC
			if c.PDAF != nil {
				pdaf = convert.PDAFFromCalibration(c.PDAF)
				if off, ok := convert.PDAFOffsetFromModule(mf); ok {
					pdaf.Offset = &off
				}
			}
		}
		a := convert.BuildAFMap(act, cal, *macro)
		af = &a
	} else if *eeprom != "" {
		return fmt.Errorf("-eeprom needs -module")
	}
	f, err := load(fs.Arg(0))
	if err != nil {
		return err
	}
	ccms, err := convert.CCMs(f, o)
	if err != nil {
		return err
	}
	t := convert.Tuning{
		Source: []string{fmt.Sprintf("%s (lux index %g, DRC gain %g)",
			filepath.Base(fs.Arg(0)), o.LuxIdx, o.DRCGain)},
		CCMs: ccms,
		AF:   af,
		PDAF: pdaf,
	}
	t.Source = append(t.Source, sources...)
	if tc, err := convert.ToneCurve(f, o); err == nil {
		t.ToneCurve = tc
		/*
		 * The luma target goes with the tone curve: the curve is made
		 * for the exposure the target gives, and libcamera's default
		 * target exposes brighter.
		 */
		if zones, err := convert.AECLumaTarget(f); err == nil {
			t.LumaZones = zones
			if h, err := convert.AECHistTarget(f); err == nil {
				t.Hist = h
			} else {
				fmt.Fprintln(os.Stderr, "go-chromatix2libcamera: no histogram luma target:", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "go-chromatix2libcamera: no AEC luma target:", err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "go-chromatix2libcamera: no tone curve:", err)
	}
	if c, err := convert.Chroma(f, o); err == nil {
		t.Chroma = c
	} else {
		fmt.Fprintln(os.Stderr, "go-chromatix2libcamera: no chroma enhancement:", err)
	}
	if locus, other, err := convert.WhitePoints(f); err == nil {
		if awbCal != nil {
			convert.CalibrateWhitePoints(locus, awbCal)
			convert.CalibrateWhitePoints(other, awbCal)
			t.AWBCal = awbCal
		}
		t.WhitePoints, t.OffLocus = locus, other
	} else {
		fmt.Fprintln(os.Stderr, "go-chromatix2libcamera: no AWB reference points:", err)
	}
	if l, err := convert.LensShading(f, o, lscCal); err == nil {
		t.LSC = l
	} else {
		fmt.Fprintln(os.Stderr, "go-chromatix2libcamera: no lens shading:", err)
	}
	if bl, err := convert.BlackLevel(f, o); err == nil {
		t.BlackLevel = bl
		if g, err := convert.BlackLevelByGain(f, o); err == nil {
			t.BlackLevelGain = g
		} else {
			fmt.Fprintln(os.Stderr, "go-chromatix2libcamera: no black level by gain:", err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "go-chromatix2libcamera: no black level:", err)
	}
	return t.WriteYAML(os.Stdout)
}

func cmdCalibration(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("calibration takes a sensor module file and an EEPROM image")
	}
	mf, err := load(args[0])
	if err != nil {
		return err
	}
	c, err := readCalibration(mf, args[1])
	if err != nil {
		return err
	}
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	return e.Encode(c)
}

// readCalibration parses a module EEPROM image, described by the sensor
// module file, with whichever calibration format detects it.
func readCalibration(module *chromatix.File, path string) (*calibration.Calibration, error) {
	m, err := calibration.ModuleFromFile(module)
	if err != nil {
		return nil, fmt.Errorf("sensor module: %w", err)
	}
	image, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := calibration.Parse(m, image)
	if err != nil {
		return nil, fmt.Errorf("%s (EEPROM %s): %w", path, m.EEPROMName, err)
	}
	return c, nil
}

// lookup resolves a symbol id (decimal or 0x-prefixed) or an exact name,
// which may match several symbols.
func lookup(f *chromatix.File, key string) ([]chromatix.Symbol, error) {
	if id, err := strconv.ParseUint(key, 0, 64); err == nil {
		if s, ok := f.Symbol(id); ok {
			return []chromatix.Symbol{s}, nil
		}
		return nil, fmt.Errorf("no symbol with id %s", key)
	}
	var out []chromatix.Symbol
	for _, s := range f.Symbols {
		if s.Name == key {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no symbol named %q", key)
	}
	return out, nil
}

func dumpWords(f *chromatix.File, s chromatix.Symbol) {
	fmt.Printf("== %#x %s (offset %#x, %d bytes)\n", s.ID, s.Name, s.Offset, s.Size)
	d := f.Data(s)
	for i := 0; i+4 <= len(d); i += 4 {
		v := binary.LittleEndian.Uint32(d[i:])
		var note []string
		if ref, ok := f.Symbol(uint64(v)); ok && v != 0 {
			note = append(note, "-> "+ref.Name)
		}
		if fl := float32FromBits(v); v != 0 && isPlausibleFloat(fl) {
			note = append(note, fmt.Sprintf("f32 %g", fl))
		}
		fmt.Printf("  %04x: %08x %11d  %s\n", i, v, int32(v), strings.Join(note, "  "))
	}
	if rem := len(d) % 4; rem != 0 {
		fmt.Printf("  %04x: % x\n", len(d)-rem, d[len(d)-rem:])
	}
}
