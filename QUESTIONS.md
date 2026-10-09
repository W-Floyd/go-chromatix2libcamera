# Questions Chromatix access would answer

What we have of Qualcomm's documents is the SM6225 and PM6225 data sheets,
which give the camera subsystem's capabilities (three CSI interfaces, ISP
throughput) and say nothing of the image-quality modules. The tuning files
themselves (`com.qti.tuned.*.bin`, `com.qti.sensormodule.*.bin`) name every
element, and `dump` and `catalog` decode all of them, but not what a field
means: that is in the XSDs of Qualcomm's chi-cdk (Chromatix) and in the
tuning tool. Where it is guessed, it is here, with what an answer would let
us do. Nothing here needs an answer to keep working: the converter writes what
it can verify, and the rest waits.

Evidence is from the Moto G Play 2024 (fogona): rear `s5kjns`, front `ov08f`.

## The format

1. **Field names, types and units of every module.** `Generic` names a field
   by its slot and guesses float or integer from the bits, which is wrong for
   the odd integer that looks like a float. The XSDs give both, and the
   units (Q formats, percentages against fractions, 8, 10 or 12-bit domains).
   Of about 100 module types per file we have a schema for about a dozen.
2. **The slots at the start of every module** (`s0` is the same small number,
   8 for the AEC modules, 1 for others, with `revision` elements beside it):
   a module ID? an enable? Are they the same in every module?
3. **How the runtime picks an instance.** A file holds many instances of one
   module (`asf32` x40, `bpcabf41` x38, `ltm15` x4), each in a symbol of its
   own, and section 2 (an index of the modules we do not decode) says which
   applies when. What do the instance's group and the index say: scenario
   (preview, video, snapshot), sensor mode, flash, hardware block? Today
   `convert` takes the first or the one that matches a lux index and DRC
   gain, which is a guess for anything but the lux and DRC selection we
   verified against the tuning's own `expKneeEntries`.
4. **Interpolation between regions.** The trigger tables (`mod_*_aec_data`:
   lux index, DRC gain, gain, CCT, ...) hold regions with a start and an end.
   Does CamX interpolate linearly between neighbours, in what variable (lux
   index, gain, or log of them), is there hysteresis, and what happens at the
   ends? libcamera's soft ISP blends by lux for `Ccm` and `Lsc` only, from
   our reading.
5. **Trigger variables.** Which ones are there and in which units: lux index,
   exposure index, DRC gain, ADRC, gain, ISO, CCT, flash, face, zoom?

## Exposure and the 3A loops (relevant to our soft ISP's loops)

6. **`Convergence`** (3 instances; slots 8, 10, 13, 14, 15 and 29 are tuned,
   e.g. 0.4/0.8, 1/1.5, 30/50, 0.1/0.15): the speed, tolerance and damping
   of the exposure loop, and the tables `brightAdjustment`,
   `darkAdjustment` and `predictiveTrigger`. The soft ISP's loop was tuned by
   hand and pulsed when it ran more often than it was built to (see the
   libcamera notes): the vendor's numbers are the design it should follow.
7. **`StartUp`** (34 slots, a first exposure and AWB?) and **`aecSkipCtrl`**
   (`DFSSkipCountZone`, `statsParsingTrigger`): how many frames to skip after
   a change before the statistics are trusted, which is what we found the
   hard way (exposure changes take two to three frames to apply on this
   sensor).
8. **`Metering`** (decoded for its luma target and histogram, the rest not):
   the metering modes and their weights, the exposure table's other columns,
   how `histogram` and `luma` targets combine.
9. **The lux index definition.** We use the exposure index of the exposure
   table, 23.45 steps per stop of gain times time (3% of sensitivity a step),
   from the tuning's `expKneeEntries`. Is that CamX's own definition, with its
   reference at the luma target, and what is a lux index of 0 or 999?
10. **ADRC / DRC.** How the DRC gain is computed (`ltm15`'s `drc_gain_data`,
    `hdr_aec_data`, `aec_data`, `gtm12`, `tmc11`/`tmc12`): from which
    histogram quantiles, with which limits, and how it splits between the
    global and the local tone mapping (`percentageOfGTM` appears in the HAL's
    own log lines).
11. **HDR.** `hdrvideo10`, `SensorHDRDefaultConfig`, `aecMultiCamSync`,
    `mf10`/`mf11`, `swmctf10`, `swmfnr20`: does this sensor do staggered,
    DOL or multi-frame HDR here, at what ratios, and how are the frames
    merged and the highlights weighted? The libcamera HDR work (alternating
    exposures, a merge and a tone map) was done from the data and the
    literature alone, and settled on its own numbers (a ratio of 8 at the
    most, the sensor linear to about 850 counts).

## White balance

12. **The `SF*` family** (`SFIlluWVV1` decoded for the illuminant weights;
    `SFDistWVV1`, `SFGeneralWVV1`, `SFMLCZV1/2/3`, `SFStatDistV1`,
    `SFStatScrV1`, `SFSubSmpV1`, `SFSpectralSensorAssistV1`, `SFDDV1`):
    the algorithm's terms and how they combine. `ctPrior` is the
    illuminant-weight vector by lux index, which libcamera does not read
    yet; the rest would tell what a full statistical AWB does with it.
13. **`AWBExtensionParam`, `awbSceneChangeConvergeV1`, `MWBGainV1`,
    `custRefPtV1`**: convergence and scene-change speeds (our white balance
    was also unsmoothed until it was found to pulse), and manual gains.

## Focus

14. **`AFAlgoTuning`, `ContrastAF`, `PDAF`, `PDLib`, `HAF`, `DepthAF`,
    `AFMONITOR`, `AFDTuningData`, `AFTuningSoftwareStatistics`, `TOF`,
    `DCIAF`**: search steps and speeds, confidence thresholds, the phase to
    defocus conversion (we read `PDAF`'s DCC map and the phase offset
    through the sensor module; what else is there), refocus triggers (the
    soft ISP's own are guessed: sharpness and scene-change thresholds).

## Image quality modules

15. **`asf32`** (x40) and **`bpcabf41`** (x38), already used for `Sharpen`
    and `Denoise`: only their layouts are inferred (no public XSD), and they
    "account for" the data. Are the strengths what we read them to be, and
    how do the two modules interact (the ABF is also a denoiser, and the
    ASF has an edge-and-noise split)?
16. **`ltm15`**: a block of weights (0.125, 0.25, 0.125, 0.5), a table of
    paired values from (255, 89) down to (1, 1), and rising breakpoints
    (0, 105, 202, 289, ...): the local tone mapping's curves, masks and
    strengths, which the HAL logs as `ltm_scale[2][64]`, `LTMIndexCurve`,
    `LTMIndexMask`, `LTMIndexWeight`, `ltm_new_dark_boost`,
    `ltm_new_bright_suppress`, `ltm_new_lce_strength`, and a
    `ltmDynamicContrastStrengthRange`. With the definitions the soft ISP's
    local tone mapping (a blurred base, compressed, with the detail kept)
    could use the vendor's own curves rather than invented ones.
17. **`demosaic36`, `cst12`, `csmce11`, `sce11`, `bds10`, `ldc10`, `bls12`,
    `cv12`** (the last decoded): the demosaic's tuning (edge and noise
    thresholds), the colour space conversion and the chroma enhancement
    (`csmce`, `sce`: skin tone enhancement?), the downscaler, lens
    distortion correction, and the black level adjustments.
18. **Noise modelling.** Our noise model is fitted from the ABF's tables
    (`std^2 = k * bin`, `k / gain` about 11 in 12-bit units, which matched
    10-bit measurements of the sensor to a read-noise intercept). Is that
    the table's meaning, and are there per-channel or per-lux tables that we
    do not read?
19. **Multi-frame and AI**: `mf10`, `mf11`, `swmctf10`, `swmfnr20`,
    `aidenoiser10`, `tintless20`, `eis10/11/12`, `RERTuningData`: what the
    software multi-frame noise reduction, the AI denoiser and the
    tintless (lens shading) correction need as inputs and tune. These are
    the candidates for a stills path built on libcamera's multi-frame work.

## Sensor module and calibration

20. **The module files** (`com.qti.sensormodule.*.bin`): only the actuator
    (`actuatorDriver`), the AWB reference points (`refPtV1`) and the
    OTP/EEPROM layouts of the two sensors are decoded. Their register tables
    (soft reset, mode settings, streaming, group hold) are listed by
    `camx-sensormodule-regs.py` in the main repo; are there the sensor's HDR
    (Smart-ISO, dual conversion gain) mode settings, whether the S5KJN1 has
    such a mode, and which register is its group hold?
21. **Which of the calibrations in the EEPROM/OTP the vendor applies** to
    which tuning: lens shading, AWB golden reference, PDAF gain maps and DCC,
    AF macro and infinity codes. We read all of them that we could decode; the
    order they are applied in, and whether a table is replaced or scaled,
    is from the tuning.

## What we would do with the answers, in order

1. A proper `Convergence`/`StartUp`/skip count for the soft ISP's exposure
   loop (questions 6, 7).
2. `ltm15`/`gtm12` for a tone mapping that follows the vendor's, and the
   DRC (10, 16).
3. The AWB statistical model's terms, to replace the learned model's stand-in
   (12, 13).
4. The AF tuning (14).
5. HDR, if the sensor's own modes exist (11, 20).
