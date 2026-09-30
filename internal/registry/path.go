package registry

import (
	"fmt"
	"strings"
)

// Hive names as they appear in JSON config. The Windows build maps these onto
// the corresponding registry.Key constants; keeping the parsing here means the
// hive-selection rules can be tested on any platform, since a path naming an
// unsupported hive is a silent no-op on the target machine if it slips through.
const (
	HiveLocalMachine = "HKLM"
	HiveCurrentUser  = "HKCU"
)

// hiveAliases maps every accepted spelling of a hive to its canonical short
// form. Config is hand-edited, so both the short and long names are allowed and
// case is not significant.
var hiveAliases = map[string]string{
	"hklm":               HiveLocalMachine,
	"hkey_local_machine": HiveLocalMachine,
	"hkcu":               HiveCurrentUser,
	"hkey_current_user":  HiveCurrentUser,
}

// splitRegistryPath separates a configured "HKLM\SUB\KEY" path into its hive
// and its subkey. It is pure: it does not touch the registry, so an
// unrecognised or malformed path is reported here rather than being written
// somewhere unintended.
func splitRegistryPath(path string) (hive, subkey string, err error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "/", `\`)

	parts := strings.SplitN(normalized, `\`, 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid registry path: %s", path)
	}

	canonical, ok := hiveAliases[strings.ToLower(strings.TrimSpace(parts[0]))]
	if !ok {
		return "", "", fmt.Errorf("unsupported registry hive: %s", parts[0])
	}
	return canonical, parts[1], nil
}
