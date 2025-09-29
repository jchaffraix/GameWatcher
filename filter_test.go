package main

import (
  "testing"
)

func TestBestMatch(t *testing.T) {
  tt := []struct {
    name string
    gameName string
    results []string
    expected string
  } {
    {"No match", "Foobar", []string{}, ""},

    {"Doesn't match DLC", "Foobar", []string{"Foobar DLC"}, ""},
    {"Doesn't match Soundtrack", "Foobar", []string{"Foobar Soundtrack"}, ""},
    {"Doesn't match OST", "Foobar", []string{"Foobar OST"}, ""},
    {"Doesn't match Artbook", "Foobar", []string{"Foobar Artbook"}, ""},
    {"Doesn't match Adventure Pack", "Foobar", []string{"Foobar Adventure Pack"}, ""},
    {"Doesn't match Content Pack", "Foobar", []string{"Foobar Content Pack"}, ""},
    {"Doesn't match Costume Pack", "Foobar", []string{"Foobar Costume Pack"}, ""},
    {"Doesn't match Season Pass", "Foobar", []string{"Foobar Season Pass"}, ""},
    {"Doesn't match Demo", "Foobar", []string{"Foobar Demo"}, ""},

    {"Ignores PC for direct matching", "Foobar", []string{"Foobar 2", "Foobar PC"}, "Foobar PC"},

    {"Matches Deluxe", "Foobar", []string{"Foobar Deluxe"}, "Foobar Deluxe"},

    {"Prefers base game rather than Deluxe", "Foobar", []string{"Foobar", "Foobar Deluxe"}, "Foobar"},
    {"Prefers base game rather than DLC", "Foobar", []string{"Foobar - extra content", "Foobar"}, "Foobar"},
  }

  for _, tc := range(tt) {
    t.Run(tc.name, func(t *testing.T) {
      output := BestMatch(tc.gameName, tc.results)
      if output != tc.expected {
        t.Errorf("Expected %s but got %s", tc.expected, output)
        return
      }
    })
  }
}
