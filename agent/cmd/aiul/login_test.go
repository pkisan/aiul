package main

import "testing"

func TestPairURL(t *testing.T) {
	got, err := pairURL("https://pm.example.com/api/aiul/events/")
	if err != nil || got != "https://pm.example.com/api/aiul/pair" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := pairURL("https://pm.example.com/api/other"); err == nil {
		t.Fatal("an endpoint not ending in /events must be refused, not guessed at")
	}
}
