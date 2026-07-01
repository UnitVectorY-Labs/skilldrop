package main

import (
	"runtime/debug"
)

var Version = "dev" // This will be set by the build systems to the release version

func main() {
	if Version == "dev" || Version == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				Version = bi.Main.Version
			}
		}
	}
	// TODO: Implement everything
	// if err := skilldrop.Run(os.Args[1:], os.Stdout, os.Stderr, Version); err != nil {
	// 	fmt.Fprintln(os.Stderr, "error:", err)
	// 	os.Exit(1)
	// }
}
