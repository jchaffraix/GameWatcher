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

func fetchGame(gameName string) (*game, error) {
  if debug {
    fmt.Println("Fetching", gameName)
  }

  // Steam uses '+' as delimiters for word in their calls.
  searchURL := fmt.Sprintf(cSearchURLMissingKeyword, strings.Join(strings.Split(gameName, " "), "+"))
  resp, err := http.Get(searchURL)
  if err != nil {
    return nil, err
  }
  defer resp.Body.Close()
  game, err := parseSearchResults(gameName, resp.Body)
  if err != nil {
    return nil, err
  }

  if debug {
    fmt.Println("Fetched", gameName)
  }
  return game, nil
}

func gameWorker(c chan string, output *Output) {
  defer output.wg.Done()
  for gameName := range(c) {
    game, err := fetchGame(gameName)
    if err != nil {
      fmt.Fprintf(os.Stderr, "Error fetching game \"%s\" (err = %+v)\n", gameName, err)
      continue
    }

    splitGameOnCriteria(game, output)

    if debug {
      fmt.Println("Done for", gameName)
    }
  }
}

func splitGameOnCriteria(game * game, output *Output) {
  output.m.Lock()
  defer output.m.Unlock()
  // Simple price point right now.
  if game.price < 7 {
    output.matchingGames = append(output.matchingGames, game)
    return
  }

  output.otherGames = append(output.otherGames, game)
}

var debug bool
var parallelism int = 10

type Output struct {
  // Channels don't work here as we read from
  // one of the channels at a time, leading to
  // deadlocks (main thread is waiting on new input
  // on one channel when all the worker threads are
  // waiting for their write to be acknoweldged
  // on the other channels). We *could* create
  // some buffer channels but that would be pretty
  // equivalent to this as they would have to be
  // sized after the total number of games to fetch.
  //
  // TODO: Think about this more. Maybe we can
  // figure out how to use a single output channel
  // (maybe by annotating the game struct?).
  matchingGames []*game
  otherGames []*game
  m sync.Mutex

  wg sync.WaitGroup
}

func newOutput() Output {
  return Output{[]*game{}, []*game{}, sync.Mutex{}, sync.WaitGroup{}}
}

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

  c := make(chan string, parallelism)
  output := newOutput()
  output.wg.Add(parallelism)

  // Start the workers.
  for i := 0; i < parallelism; i++ {
      go gameWorker(c, &output)
  }

  // Feed the games as they are read.
  go func() {
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
      c <- scanner.Text()
    }
    close(c)

    if err := scanner.Err(); err != nil {
      fmt.Fprintf(os.Stderr, "Error reading file = %s", args[0])
    }
  }()

  output.wg.Wait()

  fmt.Fprintf(os.Stdout, "==================================================\n")
  fmt.Fprintf(os.Stdout, "============ Matching games ======================\n")
  fmt.Fprintf(os.Stdout, "==================================================\n")
  for _, game := range output.matchingGames {
    fmt.Fprintf(os.Stdout, "%s: %.2f - %s\n", game.name, game.price, steamAppURL(game.id))
  }
  fmt.Fprintf(os.Stdout, "\n\n==================================================\n")

  for _, game := range output.otherGames {
    fmt.Fprintf(os.Stdout, "%s: %.2f - %s\n", game.name, game.price, steamAppURL(game.id))
  }
  fmt.Fprintf(os.Stdout, "==================================================\n")
}
