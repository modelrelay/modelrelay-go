package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

// ImageRequest is a request to generate images from a text prompt.
type ImageRequest = generated.ImageRequest

// ImageResponse contains generated images.
type ImageResponse = generated.ImageResponse

// ImageData contains a single generated image.
type ImageData = generated.ImageData

// ImageUsage contains usage statistics for image generation.
type ImageUsage = generated.ImageUsage

// ImageResponseFormat controls the output format for generated images.
type ImageResponseFormat = generated.ImageResponseFormat

// ImageResponseFormat constants.
const (
	ImageResponseFormatURL     ImageResponseFormat = generated.Url
	ImageResponseFormatB64JSON ImageResponseFormat = generated.B64Json
)

// ImagesClient provides methods for generating images using AI models.
//
// Example:
//
//	// Production use (default) - returns URLs
//	resp, err := client.Images.Generate(ctx, sdk.ImageRequest{
//	    Model:  "gemini-2.5-flash-image",
//	    Prompt: "A futuristic cityscape",
//	})
//	fmt.Println(resp.Data[0].Url)
//	fmt.Println(resp.Data[0].MimeType)
//
//	// Testing/development - returns base64
//	format := sdk.ImageResponseFormatB64JSON
//	resp, err := client.Images.Generate(ctx, sdk.ImageRequest{
//	    Model:          "gemini-2.5-flash-image",
//	    Prompt:         "A futuristic cityscape",
//	    ResponseFormat: &format,
//	})
type ImagesClient struct {
	client *Client
}

// ensureInitialized returns an error if the client is not properly initialized.
func (c *ImagesClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: images client not initialized")
	}
	return nil
}

// Generate creates images from a text prompt.
//
// By default, returns URLs (requires storage configuration on the server).
// Use ResponseFormat: &sdk.ImageResponseFormatB64JSON for testing without storage.
//
// Model is optional when using a customer token with a tier that defines a default model.
func (c *ImagesClient) Generate(ctx context.Context, req ImageRequest) (ImageResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return ImageResponse{}, err
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return ImageResponse{}, fmt.Errorf("sdk: prompt is required")
	}

	var resp ImageResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, "/images/generate", req, &resp); err != nil {
		return ImageResponse{}, err
	}
	return resp, nil
}
