package cmd

import (
	"fmt"
	"runtime/debug"
	"strings"
)

const releaseURL = "https://github.com/grievouz/discoctl/releases/tag/v"

var (
	version   = "0.0.0-dev"
	buildDate = "unknown"
)

func buildVersion() string {
	resolved := strings.TrimPrefix(version, "v")
	if resolved == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			resolved = strings.TrimPrefix(info.Main.Version, "v")
		}
	}
	if resolved == "" {
		resolved = "0.0.0-dev"
	}

	result := fmt.Sprintf("discoctl version %s", resolved)
	if buildDate != "" && buildDate != "unknown" {
		result += " (" + buildDate + ")"
	}
	if !strings.Contains(resolved, "dev") {
		result += "\n" + releaseURL + resolved
	}
	return result
}
