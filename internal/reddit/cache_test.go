package reddit

import (
	"testing"
	"time"
)

func TestCacheTTL(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	c := NewCache(10, clock.Now)
	c.Put("a", []byte("1"), time.Minute)
	if v, ok := c.Get("a"); !ok || string(v) != "1" {
		t.Fatal("miss")
	}
	clock.Advance(61 * time.Second)
	if _, ok := c.Get("a"); ok {
		t.Error("expired entry returned")
	}
}

func TestCacheLRUEviction(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	c := NewCache(2, clock.Now)
	c.Put("a", []byte("1"), time.Hour)
	c.Put("b", []byte("2"), time.Hour)
	c.Get("a") // a is now most recent
	c.Put("c", []byte("3"), time.Hour)
	if _, ok := c.Get("b"); ok {
		t.Error("b should have been evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("a should survive")
	}
	if c.Len() != 2 {
		t.Errorf("len = %d", c.Len())
	}
}

func TestCachePutReplaces(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_790_000_000, 0)}
	c := NewCache(2, clock.Now)
	c.Put("a", []byte("1"), time.Hour)
	c.Put("a", []byte("2"), time.Hour)
	if v, _ := c.Get("a"); string(v) != "2" || c.Len() != 1 {
		t.Errorf("v=%s len=%d", v, c.Len())
	}
}
