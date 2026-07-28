// Copyright (C) 2026 DaemonHound Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package output provides terminal-aware coloured output helpers.
// Colours are suppressed when stdout is not a TTY or when NO_COLOR is set.
package output

import (
	"fmt"
	"os"
)

// isTTY is true when stdout is connected to a real terminal.
var isTTY = func() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}()

func colorsEnabled() bool {
	return isTTY && os.Getenv("NO_COLOR") == ""
}

func colorize(code, s string) string {
	if !colorsEnabled() {
		return s
	}
	return code + s + "\033[0m"
}

func Green(s string) string  { return colorize("\033[32m", s) }
func Yellow(s string) string { return colorize("\033[33m", s) }
func Red(s string) string    { return colorize("\033[31m", s) }
func Cyan(s string) string   { return colorize("\033[36m", s) }
func Bold(s string) string   { return colorize("\033[1m", s) }
func Dim(s string) string    { return colorize("\033[2m", s) }

// Status returns a colourized file status string.
func Status(s string) string {
	switch s {
	case "clean":
		return Green(s)
	case "dirty", "new":
		return Yellow(s)
	case "missing", "error":
		return Red(s)
	default:
		return s
	}
}

// Action returns a colourized sync action string.
func Action(s string) string {
	switch s {
	case "pushed", "would push":
		return Cyan(s)
	case "pulled", "would pull":
		return Green(s)
	case "up to date":
		return Dim(s)
	default:
		return s
	}
}

// OK, Warn, Fail are for doctor-style check output.
func OK(msg string) string   { return fmt.Sprintf("[%s]   %s", Green("OK"), msg) }
func Warn(msg string) string { return fmt.Sprintf("[%s] %s", Yellow("WARN"), msg) }
func Fail(msg string) string { return fmt.Sprintf("[%s] %s", Red("FAIL"), msg) }
