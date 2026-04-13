package validate

import "embed"

//go:embed testdata/*
var testdataFS embed.FS

// SampleForExtension returns sample file content for a given file extension,
// or nil if no sample exists for that extension.
func SampleForExtension(ext string) []byte {
	data, err := testdataFS.ReadFile("testdata/sample." + ext)
	if err != nil {
		return nil
	}
	return data
}
