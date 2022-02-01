package utils

import "fmt"

// ShellColor returns a coloring functions for strings
type ShellColor func(...interface{}) string

var (
	// Black color
	Black = Color("\033[1;30m%s\033[0m")
	// Red color
	Red = Color("\033[1;31m%s\033[0m")
	// Green color
	Green = Color("\033[1;32m%s\033[0m")
	// Brown color
	Brown = Color("\033[1;33m%s\033[0m")
	// Blue color
	Blue = Color("\033[1;34m%s\033[0m")
	// Purple color
	Purple = Color("\033[1;35m%s\033[0m")
	// Cyan color
	Cyan = Color("\033[1;36m%s\033[0m")
	// LightGray color
	LightGray = Color("\033[1;37m%s\033[0m")

	// TODO: alternate colors depending on specialresource
	// color = []ShellColor{Green, Blue, Brown, Purple, Cyan}
)

// Color colours the string according to provide ShellColor
func Color(colorString string) ShellColor {
	sprint := func(args ...interface{}) string {
		return fmt.Sprintf(colorString,
			fmt.Sprint(args...))
	}
	return sprint
}

// Print prints colored strings to the console
func Print(msg string, color ShellColor) string {
	return color(fmt.Sprintf("%s  ", msg))
}
