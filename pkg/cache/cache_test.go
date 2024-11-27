package cache_test

import (
	"testing"
	"time"

	"github.com/dgate-io/dgate-api/pkg/cache"
	"github.com/stretchr/testify/assert"
)

func TestCache_GetSet(t *testing.T) {
	c := cache.New()
	num := 5
	c.Bucket("test").Set("key", num)
	n, ok := c.Bucket("test").Get("key")
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, num, n, "expected value to be %d, got %d", num, n)
}

func TestCache_Delete(t *testing.T) {
	c := cache.New()
	num := 5
	c.Bucket("test").Set("key", num)
	c.Bucket("test").Delete("key")
	_, ok := c.Bucket("test").Get("key")
	assert.False(t, ok, "expected key to be deleted")
}

func TestCache_SetWithTTL(t *testing.T) {
	c := cache.NewWithOpts(cache.CacheOptions{
		CheckInterval: time.Millisecond * 100,
	})
	num := 5
	c.Bucket("test").SetWithTTL("key", num, time.Millisecond*200)
	n, ok := c.Bucket("test").Get("key")
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, num, n, "expected value to be %d, got %d", num, n)
	time.Sleep(time.Millisecond * 300)
	_, ok = c.Bucket("test").Get("key")
	assert.False(t, ok, "expected key to be deleted")
}

func TestCache_MaxItems(t *testing.T) {
	c := cache.NewWithOpts(cache.CacheOptions{
		CheckInterval: time.Millisecond * 100,
	})
	c.BucketWithOpts("test", cache.BucketOptions{
		MaxItems: 2,
	})
	c.Bucket("test").Set("key1", 1)
	c.Bucket("test").Set("key2", 2)
	c.Bucket("test").Set("key3", 3)
	_, ok := c.Bucket("test").Get("key1")
	assert.False(t, ok, "expected key1 to be deleted")
	_, ok = c.Bucket("test").Get("key2")
	assert.True(t, ok, "expected key2 to be found")
	_, ok = c.Bucket("test").Get("key3")
	assert.True(t, ok, "expected key3 to be found")
}

func TestCache_MaxItems_TTL(t *testing.T) {
	c := cache.NewWithOpts(cache.CacheOptions{
		CheckInterval: time.Millisecond * 10,
	})
	c.BucketWithOpts("test", cache.BucketOptions{
		MaxItems: 2,
	})
	c.Bucket("test").SetWithTTL("key1", 1, time.Millisecond*10)
	c.Bucket("test").SetWithTTL("key2", 2, time.Millisecond*10)
	c.Bucket("test").SetWithTTL("key3", 3, time.Millisecond*100)

	var ok bool
	_, ok = c.Bucket("test").Get("key1")
	assert.False(t, ok, "expected key1 to be found")
	_, ok = c.Bucket("test").Get("key2")
	assert.True(t, ok, "expected key2 to be found")
	_, ok = c.Bucket("test").Get("key3")
	assert.True(t, ok, "expected key3 to be found")
	assert.Equal(t, 2, c.Bucket("test").Len(), "expected cache to be empty")

	time.Sleep(time.Millisecond * 50)
	_, ok = c.Bucket("test").Get("key2")
	assert.False(t, ok, "expected key2 to be deleted")
	_, ok = c.Bucket("test").Get("key3")
	assert.True(t, ok, "expected key3 to be found")
	assert.Equal(t, 1, c.Bucket("test").Len(), "expected cache to be empty")

	time.Sleep(time.Millisecond * 200)
	_, ok = c.Bucket("test").Get("key3")
	assert.False(t, ok, "expected key3 to be deleted")
	assert.Zero(t, c.Bucket("test").Len(), "expected cache to be empty")
}

func TestCache_MaxItems_Overwrite(t *testing.T) {
	c := cache.NewWithOpts(cache.CacheOptions{
		CheckInterval: time.Millisecond * 10,
	})
	c.BucketWithOpts("test", cache.BucketOptions{
		MaxItems: 2,
	})
	c.Bucket("test").SetWithTTL("key", 1, time.Millisecond*10)
	n, ok := c.Bucket("test").Get("key")
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 1, n, "expected value to be 1, got %d", n)

	c.Bucket("test").SetWithTTL("key", 2, time.Millisecond*100)
	n, ok = c.Bucket("test").Get("key")
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 2, n, "expected value to be 2, got %d", n)

	time.Sleep(time.Millisecond * 50)
	_, ok = c.Bucket("test").Get("key")
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 2, n, "expected value to be 2, got %d", n)

	time.Sleep(time.Millisecond * 100)
	_, ok = c.Bucket("test").Get("key")
	assert.False(t, ok, "expected key to be deleted")
}
