package transport

import (
	"runtime/debug"
	"strings"
)

const (
	appName              = "images-go"
	modulePath           = "github.com/selectel/" + appName
	unknownModuleVersion = "v0.0.0"
)

var moduleVersion = readModuleVersion()

func buildUserAgent(prefix string) string {
	userAgent := appName + "/" + moduleVersion

	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return userAgent
	}

	return prefix + " " + userAgent
}

func readModuleVersion() string {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownModuleVersion
	}

	for _, dependency := range buildInfo.Deps {
		if dependency.Path == modulePath {
			return dependency.Version
		}
	}

	return unknownModuleVersion
}
