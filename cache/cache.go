package main

import "fmt"

type Cache struct {
	// Source files
	GoFiles           []*File  // .go source files (excluding CgoFiles, TestGoFiles, XTestGoFiles)
	IgnoredGoFiles    []string // .go source files ignored for this build (including ignored _test.go files)
	InvalidGoFiles    []string // .go source files with detected problems (parse error, wrong package name, and so on)
	IgnoredOtherFiles []string // non-.go source files ignored for this build
	CFiles            []string // .c source files
	CXXFiles          []string // .cc, .cpp and .cxx source files
	MFiles            []string // .m (Objective-C) source files
	HFiles            []string // .h, .hh, .hpp and .hxx source files
	FFiles            []string // .f, .F, .for and .f90 Fortran source files
	SFiles            []string // .s source files
	SwigFiles         []string // .swig files
	SwigCXXFiles      []string // .swigcxx files
	SysoFiles         []string // .syso system object files to add to archive
}

type File struct {
	Name      string                      // package name
	BuildTags []string                    // tags that can influence file selection in this directory
	Imports   []string                    // import paths from GoFiles, CgoFiles
	ImportPos map[string][]token.Position // line information for Imports

	// //go:embed patterns found in Go source files
	// For example, if a source file says
	//	//go:embed a* b.c
	// then the list will contain those two strings as separate entries.
	// (See package embed for more details about //go:embed.)
	EmbedPatterns   []string                    // patterns from GoFiles, CgoFiles
	EmbedPatternPos map[string][]token.Position // line information for EmbedPatterns

	Exports []string
}
