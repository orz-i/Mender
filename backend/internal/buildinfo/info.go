// Package buildinfo exposes immutable build metadata injected by release builds.
package buildinfo

type Info struct {
	Version   string
	Commit    string
	BuildDate string
}

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}
