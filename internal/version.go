package internal

import (
	"fmt"
	"runtime/debug"
	"strings"
)

var (
	ModulePath    string
	ModuleVersion string
)

func init() {
	bi, _ := debug.ReadBuildInfo()
	ModulePath = bi.Main.Path
	if ModuleVersion == "" {
		ModuleVersion = bi.Main.Version
	}
	if !strings.Contains(ModuleVersion, "-") {
		var vcsRev, vcsTime string
		for _, i := range bi.Settings {
			switch i.Key {
			case "vcs.revision":
				vcsRev = i.Value
			case "vcs.time":
				vcsTime = i.Value
			}
		}
		ModuleVersion += fmt.Sprintf(" %s %s", vcsRev, vcsTime)
	}
}
