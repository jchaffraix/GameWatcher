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
  parsedGame := Game{"", -1, "", SteamInfo{0, 0, -1}, FanaticalInfo{-1, ""}, GreenManGamingInfo{-1, ""}, HumbleBundleInfo{-1, ""}, LoadedInfo{-1, ""}};

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
              parsedGame.steam.price = 0
            } else {
              // We drop the first letter as it is the currency.
              priceStr := priceStr[1:]
              price, err := strconv.ParseFloat(priceStr, /*bitSize=*/32)
              if err != nil {
                fmt.Fprintf(os.Stderr, "Couldn't convert text to price (" + priceStr + ")\n")
              } else {
                parsedGame.steam.price = float32(price)
              }
            }

            parsingState = lookingForEndOfCurrentGame
            break
          case inGameParsingName:
            if parsedGame.steam.bundleId != 0 {
              panic("Parsing bundle as regular game!")
            }
            parsedGame.name = string(tokenizer.Text())
            parsingState = inGameLookingForPrice
            break
          case inGameParsingBundleName:
            if parsedGame.steam.id != 0 {
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
                  parsedGame.steam.id = parsedId
                  parsingState = inGameParsingName
                } else {
                  parsedGame.steam.bundleId = parsedId
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
          if (parsedGame.steam.id == 0 && parsedGame.steam.bundleId == 0) || parsedGame.name == "" {
            fmt.Fprintf(os.Stderr, "Dropping partially parsed game: %+v\n", parsedGame)
          } else {
            games = append(games, parsedGame)
          }

          parsedGame = Game{"", -1, "", SteamInfo{0, 0, -1}, FanaticalInfo{-1, ""}, GreenManGamingInfo{-1, ""}, HumbleBundleInfo{-1, ""}, LoadedInfo{-1, ""}};
          parsingState = lookingForGame
        }
    }
  }
}

func selectBestMatchingGame(name string, games []Game) *Game {
  // TODO: Remove this conversion...
  ggames := []GenericGame{}
  for _, game := range(games) {
    ggames = append(ggames, GenericGame{game.name, game.steam.price, ""})
  }
  bestMatchingGameIdx := BestMatch(name, ggames)
  if bestMatchingGameIdx == -1 {
    return nil
  }

  return &games[bestMatchingGameIdx]
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

  if len(games) == 0 {
    panic(fmt.Sprintf("Couldn't find a matching game for %s (did you mistype the name?)", name))
  }

  return nil, selectBestMatchingGame(name, games)
}

