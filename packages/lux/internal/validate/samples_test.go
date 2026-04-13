package validate

import "testing"

func TestSampleForExtensionKnown(t *testing.T) {
	known := []string{"go", "sh", "json", "toml", "nix", "py", "rs", "lua", "zig", "css", "html", "c", "java", "swift", "js", "ts", "yaml"}
	for _, ext := range known {
		content := SampleForExtension(ext)
		if content == nil {
			t.Errorf("SampleForExtension(%q) returned nil, want content", ext)
		}
		if len(content) == 0 {
			t.Errorf("SampleForExtension(%q) returned empty content", ext)
		}
	}
}

func TestSampleForExtensionUnknown(t *testing.T) {
	unknown := []string{"xyz", "foobar", "docx", ""}
	for _, ext := range unknown {
		content := SampleForExtension(ext)
		if content != nil {
			t.Errorf("SampleForExtension(%q) = %q, want nil", ext, content)
		}
	}
}
