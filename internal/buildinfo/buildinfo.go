package buildinfo

import "runtime"

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func Platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
