package main

import (
  "errors"
  "encoding/csv"
  "fmt"
  "flag"
  "io"
  "net/http"
  "os"
  "sort"
  "strconv"
  "strings"
  "sync"

  "golang.org/x/net/html"
)

const (
  cGameIdAttr string = "data-ds-appid"
  cGameClassNameAttr string = "match_name"
  cGameClassPriceAttr string = "match_price"
  cSearchURLMissingKeyword string = "https://store.steampowered.com/search/suggest?term=%s&f=games&cc=US"

  cDefaultTargetPrice float32 = 7
)

type game struct {
  id int
  name string
  price float32
  targetPrice float32
}

func steamAppURL(id int) string {
  return fmt.Sprintf("https://store.steampowered.com/app/%d", id)
}

func parseSearchResultsAndFillGame(game *game, reader io.Reader) error {
  // Steam results are formatted as follows:
  // * Each result (game) is an anchor
  // * Under each anchor, there is an image and the price.

  // We don't use html.Parse as it just generates the extra
  // tags mandated by the HTML5 page (<body>, ...).

  // TODO: Switch to a real state machine.
  isParsingPrice := false
  isParsingName := false
  hasParsedName := false
  tokenizer := html.NewTokenizer(reader)
  for {
    tt := tokenizer.Next()
    switch tt {
      case html.ErrorToken:
        return tokenizer.Err()
      case html.TextToken:
        if isParsingPrice {
          // We drop the first letter as it is the price.
          priceStr := string(tokenizer.Text())[1:]
          price, err := strconv.ParseFloat(priceStr, /*bitSize=*/32)
          if err != nil {
            return errors.New("Couldn't convert text to price (" + priceStr + ")")
          }
          game.price = float32(price)

          isParsingPrice = false
        }
        if isParsingName {
          // We drop the first letter as it is the price.

          // We override the name so that we present an accurate summary
          // at the end.
          //
          // Ideally we would validate that the names are related but it
          // is hard as Steam does some smart matching.
          // TODO: Add this validation.
          game.name =string(tokenizer.Text())

          isParsingName = false
          hasParsedName = true
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
                  return errors.New("Couldn't convert attribute to id (" + string(attrValue) + ")")
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
          if game.id == 0 || !hasParsedName {
            panic("Couldn't parse " + game.name)
          }

          if game.price == 0 {
            return errors.New("Game is missing price (is it out yet?)")
          }

          return nil
        }
    }
  }
}

func fetchAndFillGame(game *game) error {
  if debug {
    fmt.Println("Fetching", game.name)
  }

  // Steam uses '+' as delimiters for word in their calls.
  searchURL := fmt.Sprintf(cSearchURLMissingKeyword, strings.Join(strings.Split(game.name, " "), "+"))
  resp, err := http.Get(searchURL)
  if err != nil {
    return err
  }
  defer resp.Body.Close()

  err = parseSearchResultsAndFillGame(game, resp.Body)
  if err != nil {
    return err
  }

  if debug {
    fmt.Println("Fetched", game.name)
  }
  return nil
}

func gameWorker(c chan game, output *Output) {
  defer output.wg.Done()
  for game := range(c) {
    err := fetchAndFillGame(&game)
    if err != nil {
      fmt.Fprintf(os.Stderr, "Error fetching game \"%s\" (err = %+v)\n", game.name, err)
      continue
    }

    splitGameOnCriteria(game, output)

    if debug {
      fmt.Println("Done for", game.name)
    }
  }
}

func splitGameOnCriteria(game game, output *Output) {
  output.m.Lock()
  defer output.m.Unlock()
  // Simple price point right now.
  if game.price < game.targetPrice {
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
  matchingGames []game
  otherGames []game
  m sync.Mutex

  wg sync.WaitGroup
}

// ByPrice implements sort.Interface for []game.
type ByPrice []game
func (a ByPrice) Len() int { return len(a) }
func (a ByPrice) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByPrice) Less(i, j int) bool { return a[i].price < a[j].price }


func newOutput() Output {
  return Output{[]game{}, []game{}, sync.Mutex{}, sync.WaitGroup{}}
}

func main() {
  flag.BoolVar(&debug, "debug", false, "Enable debug statements")
  flag.Parse()

  args := flag.Args()
  if len(args) != 1 {
    fmt.Printf("Usage: main file.txt\n\n\nFile contains one game name per line along with a potential target price divided by ','\nExample: Foobar, 10\n")
    return
  }

  file, err := os.Open(args[0])
  if err != nil {
    fmt.Fprintf(os.Stderr, "Couldn't open file for reading %s (err = %+v)", args[0], err)
    return
  }
  defer file.Close()

  c := make(chan game, parallelism)
  output := newOutput()
  output.wg.Add(parallelism)

  // Start the workers.
  for i := 0; i < parallelism; i++ {
      go gameWorker(c, &output)
  }

  // Feed the games as they are read.
  go func() {
    // Ensure that we close c to avoid deadlocks in case of errors.
    defer close(c)

    csvReader := csv.NewReader(file)
    for {
      records, err := csvReader.Read()
      // Handle EOF as a special error.
      if err == io.EOF {
        return
      }

      // We want to allow an optional targetPrice.
      // This means that we ignore ErrFieldCount errors by looking at the presence of `records`.
      if err != nil && records == nil {
        fmt.Fprintf(os.Stderr, "Error reading file = %s (err=%s)\n", args[0], err)
        return
      }
      if len(records) == 0 {
        panic("Invalid CSV file, no record on line")
      }
      gameName := records[0]
      // Start with our default and override it if specified.
      targetPrice := cDefaultTargetPrice
      if len(records) == 2 {
        tmp, err := strconv.ParseFloat(strings.TrimSpace(records[1]), /*bitSize=*/32)
        if err != nil {
          fmt.Fprintf(os.Stderr, "Error reading file = %s, invalid price (err=%+v)\n", args[0], err)
          return
        }
        targetPrice = float32(tmp)
      }
      c <- game{/*id=*/0, gameName, /*price=*/0, targetPrice}
    }
  }()

  output.wg.Wait()

  // Sort the output by price.
  sort.Sort(ByPrice(output.matchingGames))
  sort.Sort(ByPrice(output.otherGames))

  fmt.Fprintf(os.Stdout, "==================================================\n")
  fmt.Fprintf(os.Stdout, "============ Matching games ======================\n")
  fmt.Fprintf(os.Stdout, "==================================================\n")
  for _, game := range output.matchingGames {
    fmt.Fprintf(os.Stdout, "%s: $%.2f (target price = $%.2f) - %s \n", game.name, game.price, game.targetPrice, steamAppURL(game.id))
  }
  fmt.Fprintf(os.Stdout, "\n\n==================================================\n")

  for _, game := range output.otherGames {
    fmt.Fprintf(os.Stdout, "%s: $%.2f (target price = $%.2f) - %s\n", game.name, game.price, game.targetPrice, steamAppURL(game.id))
  }
  fmt.Fprintf(os.Stdout, "==================================================\n")
}
