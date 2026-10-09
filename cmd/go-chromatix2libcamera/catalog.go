package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/W-Floyd/go-chromatix2libcamera/chromatix"
	"github.com/W-Floyd/go-chromatix2libcamera/isp"
)

// cmdCatalog summarises what each kind of module in a file holds, to see what
// a module without a schema is made of before decoding it: how many instances
// it has, how big they are, which elements they refer to (and how many of
// them), and which of the leading slots are the same in every instance (what
// the vendor left alone) and which differ (what was tuned).
func cmdCatalog(args []string) error {
	fs := flag.NewFlagSet("catalog", flag.ContinueOnError)
	name := fs.String("name", "", "only modules whose name matches this regular expression")
	slots := fs.Int("slots", 24, "how many leading slots to summarise")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("catalog takes one file")
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

	type kind struct {
		name, version string
		instances     []chromatix.Symbol
	}
	var order []string
	kinds := map[string]*kind{}
	for _, s := range f.Modules() {
		if re != nil && !re.MatchString(s.Name) {
			continue
		}
		key := s.Name + " " + s.VersionString()
		k := kinds[key]
		if k == nil {
			k = &kind{name: s.Name, version: s.VersionString()}
			kinds[key] = k
			order = append(order, key)
		}
		k.instances = append(k.instances, s)
	}
	sort.Strings(order)

	for _, key := range order {
		k := kinds[key]
		_, schema := isp.Schema(k.instances[0])
		sizes := map[uint64]int{}
		for _, s := range k.instances {
			sizes[s.Size]++
		}
		fmt.Printf("%s  v%s  %d instance(s)", k.name, k.version, len(k.instances))
		if schema {
			fmt.Print("  [has a schema]")
		}
		fmt.Println()
		var szs []string
		for sz, n := range sizes {
			szs = append(szs, fmt.Sprintf("%#x bytes (%d slots) x%d", sz, sz/8, n))
		}
		sort.Strings(szs)
		fmt.Println("  size:", strings.Join(szs, "; "))

		// The elements the instances refer to, and the values of each of the
		// leading slots across the instances.
		refs := map[string]string{}
		values := make([]map[string]int, *slots)
		for i := range values {
			values[i] = map[string]int{}
		}
		for _, s := range k.instances {
			o, ok := f.Generic(s).(*chromatix.Object)
			if !ok {
				continue
			}
			for i, n := range o.Names {
				v := o.Values[i]
				if ref, ok := v.(*chromatix.Object); ok {
					rn, _ := ref.Get("ref")
					cn, _ := ref.Get("count")
					refs[fmt.Sprint(rn)] = fmt.Sprint(cn)
					continue
				}
				var idx int
				fmt.Sscanf(n, "s%d", &idx)
				if idx < *slots {
					b, _ := json.Marshal(v)
					values[idx][string(b)]++
				}
			}
		}
		var rn []string
		for r, c := range refs {
			rn = append(rn, fmt.Sprintf("%s x%s", r, c))
		}
		sort.Strings(rn)
		if len(rn) > 0 {
			fmt.Println("  refers to:", strings.Join(rn, ", "))
		}
		for i, m := range values {
			if len(m) == 0 {
				continue
			}
			var vs []string
			for v := range m {
				vs = append(vs, v)
			}
			sort.Strings(vs)
			if len(vs) > 4 {
				vs = append(vs[:4], fmt.Sprintf("... (%d distinct)", len(m)))
			}
			tag := "tuned"
			if len(m) == 1 {
				tag = "same"
			}
			fmt.Printf("    s%-3d %-5s %s\n", i, tag, strings.Join(vs, " "))
		}
		fmt.Println()
	}
	return nil
}
