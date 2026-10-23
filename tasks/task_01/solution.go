package main
import "strings" 

func greet(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Hello, World!"
	}

	return "Hello, " + name + "!"
}