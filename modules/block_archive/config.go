package blockarchive

// DefaultURL is the production block-archiver public endpoint.
const DefaultURL = "https://block-archiver.analytics.production.platform.ethpandaops.io"

// Config holds the Block Archive module configuration.
// Block Archive is a public service and requires no credentials.
type Config struct {
	// URL is the base URL of the block-archiver service. Defaults to the
	// production endpoint.
	URL string `yaml:"url,omitempty"`
}

// baseURL returns the configured URL or the default.
func (c *Config) baseURL() string {
	if c.URL == "" {
		return DefaultURL
	}

	return c.URL
}
