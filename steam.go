// TODO: This should be a in a `steam` package.
package main

import (
  "errors"
  "io"
  "fmt"
  "net/http"
  "os"
  "strconv"
  "strings"

  "golang.org/x/net/html"
)

const (
  cGameIdAttr string = "data-ds-appid"
  cGameBundleIdAttr string = "data-ds-bundleid"
  cGameClassNameAttr string = "match_name"
  cGameClassPriceAttr string = "match_price"
  cSteamSearchURLMissingKeyword string = "https://store.steampowered.com/search/suggest?term=%s&f=games&cc=US"

  cDefaultTargetPrice float32 = 7
)

// States for our parser.
const (
  lookingForGame = iota
  inGameLookingForName = iota
  inGameParsingName = iota
  inGameParsingBundleName = iota
  inGameLookingForPrice = iota
  inGameParsingPrice = iota
  // Used when done parsing to reset back on the end anchor tag.
  lookingForEndOfCurrentGame = iota
)

func parseSearchResult(gameName string, reader io.Reader) (error, []Game) {
  // Steam results are formatted as follows:
  // * Each result (game) is an anchor tag that contains the game id.
  // * Under each anchor, there is the name, an image and the price.
  // We ignore the image and extract the name and price.

  // We don't use html.Parse as it just generates the extra
  // tags mandated by the HTML5 page (<body>, ...).

  parsingState := lookingForGame

  games := []Game{};
  parsedGame := Game{0, 0, "", -1};

  tokenizer := html.NewTokenizer(reader)
  for {
    tt := tokenizer.Next()

    switch tt {
      case html.ErrorToken:
        err := tokenizer.Err()
        // Check that this is a real error.
        if err != io.EOF {
          return err, games
        }

        return nil, games

      case html.TextToken:
        if debugFlag {
          fmt.Fprintf(os.Stdout, "Intext, parsingState = %+v\n", parsingState)
        }
        switch parsingState {
          case inGameParsingPrice:
            priceStr := string(tokenizer.Text())
            if strings.Contains(priceStr, "Free") {
              parsedGame.price = 0
            } else {
              // We drop the first letter as it is the currency.
              priceStr := priceStr[1:]
              price, err := strconv.ParseFloat(priceStr, /*bitSize=*/32)
              if err != nil {
                fmt.Fprintf(os.Stderr, "Couldn't convert text to price (" + priceStr + ")\n")
              } else {
                parsedGame.price = float32(price)
              }
            }

            parsingState = lookingForEndOfCurrentGame
            break
          case inGameParsingName:
            if parsedGame.bundleId != 0 {
              panic("Parsing bundle as regular game!")
            }
            parsedGame.name = string(tokenizer.Text())
            parsingState = inGameLookingForPrice
            break
          case inGameParsingBundleName:
            if parsedGame.id != 0 {
              panic("Parsing regular game as bundle!")
            }
            parsedGame.name = string(tokenizer.Text())
            parsingState = inGameLookingForPrice
            break
        }
        break
      case html.StartTagToken:
        tn, _ := tokenizer.TagName()
        tagName := string(tn)
        switch tagName {
          case "a":
            // Start of a game entry.
            // We are looking for the attribute with the appId
            if debugFlag {
              fmt.Fprintf(os.Stdout, "Start of anchor, parsingState = %+v\n", parsingState)
            }
            for {
              attrName, attrValue, more := tokenizer.TagAttr();
              if string(attrName) == cGameIdAttr || string(attrName) == cGameBundleIdAttr {
                idStr := string(attrValue)
                var err error
                parsedId, err := strconv.Atoi(idStr)
                if err != nil {
                  return errors.New("Couldn't convert attribute to id (" + idStr + ")"), games
                }
                if string(attrName) == cGameIdAttr {
                  parsedGame.id = parsedId
                  parsingState = inGameParsingName
                } else {
                  parsedGame.bundleId = parsedId
                  parsingState = inGameParsingBundleName
                }

                break
              }
              if more == false {
                break
              }
            }
            break
          case "div":
            if debugFlag {
              fmt.Fprintf(os.Stdout, "Start of div, parsingState = %+v\n", parsingState)
            }
            for {
              attrName, attrValue, more := tokenizer.TagAttr();
              oldParsingState := parsingState
              if string(attrName) == "class" {
                attrValueStr := string(attrValue)
                switch parsingState {
                  case inGameLookingForPrice:
                    if attrValueStr == cGameClassPriceAttr {
                      parsingState = inGameParsingPrice
                    }
                    break
                  case inGameLookingForName:
                    if attrValueStr == cGameClassNameAttr {
                      parsingState = inGameParsingName
                    }
                    break
                }
              }

              if oldParsingState != parsingState || more == false {
                break
              }
            }
        break
        }
      case html.EndTagToken:
        tn, _ := tokenizer.TagName()
        tagName := string(tn)
        if tagName == "a" {
          if debugFlag {
            fmt.Fprintf(os.Stdout, "End of parsing game, got: %+v\n", parsedGame)
          }
          if (parsedGame.id == 0 && parsedGame.bundleId == 0) || parsedGame.name == "" {
            fmt.Fprintf(os.Stderr, "Dropping partially parsed game: %+v\n", parsedGame)
          } else {
            games = append(games, parsedGame)
          }

          parsedGame = Game{0, 0, "", -1}
          parsingState = lookingForGame
        }
    }
  }
}

func selectBestMatchingGame(name string, games []Game) *Game {
  if len(games) == 0 {
    return nil
  }

  var bestMatchingGame *Game = nil
  for idx, game := range(games) {
    // If this is a direct match for the name, stop.
    // This prevent unrelated matches to show up, especially for unreleased games.
    if strings.ToLower(game.name) == strings.ToLower(name) {
      // We can't use |game| here as it is temporary variable.
      bestMatchingGame = &games[idx]
      break
    }

    if strings.Contains(game.name, "Soundtrack") || strings.Contains(game.name, "OST") {
      continue
    }

    if strings.Contains(game.name, "Artbook") {
      continue
    }

    if strings.Contains(game.name, "Adventure Pack") || strings.Contains(game.name, "Season Pass") {
      continue
    }

    // Ignore demos.
    if game.price == 0 {
      continue
    }

    // We select the shortest name as it removes all the DLC.
    if bestMatchingGame != nil && len(game.name) >= len(bestMatchingGame.name) {
      continue
    }

    // We can't use |game| here as it is temporary variable.
    bestMatchingGame = &games[idx]
  }

  return bestMatchingGame
}


func SearchGameOnSteam(name string) (error, *Game) {
  // Steam uses '+' as delimiter for words in their URL.
  searchURL := fmt.Sprintf(cSteamSearchURLMissingKeyword, strings.Join(strings.Split(name, " "), "+"))
  resp, err := http.Get(searchURL)
  if err != nil {
    return err, nil
  }
  defer resp.Body.Close()

  err, games := parseSearchResult(name, resp.Body)
  if err != nil {
    return err, nil
  }

  return nil, selectBestMatchingGame(name, games)
}

