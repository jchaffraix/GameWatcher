package main

import (
  "strings"
)

type GenericGame struct {
  name string
  price float32
  // Generic string: can be relative or absolute.
  url string
}

func shouldIgnore(game GenericGame) bool {
  if strings.Contains(game.name, "DLC") {
    return true
  }

  if strings.Contains(game.name, "Soundtrack") || strings.Contains(game.name, "OST") {
    return true
  }

  if strings.Contains(game.name, "Artbook") {
    return true
  }

  if strings.Contains(game.name, "Adventure Pack") || strings.Contains(game.name, "Content Pack") || strings.Contains(game.name, "Costume Pack") {
    return true
  }

  if strings.Contains(game.name, "Season Pass") {
    return true
  }

  if strings.Contains(game.name, " Demo") || game.price == 0 {
    return true
  }

  return false
}

// Remove some keywords and lowers the string.
func normalizeResult(result string) string {
  normalized := result
  before, after, found := strings.Cut(normalized, " PC")
  if found {
    normalized = before + strings.TrimSpace(after)
  }

  before, after, found = strings.Cut(normalized, " Deluxe")
  if found {
    normalized = before + strings.TrimSpace(after)
  }

  return strings.ToLower(normalized)
}

// Returns a number from 0.0 (no match) to 1.0 (perfect match) to represent
// the potential of the current result.
func score(name, result string) float32 {
  normalized := normalizeResult(result)
  if strings.ToLower(name) == normalized {
    // Direct match.
    return 1.0
  }

  // We should allow some looser comparison here that:
  // 1. Ignore punctionations (e.g. dashes, colons, ...)
  // 2. Ignore non-ascii characters (e.g. TM, ...)

  // TODO: Make this smarter :)
  return 0.0
}

func BestMatch(game string, results []GenericGame) int {
  if len(results) == 0 {
    return -1
  }

  bestScore := float32(0.0)
  bestMatch := -1
  for i, result := range(results) {
    if shouldIgnore(result) {
      continue
    }
    score := score(game, result.name)
    if score > bestScore {
      bestScore = score
      bestMatch = i
    }
  }
  return bestMatch
}
