package main

import (
  "encoding/json"
  "io"
  "fmt"
  "net/http"
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

type inStockInfo struct {
  Default bool
}

type loadedGame struct {
  Name loadedGameName
  Url loadedGameUrl
  Price loadedGamePrice
  Platforms loadedGamePlatforms
  // in_stock is either 1 or {"default": false}.
  // Unmarshal will skip the field if it's not appropriate.
  InStock int `json:"in_stock"`
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
  if debugFlag {
    fmt.Printf("[Loaded] Got full response: %+v\n", string(body))
  }
  var parsedResp loadedSearchResponse
  json.Unmarshal(body, &parsedResp)
  if debugFlag {
    fmt.Printf("[Loaded] Got (parsed) response: %+v\n", parsedResp)
  }

  if len(parsedResp.Results) == 0 || len(parsedResp.Results[0].Hits) == 0 {
    if debugFlag {
      fmt.Printf("[Loaded] No match for %s\n", game.name)
    }
    return nil
  }

  // Collect the names.
  results := []GenericGame{}
  for _, hit := range(parsedResp.Results[0].Hits) {
    if hit.Platforms.Default != "Steam" {
      if debugFlag {
        fmt.Printf("[Loaded] Ignoring non-steam game: %+v\n", hit)
      }
      continue
    }
    if hit.InStock != 1 {
      if debugFlag {
        fmt.Printf("[Loaded] Ignoring game not-in-stock: %+v\n", hit)
      }
      continue
    }
    results = append(results, GenericGame{hit.Name.Default, hit.Price.USD.Default, hit.Url.Default})
  }

  bestResultIdx := BestMatch(game.name, results)
  if bestResultIdx == -1 {
      if debugFlag {
        fmt.Printf("[Loaded] No matching game for %s\n", game.name)
      }
      return nil
  }

  if debugFlag {
    fmt.Printf("[Loaded] Best hit: %+v\n", results[bestResultIdx])
  }

  game.loaded.price = results[bestResultIdx].price
  game.loaded.url = results[bestResultIdx].url
  return nil
}
