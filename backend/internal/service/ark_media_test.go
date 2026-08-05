//go:build unit

package service

import (
	"encoding/json"
	"testing"
)

func TestBuildArkVideoTaskBodyFlatRequest(t *testing.T) {
	in := []byte(`{"model":"doubao-seedance-2.0-fast","prompt":"一只猫在草地上奔跑，阳光明媚，慢动作","ratio":"16:9","duration":5}`)
	out, err := buildArkVideoTaskBody(in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["model"] != "doubao-seedance-2.0-fast" {
		t.Fatalf("model = %v", payload["model"])
	}
	if _, ok := payload["prompt"]; ok {
		t.Fatal("prompt should be converted into content, not kept top-level")
	}
	content, ok := payload["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content = %+v", payload["content"])
	}
	item := content[0].(map[string]any)
	if item["type"] != "text" || item["text"] != "一只猫在草地上奔跑，阳光明媚，慢动作" {
		t.Fatalf("content item = %+v", item)
	}
	// Ark native parameters stay top-level untouched.
	if payload["ratio"] != "16:9" || payload["duration"].(float64) != 5 {
		t.Fatalf("ratio/duration = %v / %v", payload["ratio"], payload["duration"])
	}
}

func TestBuildArkVideoTaskBodyKeepsExtraTopLevelParams(t *testing.T) {
	in := []byte(`{"model":"m","prompt":"p","resolution":"720p","seed":11,"watermark":true,"generate_audio":true,"camera_fixed":false}`)
	out, err := buildArkVideoTaskBody(in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["resolution"] != "720p" || payload["seed"].(float64) != 11 {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["watermark"] != true || payload["generate_audio"] != true || payload["camera_fixed"] != false {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestBuildArkVideoTaskBodyNativePassthrough(t *testing.T) {
	in := []byte(`{"model":"m","content":[{"type":"text","text":"原生格式 --ratio 16:9"}],"ratio":"16:9"}`)
	out, err := buildArkVideoTaskBody(in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if string(out) != string(in) {
		t.Fatalf("native content body should pass through untouched, got %s", out)
	}
}

func TestBuildArkVideoTaskBodyImageToVideoString(t *testing.T) {
	in := []byte(`{"model":"m","prompt":"让画面动起来","img_url":"https://example.com/a.png"}`)
	out, err := buildArkVideoTaskBody(in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := payload["img_url"]; ok {
		t.Fatal("img_url should be moved into content")
	}
	content := payload["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content = %+v", content)
	}
	imgItem := content[1].(map[string]any)
	if imgItem["type"] != "image_url" || imgItem["image_url"].(map[string]any)["url"] != "https://example.com/a.png" {
		t.Fatalf("image item = %+v", imgItem)
	}
}

func TestBuildArkVideoTaskBodyImageToVideoObject(t *testing.T) {
	in := []byte(`{"model":"m","image_url":{"url":"https://example.com/b.png"}}`)
	out, err := buildArkVideoTaskBody(in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	content := payload["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["type"] != "image_url" {
		t.Fatalf("content = %+v", content)
	}
}

func TestBuildArkVideoTaskBodyEmptyContentTreatedAsFlat(t *testing.T) {
	in := []byte(`{"model":"m","prompt":"p","content":[]}`)
	out, err := buildArkVideoTaskBody(in)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	content := payload["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["text"] != "p" {
		t.Fatalf("empty content array should be rebuilt from prompt, got %+v", content)
	}
}

func TestBuildArkVideoTaskBodyRequiresPrompt(t *testing.T) {
	if _, err := buildArkVideoTaskBody([]byte(`{"model":"m"}`)); err == nil {
		t.Fatal("expected error when neither content nor prompt present")
	}
	if _, err := buildArkVideoTaskBody([]byte(`not-json`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
	if _, err := buildArkVideoTaskBody(nil); err == nil {
		t.Fatal("expected error for empty body")
	}
}
