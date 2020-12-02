package controllers

import "fmt"

type ShellColor func(...interface{}) string

var (
	Black     = Color("\033[1;30m%s\033[0m")
	Red       = Color("\033[1;31m%s\033[0m")
	Green     = Color("\033[1;32m%s\033[0m")
	Brown     = Color("\033[1;33m%s\033[0m")
	Blue      = Color("\033[1;34m%s\033[0m")
	Purple    = Color("\033[1;35m%s\033[0m")
	Cyan      = Color("\033[1;36m%s\033[0m")
	LightGray = Color("\033[1;37m%s\033[0m")

	color = []ShellColor{Green, Blue, Brown, Purple, Cyan}
)

func Color(colorString string) ShellColor {
	sprint := func(args ...interface{}) string {
		return fmt.Sprintf(colorString,
			fmt.Sprint(args...))
	}
	return sprint
}

func prettyPrint(msg string, color ShellColor) string {
	return color(fmt.Sprintf("%s  ", msg))
}
