package main

import (
  "encoding/json"
  "io"
  "fmt"
  "net/http"
  "os"
  "strings"
)

const (
  cLoadedSearchURL string = "https://muvyib7tey-dsn.algolia.net/1/indexes/*/queries?x-algolia-agent=Algolia%20for%20JavaScript%20(3.35.1)%3B%20Browser%3B%20instantsearch.js%20(4.7.2)%3B%20Magento2%20integration%20(3.10.5)%3B%20JS%20Helper%20(3.2.2)&x-algolia-application-id=MUVYIB7TEY&x-algolia-api-key=ODNjY2VjZjExZGE2NTg3ZDkyMGQ4MjljYzYwM2U0NmRjYWI4MDgwNTQ0NjgzNmE2ZGQyY2ZmMDlkMzAyYTI4NXRhZ0ZpbHRlcnM9"
)

type loadedGamePlatforms struct {
  Default string
}

type loadedGameUsdPrice struct {
  Default float32
}

type loadedGamePrice struct {
  // We ignore other currencies.
  USD loadedGameUsdPrice
}

type loadedGameUrl struct {
  Default string
}

type loadedGameName struct {
  Default string
}

type loadedGame struct {
  Name loadedGameName
  Url loadedGameUrl
  Price loadedGamePrice
  Platforms loadedGamePlatforms
}

type loadedResults struct {
  Hits []loadedGame
}

type loadedSearchResponse struct {
  Results []loadedResults
}

func FillLoadedInfo(game *Game) error {
  // We send a JSON payload as application/x-www-form-urlencoded to the search endpoint.
  buf := fmt.Sprintf("{\"requests\":[{\"indexName\":\"magento2_default_products\",\"params\":\"hitsPerPage=5&query=%s\"}]}", game.name)
  reader := strings.NewReader(buf)
  resp, err := http.Post(cLoadedSearchURL, "application/x-www-form-urlencoded", reader)
  if err != nil {
    return err
  }

  defer resp.Body.Close()
  body, err := io.ReadAll(resp.Body)
  if err != nil {
    return err
  }
  var parsedResp loadedSearchResponse
  json.Unmarshal(body, &parsedResp)
  if debugFlag {
    fmt.Printf("Got (parsed) Loaded response: %+v\n", parsedResp)
  }

  if len(parsedResp.Results) == 0 || len(parsedResp.Results[0].Hits) == 0 {
    if debugFlag {
      fmt.Printf("No Loaded match for %s\n", game.name)
    }
    return nil
  }

  // We take the first result as it's the most relevant.
  // TODO: Should I filter this like we do for steam?
  hit := parsedResp.Results[0].Hits[0]
  // Loaded tends to append "PC" to the name so look for a prefix match.
  if !strings.HasPrefix(strings.ToLower(hit.Name.Default), strings.ToLower(game.name)) {
    if debugFlag {
      fmt.Fprintf(os.Stderr, "Loaded returned the wrong game for \"%s\" (hit=%+v)\n", game.name, hit)
    }
    return nil
  }
  game.loaded.price = hit.Price.USD.Default
  game.loaded.url = hit.Url.Default
  return nil
}
