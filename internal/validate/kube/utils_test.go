package kube

import "testing"

func TestContains(t *testing.T) {
    sl := []string{"one", "two", "three"}

    t.Run("should return true if slice contains target", func(t *testing.T) {
        got := Contains(&sl, "two")
        want := true
        if got != want {
            t.Errorf("got %v, want %v", got, want)
        }
    })
    
    t.Run("should return false if slice does not contain target", func(t *testing.T) {
        got := Contains(&sl, "four")
        want := false
        if got != want {
            t.Errorf("got %v, want %v", got, want)
        }
    })
}
