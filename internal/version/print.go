package version

import (
	"encoding/json"
	"fmt"
	"io"
)

// Info returns the build version fields as a map suitable for JSON output.
func Info() map[string]string {
	return map[string]string{
		"version":    Version,
		"git_commit": GitCommit,
		"build_time": BuildTime,
	}
}

// Fprint writes version information for the named binary to w. When asJSON is
// true it writes the indented JSON form of Info; otherwise it writes a single
// human-readable line.
func Fprint(w io.Writer, name string, asJSON bool) error {
	if asJSON {
		data, err := json.MarshalIndent(Info(), "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling version info: %w", err)
		}

		_, err = fmt.Fprintln(w, string(data))

		return err
	}

	_, err := fmt.Fprintf(w, "%s version %s (commit: %s, built: %s)\n",
		name, Version, GitCommit, BuildTime)

	return err
}
