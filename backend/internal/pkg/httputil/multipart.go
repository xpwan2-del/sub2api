package httputil

import (
	"bytes"
	"mime"
	"mime/multipart"
	"strings"
)

// maxMultipartModelMemory 限制解析 multipart form 时载入内存的字节数，
// 与 OpenAI 网关视频/图片入口保持一致。
const maxMultipartModelMemory = 32 << 20

// ExtractModelFromMultipart 从 multipart/form-data 请求体中提取 "model" 字段值。
//
// 专为 bundle 路由解析等只能拿到原始 body + Content-Type 的中间件场景设计：
// 它不依赖 *http.Request，不消费任何 io.Reader，调用方的 body 字节保持原样可用。
// 解析失败（非法 Content-Type、损坏 body、缺 model 字段）统一返回空字符串。
func ExtractModelFromMultipart(body []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(maxMultipartModelMemory)
	if err != nil {
		return ""
	}
	defer func() { _ = form.RemoveAll() }()
	if values := form.Value["model"]; len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}
