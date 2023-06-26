package main

import (
  "encoding/json"
  "io"
  "fmt"
  "net/http"
  "strings"
)

const (
  cFanaticalKeyURL string = "https://www.fanatical.com/api/algolia/key"
  cFanaticalSearchURLMissingKey string = "https://w2m9492ddv-dsn.algolia.net/1/indexes/fan_alt_rank/query?x-algolia-api-key=%s&x-algolia-application-id=W2M9492DDV"
)

type fanaticalKeyResponse struct {
  Key string
  ValidUntil int
}

var fanaticalKey string = ""

func InitFanatical() error {
  resp, err := http.Get(cFanaticalKeyURL)
  if err != nil {
    return err
  }
  defer resp.Body.Close()
  body, err := io.ReadAll(resp.Body)
  if err != nil {
    return err
  }
  var parsedResp fanaticalKeyResponse
  err = json.Unmarshal(body, &parsedResp)
  if err != nil {
    return err
  }

  // Note: We ignore the ValidUntil field as we should process all entries within the lifetime of the key.
  fanaticalKey = parsedResp.Key
  return nil
}

type fanaticalPrice struct {
  // We ignore other currencies.
  USD float32
}

type fanaticalGame struct {
  Name string
  Price fanaticalPrice
  Slug string
}

type fanaticalSearchResponse struct {
  Hits []fanaticalGame
}

func FillFanaticalInfo(game *Game) error {
  // Ignore unreleased games.
  if game.steam.id == 0 && game.steam.bundleId == 0 {
    if debugFlag {
      fmt.Println("Ignoring unreleased game (Fanatical)")
    }
    return nil
  }

  searchURL := fmt.Sprintf(cFanaticalSearchURLMissingKey, fanaticalKey)
  if debugFlag {
    fmt.Printf("Fanatical search URL: %s\n", searchURL)
  }

  // We send a JSON payload as application/x-www-form-urlencoded to the search endpoint.
  buf := fmt.Sprintf("{\"query\":\"%s\",\"hitsPerPage\":5,\"filters\":\"\"}", game.name)
  fmt.Printf("Buff : %s", buf)
  reader := strings.NewReader(buf)
  resp, err := http.Post(searchURL, "application/x-www-form-urlencoded", reader)
  if err != nil {
    return err
  }

  defer resp.Body.Close()
  body, err := io.ReadAll(resp.Body)
  if err != nil {
    return err
  }
  var parsedResp fanaticalSearchResponse
  json.Unmarshal(body, &parsedResp)
  if debugFlag {
    fmt.Printf("Got Fanatical response: %+v\n", parsedResp)
  }

  if len(parsedResp.Hits) == 0 {
    if debugFlag {
      fmt.Printf("Not match in Fanatical for %s\n", game.name)
    }
    return nil
  }

  // We take the first one as it's the most relevant.
  // TODO: Should I filter this like steam?
  game.fanatical.price = parsedResp.Hits[0].Price.USD
  game.fanatical.slug = parsedResp.Hits[0].Slug
  return nil
}