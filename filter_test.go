package main

import (
  "testing"
)

func gg(name string) GenericGame {
  return GenericGame{name, 12.3, ""}
}

func TestBestMatch(t *testing.T) {
  tt := []struct {
    name string
    game string
    results []GenericGame
    expected int
  } {
    {"No match", "Foobar", []GenericGame{}, -1},

    {"Doesn't match DLC", "Foobar", []GenericGame{gg("Foobar DLC")}, -1},
    {"Doesn't match Soundtrack", "Foobar", []GenericGame{gg("Foobar Soundtrack")}, -1},
    {"Doesn't match OST", "Foobar", []GenericGame{gg("Foobar OST")}, -1},
    {"Doesn't match Artbook", "Foobar", []GenericGame{gg("Foobar Artbook")}, -1},
    {"Doesn't match Adventure Pack", "Foobar", []GenericGame{gg("Foobar Adventure Pack")}, -1},
    {"Doesn't match Content Pack", "Foobar", []GenericGame{gg("Foobar Content Pack")}, -1},
    {"Doesn't match Costume Pack", "Foobar", []GenericGame{gg("Foobar Costume Pack")}, -1},
    {"Doesn't match Season Pass", "Foobar", []GenericGame{gg("Foobar Season Pass")}, -1},
    {"Doesn't match Demo", "Foobar", []GenericGame{gg("Foobar Demo")}, -1},

    {"Doesn't match a game with 0 price (demo)", "Foobar", []GenericGame{GenericGame{"Foobar", float32(0.0), "/some_path"}}, -1},

    {"Ignores PC for direct matching", "Foobar", []GenericGame{gg("Foobar 2"), gg("Foobar PC")}, 1},

    {"Matches Deluxe", "Foobar", []GenericGame{gg("Foobar Deluxe")}, 0},

    {"Prefers base game rather than Deluxe", "Foobar", []GenericGame{gg("Foobar"), gg("Foobar Deluxe")}, 0},
    {"Prefers base game rather than DLC", "Foobar", []GenericGame{gg("Foobar - extra content"), gg("Foobar")}, 1},
  }

  for _, tc := range(tt) {
    t.Run(tc.name, func(t *testing.T) {
      output := BestMatch(tc.game, tc.results)
      if output != tc.expected {
        t.Errorf("Expected %d but got %d", tc.expected, output)
        return
      }
    })
  }
}
