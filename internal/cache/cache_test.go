package cache

import (
	"testing"
	"time"
)

func TestSetAndGet(t *testing.T) {
	c := New(1 * time.Minute)
	c.Set("k1", "hello")

	val, ok := c.Get("k1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "hello" {
		t.Errorf("got %v, want hello", val)
	}
}

func TestMiss(t *testing.T) {
	c := New(1 * time.Minute)
	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestExpiry(t *testing.T) {
	c := New(1 * time.Millisecond)
	c.Set("k1", "value")
	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("k1")
	if ok {
		t.Error("expected expired entry to be a miss")
	}
}

func TestDelete(t *testing.T) {
	c := New(1 * time.Minute)
	c.Set("k1", "value")
	c.Delete("k1")

	_, ok := c.Get("k1")
	if ok {
		t.Error("expected miss after delete")
	}
}

func TestClear(t *testing.T) {
	c := New(1 * time.Minute)
	c.Set("k1", "v1")
	c.Set("k2", "v2")
	c.Clear()

	if _, ok := c.Get("k1"); ok {
		t.Error("expected miss after clear")
	}
	if _, ok := c.Get("k2"); ok {
		t.Error("expected miss after clear")
	}
}

func TestKey(t *testing.T) {
	got := Key("github", "releases", "golang", "go")
	want := "github:releases:golang:go"
	if got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}
