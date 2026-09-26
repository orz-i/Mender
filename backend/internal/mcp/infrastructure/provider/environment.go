package provider

import (
	"fmt"
	"sort"
	"strings"
)

var ambientEnvironmentAllowlist = map[string]struct{}{
	"APPDATA":         {},
	"COMSPEC":         {},
	"HOME":            {},
	"LANG":            {},
	"LC_ALL":          {},
	"LC_CTYPE":        {},
	"LOCALAPPDATA":    {},
	"LOGNAME":         {},
	"PATH":            {},
	"SYSTEMROOT":      {},
	"TEMP":            {},
	"TMP":             {},
	"TMPDIR":          {},
	"USER":            {},
	"USERPROFILE":     {},
	"WINDIR":          {},
	"XDG_CACHE_HOME":  {},
	"XDG_CONFIG_HOME": {},
	"XDG_DATA_HOME":   {},
	"XDG_RUNTIME_DIR": {},
}

func commandEnvironment(
	ambient []string,
	explicit map[string]string,
) ([]string, error) {
	values := make(map[string]string)
	names := make(map[string]string)

	for _, item := range ambient {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		normalized := strings.ToUpper(key)
		if _, allowed := ambientEnvironmentAllowlist[normalized]; !allowed {
			continue
		}
		names[normalized] = key
		values[normalized] = value
	}

	for key, value := range explicit {
		if key == "" || strings.ContainsRune(key, '=') || strings.ContainsRune(key, 0) {
			return nil, fmt.Errorf("connect MCP command: invalid environment key")
		}
		normalized := strings.ToUpper(key)
		names[normalized] = key
		values[normalized] = value
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]string, 0, len(keys))
	for _, normalized := range keys {
		result = append(result, names[normalized]+"="+values[normalized])
	}
	return result, nil
}
