package compression

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
)

// Compressor defines the interface for cache value compression
type Compressor interface {
	// Compress compresses the given data and returns compressed bytes
	Compress(data []byte) ([]byte, error)

	// Decompress decompresses the given compressed bytes
	Decompress(compressed []byte) ([]byte, error)

	// Name returns the name/identifier of the compressor
	Name() string
}

// CompressorType represents different compression algorithms
type CompressorType string

const (
	CompressorNone    CompressorType = "none"
	CompressorGzip    CompressorType = "gzip"
	CompressorDeflate CompressorType = "deflate"
)

// Config holds compression configuration
type Config struct {
	// Enabled determines whether compression is enabled
	Enabled bool

	// Algorithm specifies which compression algorithm to use
	Algorithm CompressorType

	// MinSize is the minimum size in bytes before compression is applied
	// Values smaller than this will not be compressed to avoid overhead
	MinSize int

	// Level is the compression level (1-9 for gzip/deflate, -1 for default)
	Level int
}

// NewDefaultConfig creates a default compression configuration
func NewDefaultConfig() *Config {
	return &Config{
		Enabled:   false, // Disabled by default
		Algorithm: CompressorGzip,
		MinSize:   1024, // 1KB minimum
		Level:     -1,   // Default level
	}
}

// WithEnabled sets whether compression is enabled
func (c *Config) WithEnabled(enabled bool) *Config {
	c.Enabled = enabled
	return c
}

// WithAlgorithm sets the compression algorithm
func (c *Config) WithAlgorithm(algorithm CompressorType) *Config {
	c.Algorithm = algorithm
	return c
}

// WithMinSize sets the minimum size threshold for compression
func (c *Config) WithMinSize(minSize int) *Config {
	c.MinSize = minSize
	return c
}

// WithLevel sets the compression level
func (c *Config) WithLevel(level int) *Config {
	c.Level = level
	return c
}

// NoOpCompressor provides a no-op implementation that doesn't compress
type NoOpCompressor struct{}

// NewNoOpCompressor creates a new no-op compressor
func NewNoOpCompressor() *NoOpCompressor {
	return &NoOpCompressor{}
}

// Compress returns the data unchanged
func (n *NoOpCompressor) Compress(data []byte) ([]byte, error) {
	return data, nil
}

// Decompress returns the data unchanged
func (n *NoOpCompressor) Decompress(compressed []byte) ([]byte, error) {
	return compressed, nil
}

// Name returns the compressor name
func (n *NoOpCompressor) Name() string {
	return "none"
}

// GzipCompressor implements compression using gzip
type GzipCompressor struct {
	level int
}

// NewGzipCompressor creates a new gzip compressor with the specified level
func NewGzipCompressor(level int) *GzipCompressor {
	return &GzipCompressor{level: level}
}

// Compress compresses data using gzip
func (g *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	writer, err := gzip.NewWriterLevel(&buf, g.level)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip writer: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		_ = writer.Close() // Ignore error on cleanup path
		return nil, fmt.Errorf("failed to write compressed data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress decompresses gzip data
func (g *GzipCompressor) Decompress(compressed []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		_ = reader.Close() // Ignore error on defer
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read decompressed data: %w", err)
	}

	return data, nil
}

// Name returns the compressor name
func (g *GzipCompressor) Name() string {
	return "gzip"
}

// DeflateCompressor implements compression using zlib/deflate
type DeflateCompressor struct {
	level int
}

// NewDeflateCompressor creates a new deflate compressor with the specified level
func NewDeflateCompressor(level int) *DeflateCompressor {
	return &DeflateCompressor{level: level}
}

// Compress compresses data using deflate
func (d *DeflateCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	writer, err := zlib.NewWriterLevel(&buf, d.level)
	if err != nil {
		return nil, fmt.Errorf("failed to create deflate writer: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		_ = writer.Close() // Ignore error on cleanup path
		return nil, fmt.Errorf("failed to write compressed data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close deflate writer: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress decompresses deflate data
func (d *DeflateCompressor) Decompress(compressed []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("failed to create deflate reader: %w", err)
	}
	defer func() {
		_ = reader.Close() // Ignore error on defer
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read decompressed data: %w", err)
	}

	return data, nil
}

// Name returns the compressor name
func (d *DeflateCompressor) Name() string {
	return "deflate"
}

// NewCompressor creates a new compressor based on the configuration
func NewCompressor(config *Config) (Compressor, error) {
	if config == nil || !config.Enabled {
		return NewNoOpCompressor(), nil
	}

	switch config.Algorithm {
	case CompressorNone:
		return NewNoOpCompressor(), nil
	case CompressorGzip:
		return NewGzipCompressor(config.Level), nil
	case CompressorDeflate:
		return NewDeflateCompressor(config.Level), nil
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", config.Algorithm)
	}
}

// SerializeAndCompress converts a value to bytes and compresses it if it meets size threshold
func SerializeAndCompress(value any, compressor Compressor, minSize int) ([]byte, bool, error) {
	// Serialize the value to bytes using JSON encoding (more compatible than gob)
	serialized, err := json.Marshal(value)
	if err != nil {
		return nil, false, fmt.Errorf("failed to serialize value: %w", err)
	}

	// Only compress if the serialized data meets the minimum size threshold
	if len(serialized) < minSize {
		return serialized, false, nil
	}

	compressed, err := compressor.Compress(serialized)
	if err != nil {
		return nil, false, fmt.Errorf("failed to compress data: %w", err)
	}

	// Only use compression if it actually reduces size
	if len(compressed) >= len(serialized) {
		return serialized, false, nil
	}

	return compressed, true, nil
}

// DecompressAndDeserialize decompresses and deserializes data back to a value
func DecompressAndDeserialize(data []byte, isCompressed bool, compressor Compressor, target any) error {
	var serialized []byte
	var err error

	if isCompressed {
		serialized, err = compressor.Decompress(data)
		if err != nil {
			return fmt.Errorf("failed to decompress data: %w", err)
		}
	} else {
		serialized = data
	}

	// Deserialize using JSON
	if err := json.Unmarshal(serialized, target); err != nil {
		return fmt.Errorf("failed to deserialize value: %w", err)
	}

	return nil
}

// Ensure interfaces are implemented
var (
	_ Compressor = (*NoOpCompressor)(nil)
	_ Compressor = (*GzipCompressor)(nil)
	_ Compressor = (*DeflateCompressor)(nil)
)
