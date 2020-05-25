package main

import (
  "bufio"
  "errors"
  "fmt"
  "flag"
  "io"
  "net/http"
  "os"
  "strconv"
  "strings"
  "sync"

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

func steamAppURL(id int) string {
  return fmt.Sprintf("https://store.steampowered.com/app/%d", id)
}

func parseSearchResults(gameName string, reader io.Reader) (*game, error) {
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

          isParsingName = false
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
          if game.id == 0 || game.name == "" {
            panic("Couldn't parse " + gameName)
          }

          if game.price == 0 {
            return nil, errors.New("Game is missing price (is it out yet?)")
          }

          return &game, nil
        }
    }
  }
}

func fetchGame(gameName string, c chan *game, wg *sync.WaitGroup) {
  defer wg.Done()
  // Steam uses '+' as delimiters for word in their calls.
  searchURL := fmt.Sprintf(cSearchURLMissingKeyword, strings.Join(strings.Split(gameName, " "), "+"))
  resp, err := http.Get(searchURL)
  if err != nil {
    fmt.Fprintf(os.Stderr, "Error fetching game \"%s\" (err = %+v)\n", gameName, err)
    return
  }

  defer resp.Body.Close()
  game, err := parseSearchResults(gameName, resp.Body)
  if err != nil {
    fmt.Fprintf(os.Stderr, "Error fetching game \"%s\" (err = %+v)\n", gameName, err)
    return
  }
  c <- game
  if debug {
    fmt.Println("Done for", gameName)
  }
}

func splitGamesOnCriteria(c chan *game) ([]*game, []*game) {
  // Simple price point right now.
  var matchingGames []*game
  var otherGames []*game

  for game := range c {
    if game.price < 7 {
      matchingGames = append(matchingGames, game)
      continue
    }

    otherGames = append(otherGames, game)
  }

  return matchingGames, otherGames
}

var debug bool

func main() {
  flag.BoolVar(&debug, "debug", false, "Enable debug statements")
  flag.Parse()

  args := flag.Args()
  if len(args) != 1 {
    fmt.Printf("Usage: main file.txt\n\n\nFile contains one game name per line\n")
    return
  }

  file, err := os.Open(args[0])
  if err != nil {
    fmt.Fprintf(os.Stderr, "Couldn't open file for reading %s (err = %+v)", args[0], err)
    return
  }
  defer file.Close()

  scanner := bufio.NewScanner(file)
  gameNames := make([]string, 10)
  for scanner.Scan() {
    gameNames = append(gameNames, scanner.Text())
  }

  if err := scanner.Err(); err != nil {
    fmt.Fprintf(os.Stderr, "Error reading file = %s", args[0])
    return
  }

  var wg sync.WaitGroup
  wg.Add(len(gameNames))
  c := make(chan *game, len(gameNames))
  for _, gameName := range gameNames {
    if debug {
      fmt.Println("Fetching", gameName)
    }
    go fetchGame(gameName, c, &wg)
  }

  wg.Wait()
  close(c)
  if debug {
    fmt.Println("Done fetching!")
  }

  matchingGames, otherGames := splitGamesOnCriteria(c)

  fmt.Fprintf(os.Stdout, "==================================================\n")
  fmt.Fprintf(os.Stdout, "============ Matching games ======================\n")
  fmt.Fprintf(os.Stdout, "==================================================\n")
  for _, game := range matchingGames {
    fmt.Fprintf(os.Stdout, "%s: %.2f - %s\n", game.name, game.price, steamAppURL(game.id))
  }
  fmt.Fprintf(os.Stdout, "\n\n==================================================\n")

  for _, game := range otherGames {
    fmt.Fprintf(os.Stdout, "%s: %.2f - %s\n", game.name, game.price, steamAppURL(game.id))
  }
  fmt.Fprintf(os.Stdout, "==================================================\n")
}
