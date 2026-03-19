package main

import "testing"

func TestGeneratePluginOutputDir(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, "."},
		{"directory arg", []string{"/output"}, "/output"},
		{"stdout mode", []string{"-"}, ""},
		{"skills-dir space separated", []string{"--skills-dir", "/skills", "/output"}, "/output"},
		{"skills-dir equals separated", []string{"--skills-dir=/skills", "/output"}, "/output"},
		{"skills-dir single dash", []string{"-skills-dir", "/skills", "/output"}, "/output"},
		{"skills-dir only", []string{"--skills-dir", "/skills"}, "."},
		{"skills-dir with stdout", []string{"--skills-dir", "/skills", "-"}, ""},
		{"skills-dir equals with stdout", []string{"--skills-dir=/skills", "-"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generatePluginOutputDir(tt.args)
			if got != tt.want {
				t.Errorf("generatePluginOutputDir(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
