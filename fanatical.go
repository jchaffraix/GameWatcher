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
  cFanaticalKeyURL string = "https://www.fanatical.com/api/algolia/key"
  cFanaticalSearchURLMissingKey string = "https://w2m9492ddv-dsn.algolia.net/1/indexes/fan_alt_rank/query?x-algolia-api-key=%s&x-algolia-application-id=W2M9492DDV"
  cFanaticalAnonId string = "deadbeef-8888-8888-8888-deadbeef88"
)

type fanaticalKeyResponse struct {
  Key string
  ValidUntil int
}

var fanaticalKey string = ""

func InitFanatical() error {
  req, err := http.NewRequest("GET", cFanaticalKeyURL, nil)
  if err != nil {
    return err
  }
  req.Header.Add("anonid", cFanaticalAnonId)
  client := &http.Client{}
  resp, err := client.Do(req)
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
  if fanaticalKey == "" {
    panic("Invalid search key for Fanatical")
  }

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
  searchURL := fmt.Sprintf(cFanaticalSearchURLMissingKey, fanaticalKey)
  if debugFlag {
    fmt.Printf("Fanatical search URL: \"%s\"\n", searchURL)
  }

  // We send a JSON payload as application/x-www-form-urlencoded to the search endpoint.
  buf := fmt.Sprintf("{\"query\":\"%s\",\"hitsPerPage\":5,\"filters\":\"\"}", game.name)
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
    fmt.Printf("Got (parsed) Fanatical response: %+v\n", parsedResp)
  }

  if len(parsedResp.Hits) == 0 {
    if debugFlag {
      fmt.Printf("No Fanatical match for %s\n", game.name)
    }
    return nil
  }

  // We take the first result as it's the most relevant.
  // TODO: Should I filter this like we do for steam?
  hit := parsedResp.Hits[0]
  // If fanatical doesn't find a game, it will still include some games whose name is close to the query.
  if !strings.EqualFold(hit.Name, game.name) {
    if debugFlag {
      fmt.Fprintf(os.Stderr, "Fanatical returned the wrong game for \"%s\" (hit=%+v)\n", game.name, hit)
    }
    return nil
  }
  game.fanatical.price = hit.Price.USD
  game.fanatical.slug = hit.Slug
  return nil
}
