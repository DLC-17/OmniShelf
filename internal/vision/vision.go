// Package vision wraps the Google Cloud Vision ImageAnnotatorClient for the
// trading-card OCR flow: one method, DetectText, returns the full merged text
// block Vision read off a photo. The underlying gRPC client is created lazily
// on first use so startup never blocks on GCP; a missing credentials file
// leaves the client unconfigured and every call yields ErrNotConfigured, so
// the cards module degrades gracefully (same pattern as internal/discogs).
package vision

import (
	"context"
	"errors"
	"fmt"
	"sync"

	visionapi "cloud.google.com/go/vision/v2/apiv1"
	"cloud.google.com/go/vision/v2/apiv1/visionpb"
	"google.golang.org/api/option"
)

// Sentinel errors callers translate into the API envelope.
var (
	// ErrNotConfigured means no service-account credentials file was supplied
	// at startup.
	ErrNotConfigured = errors.New("vision: not configured")
	// ErrNoText means Vision found no text annotations on the image.
	ErrNoText = errors.New("vision: no text detected")
	// ErrUpstream means the Vision API call (or the lazy client setup) failed.
	ErrUpstream = errors.New("vision: service unavailable")
)

// Client performs OCR through the Google Cloud Vision API.
type Client struct {
	credentialsFile string

	mu     sync.Mutex
	client *visionapi.ImageAnnotatorClient
}

// New returns a Vision client that authenticates with the given
// service-account JSON file. When credentialsFile is empty the client is
// considered unconfigured and DetectText returns ErrNotConfigured. No
// connection is made here: the gRPC client is initialized lazily on first use.
func New(credentialsFile string) *Client {
	return &Client{credentialsFile: credentialsFile}
}

// Configured reports whether a credentials file path was supplied.
func (c *Client) Configured() bool {
	return c.credentialsFile != ""
}

// DetectText runs Vision text detection on the image bytes and returns the
// full merged text block (annotation index 0, lines joined with "\n"). An
// image with no readable text yields ErrNoText; a failed API call or client
// setup yields an error matching ErrUpstream. The caller owns the context
// deadline (the cards service applies an 8s budget per scan).
func (c *Client) DetectText(ctx context.Context, imageBytes []byte) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	client, err := c.get(ctx)
	if err != nil {
		return "", errors.Join(ErrUpstream, err)
	}

	resp, err := client.BatchAnnotateImages(ctx, &visionpb.BatchAnnotateImagesRequest{
		Requests: []*visionpb.AnnotateImageRequest{{
			Image:    &visionpb.Image{Content: imageBytes},
			Features: []*visionpb.Feature{{Type: visionpb.Feature_TEXT_DETECTION}},
		}},
	})
	if err != nil {
		return "", errors.Join(ErrUpstream, err)
	}
	if len(resp.GetResponses()) == 0 {
		return "", errors.Join(ErrUpstream, errors.New("empty batch response"))
	}
	res := resp.GetResponses()[0]
	if e := res.GetError(); e != nil {
		return "", errors.Join(ErrUpstream, fmt.Errorf("annotate: %s", e.GetMessage()))
	}
	annotations := res.GetTextAnnotations()
	if len(annotations) == 0 || annotations[0].GetDescription() == "" {
		return "", ErrNoText
	}
	return annotations[0].GetDescription(), nil
}

// get returns the shared ImageAnnotatorClient, creating it on first use. A
// failed creation is not cached, so a transient GCP outage at first scan does
// not wedge the client forever.
func (c *Client) get(ctx context.Context) (*visionapi.ImageAnnotatorClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	client, err := visionapi.NewImageAnnotatorClient(
		ctx,
		option.WithAuthCredentialsFile(option.ServiceAccount, c.credentialsFile),
	)
	if err != nil {
		return nil, err
	}
	c.client = client
	return c.client, nil
}
