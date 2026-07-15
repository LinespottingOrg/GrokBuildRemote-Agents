package main

import "github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/inject"

// newInjector returns the build-tagged platform injector from package inject.
func newInjector() inject.Injector {
	return inject.Default()
}
