package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/W-Floyd/go-chromatix2libcamera/convert"
)

// defaultConfig is where a device's packaging installs its config.
const defaultConfig = "/etc/go-chromatix2libcamera.json"

// defaultOutput is libcamera's directory for the software ISP's tuning
// files outside the package (libcamera looks there before
// /usr/share/libcamera/ipa/soft).
const defaultOutput = "/etc/libcamera/ipa/soft"

// Config describes a device's cameras for generate.
//
//	{
//	  "vendor": "/run/msm-firmware-loader/mnt/vendor*/lib64/camera",
//	  "cameras": [
//	    { "sensor": "s5kjn1",
//	      "tuned": "com.qti.tuned.mot_fogona_s5kjns.bin",
//	      "module": "com.qti.sensormodule.mot_fogona_s5kjns_qtech.bin",
//	      "eeprom": "i2c-bus@0/eeprom@50" },
//	    { "sensor": "ov08f10", "tuned": "com.qti.tuned.mot_ov08f.bin" }
//	  ]
//	}
type Config struct {
	// Vendor is a glob of the directories holding the Chromatix files; the
	// first that holds a camera's tuned file is used.
	Vendor string `json:"vendor"`
	// Output is where the tuning files go (default defaultOutput).
	Output  string         `json:"output"`
	Cameras []CameraConfig `json:"cameras"`
}

// CameraConfig is one camera: the libcamera sensor name its tuning file is
// named after, its Chromatix files, and the device tree node of its module
// EEPROM, matched as the end of the nvmem device's of_node path.
type CameraConfig struct {
	Sensor string `json:"sensor"`
	Tuned  string `json:"tuned"`
	Module string `json:"module"`
	EEPROM string `json:"eeprom"`
}

func cmdGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	config := fs.String("config", defaultConfig, "the device's camera config")
	output := fs.String("output", "", "directory to write the tuning files to (default: the config's, or "+defaultOutput+")")
	dryRun := fs.Bool("n", false, "report what would change, write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("generate takes no arguments")
	}

	buf, err := os.ReadFile(*config)
	if err != nil {
		return err
	}
	var c Config
	if err := json.Unmarshal(buf, &c); err != nil {
		return fmt.Errorf("%s: %w", *config, err)
	}
	out := *output
	if out == "" {
		out = c.Output
	}
	if out == "" {
		out = defaultOutput
	}

	var failed []string
	for _, cam := range c.Cameras {
		if err := generateCamera(c.Vendor, cam, out, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "go-chromatix2libcamera: %s: %v\n", cam.Sensor, err)
			failed = append(failed, cam.Sensor)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("no tuning for %s", strings.Join(failed, ", "))
	}
	return nil
}

func generateCamera(vendor string, cam CameraConfig, out string, dryRun bool) error {
	if cam.Sensor == "" || cam.Tuned == "" {
		return fmt.Errorf("a camera needs a sensor and a tuned file")
	}
	dir, err := findVendorDir(vendor, cam.Tuned)
	if err != nil {
		return err
	}
	module := ""
	if cam.Module != "" {
		module = filepath.Join(dir, cam.Module)
	}

	/*
	 * Without the EEPROM the tuning is still the vendor's, for the golden
	 * module rather than this unit: better than none.
	 */
	eeprom := ""
	if cam.EEPROM != "" && module != "" {
		if eeprom, err = findNVMEM(cam.EEPROM); err != nil {
			fmt.Fprintf(os.Stderr, "go-chromatix2libcamera: %s: %v, generating without per-unit calibration\n",
				cam.Sensor, err)
		}
	}

	t, err := buildTuning(filepath.Join(dir, cam.Tuned), module, eeprom,
		convert.DefaultOptions, convert.DefaultMacroDioptres)
	if err != nil && eeprom != "" {
		fmt.Fprintf(os.Stderr, "go-chromatix2libcamera: %s: %v, generating without per-unit calibration\n",
			cam.Sensor, err)
		t, err = buildTuning(filepath.Join(dir, cam.Tuned), module, "",
			convert.DefaultOptions, convert.DefaultMacroDioptres)
	}
	if err != nil {
		return err
	}

	var b bytes.Buffer
	if err := t.WriteYAML(&b); err != nil {
		return err
	}
	path := filepath.Join(out, cam.Sensor+".yaml")
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b.Bytes()) {
		fmt.Printf("%s: unchanged\n", path)
		return nil
	}
	if dryRun {
		fmt.Printf("%s: would be written\n", path)
		return nil
	}
	if err := writeAtomic(path, b.Bytes()); err != nil {
		return err
	}
	fmt.Printf("%s: written\n", path)
	return nil
}

// findVendorDir returns the first directory matching the glob that holds
// the file.
func findVendorDir(glob, file string) (string, error) {
	dirs, err := filepath.Glob(glob)
	if err != nil {
		return "", err
	}
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, file)); err == nil {
			return d, nil
		}
	}
	return "", fmt.Errorf("no %s under %s", file, glob)
}

// findNVMEM returns the nvmem device whose device tree node's path ends in
// node: I2C bus numbers, and so the devices' names, change between boots.
func findNVMEM(node string) (string, error) {
	devs, err := filepath.Glob("/sys/bus/nvmem/devices/*")
	if err != nil {
		return "", err
	}
	for _, d := range devs {
		of, err := filepath.EvalSymlinks(filepath.Join(d, "of_node"))
		if err != nil {
			continue
		}
		if strings.HasSuffix(of, "/"+strings.TrimPrefix(node, "/")) {
			return filepath.Join(d, "nvmem"), nil
		}
	}
	return "", fmt.Errorf("no nvmem device for %s", node)
}

// writeAtomic replaces path with data, never leaving it half written.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(tmp.Name(), 0o644)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
