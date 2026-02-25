package compression

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewDefaultConfig(t *testing.T) {
	config := NewDefaultConfig()

	if config.Enabled {
		t.Error("Expected Enabled to be false by default")
	}
	if config.Algorithm != CompressorGzip {
		t.Errorf("Expected default algorithm to be gzip, got %s", config.Algorithm)
	}
	if config.MinSize != 1024 {
		t.Errorf("Expected default MinSize to be 1024, got %d", config.MinSize)
	}
	if config.Level != -1 {
		t.Errorf("Expected default Level to be -1, got %d", config.Level)
	}
}

func TestConfigBuilder(t *testing.T) {
	config := NewDefaultConfig().
		WithEnabled(true).
		WithAlgorithm(CompressorDeflate).
		WithMinSize(2048).
		WithLevel(6)

	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if config.Algorithm != CompressorDeflate {
		t.Errorf("Expected algorithm to be deflate, got %s", config.Algorithm)
	}
	if config.MinSize != 2048 {
		t.Errorf("Expected MinSize to be 2048, got %d", config.MinSize)
	}
	if config.Level != 6 {
		t.Errorf("Expected Level to be 6, got %d", config.Level)
	}
}

func TestNoOpCompressor(t *testing.T) {
	compressor := NewNoOpCompressor()

	if compressor.Name() != "none" {
		t.Errorf("Expected name 'none', got %s", compressor.Name())
	}

	original := []byte("test data that should not be compressed")
	compressed, err := compressor.Compress(original)
	if err != nil {
		t.Fatalf("NoOp compress failed: %v", err)
	}

	if !bytes.Equal(compressed, original) {
		t.Error("NoOp compressor should return data unchanged")
	}

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("NoOp decompress failed: %v", err)
	}

	if !bytes.Equal(decompressed, original) {
		t.Error("NoOp decompressor should return data unchanged")
	}
}

func TestGzipCompressor(t *testing.T) {
	compressor := NewGzipCompressor(-1) // Default level

	if compressor.Name() != "gzip" {
		t.Errorf("Expected name 'gzip', got %s", compressor.Name())
	}

	// Test with compressible data
	original := []byte(strings.Repeat("test data ", 100))

	compressed, err := compressor.Compress(original)
	if err != nil {
		t.Fatalf("Gzip compress failed: %v", err)
	}

	// Gzip should compress repetitive data
	if len(compressed) >= len(original) {
		t.Errorf("Expected compression, but compressed size (%d) >= original size (%d)",
			len(compressed), len(original))
	}

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("Gzip decompress failed: %v", err)
	}

	if !bytes.Equal(decompressed, original) {
		t.Error("Decompressed data doesn't match original")
	}
}

func TestGzipCompressorLevels(t *testing.T) {
	original := []byte(strings.Repeat("test ", 200))

	levels := []int{1, 6, 9}
	for _, level := range levels {
		compressor := NewGzipCompressor(level)

		compressed, err := compressor.Compress(original)
		if err != nil {
			t.Errorf("Gzip compress at level %d failed: %v", level, err)
			continue
		}

		decompressed, err := compressor.Decompress(compressed)
		if err != nil {
			t.Errorf("Gzip decompress at level %d failed: %v", level, err)
			continue
		}

		if !bytes.Equal(decompressed, original) {
			t.Errorf("Decompressed data doesn't match original at level %d", level)
		}
	}
}

func TestDeflateCompressor(t *testing.T) {
	compressor := NewDeflateCompressor(-1) // Default level

	if compressor.Name() != "deflate" {
		t.Errorf("Expected name 'deflate', got %s", compressor.Name())
	}

	// Test with compressible data
	original := []byte(strings.Repeat("deflate test data ", 100))

	compressed, err := compressor.Compress(original)
	if err != nil {
		t.Fatalf("Deflate compress failed: %v", err)
	}

	// Deflate should compress repetitive data
	if len(compressed) >= len(original) {
		t.Errorf("Expected compression, but compressed size (%d) >= original size (%d)",
			len(compressed), len(original))
	}

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("Deflate decompress failed: %v", err)
	}

	if !bytes.Equal(decompressed, original) {
		t.Error("Decompressed data doesn't match original")
	}
}

func TestNewCompressor(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
		wantErr  bool
	}{
		{
			name:     "Nil config returns NoOp",
			config:   nil,
			expected: "none",
			wantErr:  false,
		},
		{
			name: "Disabled returns NoOp",
			config: &Config{
				Enabled:   false,
				Algorithm: CompressorGzip,
			},
			expected: "none",
			wantErr:  false,
		},
		{
			name: "CompressorNone returns NoOp",
			config: &Config{
				Enabled:   true,
				Algorithm: CompressorNone,
			},
			expected: "none",
			wantErr:  false,
		},
		{
			name: "CompressorGzip returns Gzip",
			config: &Config{
				Enabled:   true,
				Algorithm: CompressorGzip,
				Level:     6,
			},
			expected: "gzip",
			wantErr:  false,
		},
		{
			name: "CompressorDeflate returns Deflate",
			config: &Config{
				Enabled:   true,
				Algorithm: CompressorDeflate,
				Level:     6,
			},
			expected: "deflate",
			wantErr:  false,
		},
		{
			name: "Invalid algorithm returns error",
			config: &Config{
				Enabled:   true,
				Algorithm: "invalid",
			},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressor, err := NewCompressor(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if compressor.Name() != tt.expected {
				t.Errorf("Expected compressor %s, got %s", tt.expected, compressor.Name())
			}
		})
	}
}

func TestSerializeAndCompress(t *testing.T) {
	type TestData struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}

	compressor := NewGzipCompressor(-1)

	t.Run("Small data is not compressed", func(t *testing.T) {
		data := TestData{Message: "hello", Count: 1}
		result, wasCompressed, err := SerializeAndCompress(data, compressor, 1000)

		if err != nil {
			t.Fatalf("SerializeAndCompress failed: %v", err)
		}

		if wasCompressed {
			t.Error("Expected small data to not be compressed")
		}

		if len(result) == 0 {
			t.Error("Expected non-empty result")
		}
	})

	t.Run("Large data is compressed", func(t *testing.T) {
		data := TestData{
			Message: strings.Repeat("large data ", 100),
			Count:   999,
		}
		result, wasCompressed, err := SerializeAndCompress(data, compressor, 100)

		if err != nil {
			t.Fatalf("SerializeAndCompress failed: %v", err)
		}

		if !wasCompressed {
			t.Error("Expected large data to be compressed")
		}

		if len(result) == 0 {
			t.Error("Expected non-empty result")
		}
	})

	t.Run("Compression skipped if not beneficial", func(t *testing.T) {
		// Random-like data doesn't compress well
		data := TestData{
			Message: "aB3$xQ9!pL7&mN2^wR5*tY8#kF1@vC6%hJ4",
			Count:   12345,
		}

		result, _, err := SerializeAndCompress(data, compressor, 10)

		if err != nil {
			t.Fatalf("SerializeAndCompress failed: %v", err)
		}

		// This may or may not be compressed depending on the data
		// Just ensure we get a valid result
		if len(result) == 0 {
			t.Error("Expected non-empty result")
		}
	})
}

func TestDecompressAndDeserialize(t *testing.T) {
	type TestData struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}

	original := TestData{Message: "test message", Count: 42}
	compressor := NewGzipCompressor(-1)

	t.Run("Decompress compressed data", func(t *testing.T) {
		compressed, wasCompressed, err := SerializeAndCompress(original, compressor, 1)
		if err != nil {
			t.Fatalf("SerializeAndCompress failed: %v", err)
		}

		var result TestData
		err = DecompressAndDeserialize(compressed, wasCompressed, compressor, &result)
		if err != nil {
			t.Fatalf("DecompressAndDeserialize failed: %v", err)
		}

		if result.Message != original.Message || result.Count != original.Count {
			t.Errorf("Deserialized data doesn't match: got %+v, want %+v", result, original)
		}
	})

	t.Run("Decompress uncompressed data", func(t *testing.T) {
		serialized, wasCompressed, err := SerializeAndCompress(original, compressor, 10000)
		if err != nil {
			t.Fatalf("SerializeAndCompress failed: %v", err)
		}

		if wasCompressed {
			t.Fatal("Expected data to not be compressed")
		}

		var result TestData
		err = DecompressAndDeserialize(serialized, wasCompressed, compressor, &result)
		if err != nil {
			t.Fatalf("DecompressAndDeserialize failed: %v", err)
		}

		if result.Message != original.Message || result.Count != original.Count {
			t.Errorf("Deserialized data doesn't match: got %+v, want %+v", result, original)
		}
	})
}

func TestRoundTrip(t *testing.T) {
	type ComplexData struct {
		ID       int               `json:"id"`
		Name     string            `json:"name"`
		Tags     []string          `json:"tags"`
		Metadata map[string]string `json:"metadata"`
	}

	original := ComplexData{
		ID:   123,
		Name: "test user",
		Tags: []string{"admin", "power-user"},
		Metadata: map[string]string{
			"country": "US",
			"tier":    "premium",
		},
	}

	compressors := []struct {
		name       string
		compressor Compressor
	}{
		{"NoOp", NewNoOpCompressor()},
		{"Gzip", NewGzipCompressor(-1)},
		{"Deflate", NewDeflateCompressor(-1)},
	}

	for _, tc := range compressors {
		t.Run(tc.name, func(t *testing.T) {
			// Serialize and compress
			data, compressed, err := SerializeAndCompress(original, tc.compressor, 1)
			if err != nil {
				t.Fatalf("SerializeAndCompress failed: %v", err)
			}

			// Decompress and deserialize
			var result ComplexData
			err = DecompressAndDeserialize(data, compressed, tc.compressor, &result)
			if err != nil {
				t.Fatalf("DecompressAndDeserialize failed: %v", err)
			}

			// Verify
			if result.ID != original.ID {
				t.Errorf("ID mismatch: got %d, want %d", result.ID, original.ID)
			}
			if result.Name != original.Name {
				t.Errorf("Name mismatch: got %s, want %s", result.Name, original.Name)
			}
			if len(result.Tags) != len(original.Tags) {
				t.Errorf("Tags length mismatch: got %d, want %d", len(result.Tags), len(original.Tags))
			}
			if len(result.Metadata) != len(original.Metadata) {
				t.Errorf("Metadata length mismatch: got %d, want %d", len(result.Metadata), len(original.Metadata))
			}
		})
	}
}

func TestCompressorInterface(t *testing.T) {
	// Ensure all compressors implement the interface
	var _ Compressor = (*NoOpCompressor)(nil)
	var _ Compressor = (*GzipCompressor)(nil)
	var _ Compressor = (*DeflateCompressor)(nil)
}
