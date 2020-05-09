package main

import (
  "errors"
  "fmt"
  "io"
  "net/http"
  "os"
  "strconv"
  "strings"

  "golang.org/x/net/html"
)

const cGameIdAttr string = "data-ds-appid"
const cGameClassNameAttr string = "match_name"
const cGameClassPriceAttr string = "match_price"
const cSearchURLMissingKeyword string = "https://store.steampowered.com/search/suggest?term=%s&f=games&cc=US"

type game struct {
  id int
  name string
  price float32
}

func parseSearchResults(reader io.Reader) (*game, error) {
  // Steam results are formatted as follows:
  // * Each result (game) is an anchor
  // * Under each anchor, there is an image and the price.

  // We don't use html.Parse as it just generates the extra
  // tags mandated by the HTML5 page (<body>, ...).
  var game game

  isParsingPrice := false
  isParsingName := false
  tokenizer := html.NewTokenizer(reader)
  for {
    tt := tokenizer.Next()
    switch tt {
      case html.ErrorToken:
        return nil, tokenizer.Err()
      case html.TextToken:
        if isParsingPrice {
          // We drop the first letter as it is the price.
          priceStr := string(tokenizer.Text())[1:]
          price, err := strconv.ParseFloat(priceStr, /*bitSize=*/32)
          if err != nil {
            return nil, errors.New("Couldn't convert text to price (" + priceStr + ")")
          }
          game.price = float32(price)

          isParsingPrice = false
        }
        if isParsingName {
          // We drop the first letter as it is the price.
          game.name = string(tokenizer.Text())

          isParsingPrice = false
        }
      case html.StartTagToken:
        tn, _ := tokenizer.TagName()
        tagName := string(tn)
        switch tagName {
          case "a":
            // Start of a game entry.
            // We are looking for the attribute with the appId
            for {
              attrName, attrValue, more := tokenizer.TagAttr();
              if string(attrName) == cGameIdAttr {
                var err error
                game.id, err = strconv.Atoi(string(attrValue))
                if err != nil {
                  return nil, errors.New("Couldn't convert attribute to id (" + string(attrValue) + ")")
                }
                break
              }
              if more == false {
                break
              }
            }
            break
          case "div":
            for {
              attrName, attrValue, more := tokenizer.TagAttr();
              if string(attrName) == "class" {
                attrValueStr := string(attrValue)
                if attrValueStr == cGameClassPriceAttr {
                  isParsingPrice = true
                  break
                }
                if attrValueStr == cGameClassNameAttr {
                  isParsingName = true
                  break
                }
              }
              if more == false {
                break
              }
            }
        break
        }
      case html.EndTagToken:
        tn, _ := tokenizer.TagName()
        tagName := string(tn)
        if tagName == "a" {
          // We only care about the first result.

          if game.id == 0 || game.price == 0 {
            return nil, errors.New("Game couldn't be parsed!")
          }
          return &game, nil
        }
    }
  }
}

func fetchGame(keywords []string) (*game, error) {
  searchURL := fmt.Sprintf(cSearchURLMissingKeyword, strings.Join(keywords, "+"))
  resp, err := http.Get(searchURL)
  if err != nil {
    fmt.Printf("Error fetching keywords \"%s\" (err = %+v)\n", strings.Join(keywords, " "), err)
    return nil, err
  }

  defer resp.Body.Close()
  return parseSearchResults(resp.Body)
}

func main() {
  if len(os.Args) == 1 {
    fmt.Printf("Usage: %s file.txt\n\n\nFile contains one game name per line", os.Args[0])
    return
  }

  keywords := os.Args[1:]
  game, err := fetchGame(keywords)
  if err != nil {
    fmt.Printf("Error fetching keywords \"%s\" (err = %+v)\n", strings.Join(keywords, " "), err)
  }

  fmt.Printf("Game = %+v\n", game)
}
