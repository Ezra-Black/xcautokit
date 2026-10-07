package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestScreenshotInlineContentAndPathOnly(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 120, 240))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	result, output, err := screenshotResult(path, "device-A", true)
	if err != nil {
		t.Fatal(err)
	}
	if output["width"] != 120 || output["height"] != 240 || output["path"] != path {
		t.Fatalf("metadata: %v", output)
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected text plus native image, got %v", result.Content)
	}
	img, ok := result.Content[1].(*mcp.ImageContent)
	if !ok || img.MIMEType != "image/png" || !bytes.Equal(img.Data, encoded.Bytes()) {
		t.Fatal("native image lost original PNG bytes")
	}
	wire, err := json.Marshal(img)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Data string `json:"data"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(wire, &raw); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw.Data)
	if err != nil || raw.Type != "image" || !bytes.Equal(decoded, encoded.Bytes()) {
		t.Fatal("MCP image not encoded correctly")
	}
	result, output, err = screenshotResult(path, "device-A", false)
	if err != nil || result != nil || output["width"] != 120 {
		t.Fatalf("path-only result: %v %v %v", result, output, err)
	}
}

func TestScreenshotRejectsInvalidArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(path, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := screenshotResult(path, "A", true); err == nil {
		t.Fatal("invalid image reported as success")
	}
}

func TestCapturePathCreatesParentAndUsesUniqueNames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	a, err := capturePath("", dir, "shot", "png")
	if err != nil {
		t.Fatal(err)
	}
	b, err := capturePath("", dir, "shot", "png")
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !filepath.IsAbs(a) {
		t.Fatalf("capture paths not unique/absolute: %s %s", a, b)
	}
}
