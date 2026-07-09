package httputil

import (
	"bytes"
	"mime/multipart"
	"testing"
)

// writeMultipartForm 构造一个 multipart/form-data body，返回 body 字节与对应 Content-Type。
// 字段顺序通过有序写入保证，便于断言。
func writeMultipartForm(t *testing.T, fields map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field %q: %v", k, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return buf.Bytes(), w.FormDataContentType()
}

func TestExtractModelFromMultipart_ReturnsModelField(t *testing.T) {
	body, contentType := writeMultipartForm(t, map[string]string{
		"model":  "grok-imagine-video",
		"prompt": "a cat",
	})

	got := ExtractModelFromMultipart(body, contentType)
	if got != "grok-imagine-video" {
		t.Fatalf("expected model %q, got %q", "grok-imagine-video", got)
	}
}

func TestExtractModelFromMultipart_TrimsWhitespace(t *testing.T) {
	body, contentType := writeMultipartForm(t, map[string]string{
		"model": "  gpt-4o  ",
	})

	got := ExtractModelFromMultipart(body, contentType)
	if got != "gpt-4o" {
		t.Fatalf("expected trimmed model %q, got %q", "gpt-4o", got)
	}
}

func TestExtractModelFromMultipart_EmptyWhenNoModelField(t *testing.T) {
	body, contentType := writeMultipartForm(t, map[string]string{
		"prompt": "a cat",
	})

	if got := ExtractModelFromMultipart(body, contentType); got != "" {
		t.Fatalf("expected empty model when field absent, got %q", got)
	}
}

func TestExtractModelFromMultipart_EmptyWhenContentTypeInvalid(t *testing.T) {
	body, _ := writeMultipartForm(t, map[string]string{"model": "grok-imagine-video"})

	if got := ExtractModelFromMultipart(body, "not-a-valid-content-type"); got != "" {
		t.Fatalf("expected empty model for invalid content type, got %q", got)
	}
}

func TestExtractModelFromMultipart_EmptyWhenBodyCorrupt(t *testing.T) {
	_, contentType := writeMultipartForm(t, map[string]string{"model": "grok-imagine-video"})

	if got := ExtractModelFromMultipart([]byte("not multipart"), contentType); got != "" {
		t.Fatalf("expected empty model for corrupt body, got %q", got)
	}
}
