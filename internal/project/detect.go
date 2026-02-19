package project

// Detection holds the results of project auto-detection.
type Detection struct {
	Language       string // rust, go, typescript, python, unknown
	Runner         string // cargo-nextest, go-test, vitest, pytest, unknown
	DefaultPackage string // extracted from manifest
	BuildCommand   string // language-specific default
	TestCommand    string // language-specific default
}

// Detect inspects the project root for language indicators.
func Detect(projectRoot string) *Detection {
	panic("not implemented")
}
