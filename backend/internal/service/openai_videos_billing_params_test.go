package service

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOpenAIVideosBillingParams_JSON(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantRes string
		wantDur int
		wantN   int
	}{
		{"resolution+duration+n", `{"model":"x","resolution":"1080p","duration":10,"n":2}`, "1080p", 10, 2},
		{"size WxH + seconds", `{"size":"1920x1080","seconds":5}`, "1920x1080", 5, 0},
		{"resolution preferred over size", `{"resolution":"720p","size":"1080p"}`, "720p", 0, 0},
		{"duration preferred over seconds", `{"duration":12,"seconds":3}`, "", 12, 0},
		{"empty body object", `{"model":"x"}`, "", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, dur, n := parseOpenAIVideosBillingParams("application/json", []byte(tc.body))
			require.Equal(t, tc.wantRes, res)
			require.Equal(t, tc.wantDur, dur)
			require.Equal(t, tc.wantN, n)
		})
	}
}

func TestParseOpenAIVideosBillingParams_Multipart(t *testing.T) {
	build := func(t *testing.T, fields map[string]string) (string, []byte) {
		t.Helper()
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for k, v := range fields {
			require.NoError(t, w.WriteField(k, v))
		}
		require.NoError(t, w.Close())
		return w.FormDataContentType(), buf.Bytes()
	}

	t.Run("resolution+duration+n", func(t *testing.T) {
		ct, body := build(t, map[string]string{"resolution": "720p", "duration": "8", "n": "3"})
		res, dur, n := parseOpenAIVideosBillingParams(ct, body)
		require.Equal(t, "720p", res)
		require.Equal(t, 8, dur)
		require.Equal(t, 3, n)
	})

	t.Run("size WxH + seconds", func(t *testing.T) {
		ct, body := build(t, map[string]string{"size": "3840x2160", "seconds": "5"})
		res, dur, n := parseOpenAIVideosBillingParams(ct, body)
		require.Equal(t, "3840x2160", res)
		require.Equal(t, 5, dur)
		require.Equal(t, 0, n)
	})

	t.Run("empty body", func(t *testing.T) {
		res, dur, n := parseOpenAIVideosBillingParams("", nil)
		require.Equal(t, "", res)
		require.Equal(t, 0, dur)
		require.Equal(t, 0, n)
	})
}
