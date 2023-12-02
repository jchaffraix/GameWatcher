package main

import (
  "encoding/json"
  "io"
  "fmt"
  "net/http"
  "net/url"
  "os"
  "strconv"
  "strings"
)

const (
  // The key is returned in https://www.humblebundle.com/ (in a <script type="application/json"> that is removed after loading).
  cHBApiKey string = "5229f8b3dec4b8ad265ad17ead42cb7f"
  cHBSearchURLMissingKey string = "https://ayszewdaz2-dsn.algolia.net/1/indexes/replica_product_query_site_search/query?x-algolia-application-id=AYSZEWDAZ2&x-algolia-api-key=%s"
)

type hbSearchPricing struct {
  // List of supported countries.
  // The first element in the array is the price.
  // The second one is the currency.
  US []string
}

type hbSearchHit struct {
  Name string `json:"human_name"`

  Path string `json:"link"`
 
  // Array that contains the different methods, must contain "steam".
  Delivery []string `json:"delivery_methods"`

  Pricing hbSearchPricing `json:"current_pricing"`
}


type hbSearchResponse struct {
  Hits []hbSearchHit
}

func hasSteamDelivery(hit hbSearchHit) bool {
  for _, delivery := range(hit.Delivery) {
    if delivery == "steam" {
      return true
    }
  }
  return false
}

func FillHumbleBundleInfo(game *Game) error {
  searchURL := fmt.Sprintf(cHBSearchURLMissingKey, cHBApiKey)
  if debugFlag {
    fmt.Printf("HumbleBundle search URL: \"%s\"\n", searchURL)
  }
  // The query is the JSON object send as application/x-www-form-urlencoded
  // url.QueryEscape replaces spaces with '+', which is not what we want for a POST.
  query := strings.ReplaceAll(url.QueryEscape(game.name), "+", "%20")
  buf := fmt.Sprintf("{\"params\":\"query=%s&hitsPerPage=5&page=0\"}", query)

  if debugFlag {
    fmt.Printf("HumbleBundle payload: \"%s\"\n", buf)
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
  var parsedResp hbSearchResponse
  json.Unmarshal(body, &parsedResp)
  if debugFlag {
    fmt.Printf("Got (parsed) HumbleBundle response: %+v\n", parsedResp)
  }

  if len(parsedResp.Hits) == 0 {
    if debugFlag {
      fmt.Printf("No HumbleBundle match for %s\n", game.name)
    }
    return nil
  }

  for _, hit := range(parsedResp.Hits) {
    if strings.EqualFold(hit.Name, game.name) {
      if !hasSteamDelivery(hit) {
        if debugFlag {
          fmt.Printf("Ignoring hit for \"%s\" with a steam delivery, full hit: %+v\n", game.name, hit)
        }
        continue
      }
      price, err := strconv.ParseFloat(hit.Pricing.US[0], 32)
      if err != nil {
        fmt.Fprintf(os.Stderr, "Invalid price for \"%s\", full hit: %+v\n", game.name, hit)
        return nil
      }

      game.hb.price = float32(price)
      game.hb.path = hit.Path
      return nil
    }
  }
 
  if debugFlag {
    fmt.Printf("Did not find a HumbleBundle match for \"%s\"\n", game.name)
  }

  return nil
}
