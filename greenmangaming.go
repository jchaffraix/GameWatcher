package main

import (
  "encoding/json"
  "io"
  "fmt"
  "net/http"
  "net/url"
  "os"
  "strings"
  "strconv"
)

const (
  // The key is returned in https://www.greenmangaming.com/en/Modals/AlgoliaSearchModal but it seems hard-coded.
  cGMGApiKey string = "3bc4cebab2aa8cddab9e9a3cfad5aef3"
  cGMGSearchURLMissingKey string = "https://sczizsp09z-dsn.algolia.net/1/indexes/*/queries?x-algolia-api-key=%s&x-algolia-application-id=SCZIZSP09Z"
)

type gmgRegionDetails struct {
  Price float32 `json:"Drp"`
  DiscountPercentage float32 `json:"DrpDiscountPercentage"`

  // IsOnSale
  // ReleaseDateStatus
}

type gmgRegion struct {
  US gmgRegionDetails
}

type gmgSearchHit struct {
  DisplayName string
  SteamAppId string
  Url string
  Regions gmgRegion

  // OutOfStockRegions
}

type gmgSearchResult struct {
  Hits []gmgSearchHit 
}

type gmgSearchResponse struct {
  Results []gmgSearchResult
}

func FillGreenManGamingInfo(game *Game) error {
  searchURL := fmt.Sprintf(cGMGSearchURLMissingKey, cGMGApiKey)
  if debugFlag {
    fmt.Printf("GreenManGaming search URL: \"%s\"\n", searchURL)
  }

  // The query is the JSON object send as application/x-www-form-urlencoded
  // url.QueryEscape replaces spaces with '+', which is not what we want for a POST.
  query := strings.ReplaceAll(url.QueryEscape(game.name), "+", "%20")
  buf := fmt.Sprintf("{\"requests\":[{\"indexName\":\"prod_ProductSearch_US\",\"params\":\"query=%s\"}]}", query)

  if debugFlag {
    fmt.Printf("GreenManGaming payload: \"%s\"\n", buf)
  }

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
  var parsedResp gmgSearchResponse
  json.Unmarshal(body, &parsedResp)
  if debugFlag {
    fmt.Printf("Got (parsed) GreenManGaming response: %+v\n", parsedResp)
  }

  if len(parsedResp.Results) != 1 {
    fmt.Fprintf(os.Stderr, "GreenManGaming returned unexpected numbers of result(s) for \"%s\", parsed response: %+v\n", game.name, parsedResp)
    // TODO: We should return an error.
    return nil
  }

  if len(parsedResp.Results[0].Hits) == 0 {
    if debugFlag {
      fmt.Printf("No GreenManGaming match for %s\n", game.name)
    }
    return nil
  }

  for _, hit := range(parsedResp.Results[0].Hits) {
    if hit.SteamAppId == "BUNDLE" {
      continue
    }

    steamAppId, err := strconv.Atoi(strings.TrimSpace(hit.SteamAppId))
    if err != nil {
      fmt.Fprintf(os.Stderr, "[GreenManGaming] Invalid SteamAppId for \"%s\", full hit: %+v\n", game.name, hit)
      continue
    }

    // TODO: Handle bundle.
    if steamAppId == game.steam.id {
      // TODO: Handle out of stock games.
      game.gmg.price = hit.Regions.US.Price
      game.gmg.path = hit.Url
      return nil
    }
  }

  if debugFlag {
    fmt.Printf("Did not find a GreenManGaming match for \"%s\"\n", game.name)
  }
  return nil
}
