package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPublicModelCatalogFromGatewayModels(t *testing.T) {
	catalog := buildPublicModelCatalogFromGatewayModels([]string{
		"grok-imagine-video",
		"claude-sonnet-4",
		"",
		"gpt-4o-mini",
	})

	require.Len(t, catalog, 3)
	require.Equal(t, "claude-sonnet-4", catalog[0].Name)
	require.Equal(t, "anthropic", catalog[0].Platform)
	require.Contains(t, catalog[0].Capabilities, "coding")
	require.Equal(t, "gpt-4o-mini", catalog[1].Name)
	require.Equal(t, "openai", catalog[1].Platform)
	require.Equal(t, "grok-imagine-video", catalog[2].Name)
	require.Equal(t, "xai", catalog[2].Platform)
	require.Contains(t, catalog[2].Capabilities, "multimodal")
}
