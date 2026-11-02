package obcache

import "testing"

const testKeyConst = "test-key"

func TestRedisConfigBuilders(t *testing.T) {
	// Test NewRedisConfig
	config1 := NewRedisConfig("localhost:6379")
	if config1.StoreType != StoreTypeRedis {
		t.Fatal("Expected StoreType to be Redis")
	}
	if config1.Redis.Addr != "localhost:6379" {
		t.Fatalf("Expected Redis addr 'localhost:6379', got '%s'", config1.Redis.Addr)
	}
	if config1.Redis.KeyPrefix != "obcache2:" {
		t.Fatalf("Expected default key prefix 'obcache2:', got '%s'", config1.Redis.KeyPrefix)
	}

	// Test configuration chaining
	config2 := NewDefaultConfig().
		WithRedis(&RedisConfig{
			Addr:      "redis:6379",
			Password:  "password",
			DB:        2,
			KeyPrefix: "myapp:",
		})

	if config2.StoreType != StoreTypeRedis {
		t.Fatal("Expected StoreType to be Redis after WithRedis")
	}
	if config2.Redis.Addr != "redis:6379" {
		t.Fatalf("Expected Redis addr 'redis:6379', got '%s'", config2.Redis.Addr)
	}
	if config2.Redis.Password != "password" {
		t.Fatalf("Expected Redis password 'password', got '%s'", config2.Redis.Password)
	}
	if config2.Redis.DB != 2 {
		t.Fatalf("Expected Redis DB 2, got %d", config2.Redis.DB)
	}
	if config2.Redis.KeyPrefix != "myapp:" {
		t.Fatalf("Expected Redis key prefix 'myapp:', got '%s'", config2.Redis.KeyPrefix)
	}
}
