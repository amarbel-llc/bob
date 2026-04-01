package tools

import (
	"encoding/json"
	"testing"
)

func TestDecodeBase64Content_SingleObject(t *testing.T) {
	input := `{"name":"README.md","content":"SGVsbG8gV29ybGQ=\n","encoding":"base64","size":11}`
	result := decodeBase64Content(input)

	var obj map[string]any
	if err := json.Unmarshal([]byte(result), &obj); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if obj["content"] != "Hello World" {
		t.Errorf("expected decoded content 'Hello World', got %q", obj["content"])
	}
	if obj["encoding"] != "utf-8" {
		t.Errorf("expected encoding 'utf-8', got %q", obj["encoding"])
	}
}

func TestDecodeBase64Content_Array(t *testing.T) {
	input := `[{"name":"a.txt","content":"YWFh","encoding":"base64"},{"name":"b.txt","content":"YmJi","encoding":"base64"}]`
	result := decodeBase64Content(input)

	var arr []map[string]any
	if err := json.Unmarshal([]byte(result), &arr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if arr[0]["content"] != "aaa" {
		t.Errorf("expected 'aaa', got %q", arr[0]["content"])
	}
	if arr[1]["content"] != "bbb" {
		t.Errorf("expected 'bbb', got %q", arr[1]["content"])
	}
}

func TestDecodeBase64Content_NoEncoding(t *testing.T) {
	input := `{"name":"file.txt","content":"plain text"}`
	result := decodeBase64Content(input)

	if result != input {
		t.Errorf("expected no change for non-base64 content")
	}
}

func TestDecodeBase64Content_InvalidJSON(t *testing.T) {
	input := "not json at all"
	result := decodeBase64Content(input)

	if result != input {
		t.Errorf("expected passthrough for invalid JSON")
	}
}
