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

// States for our parser.
const (
  lookingForGame = iota
  inGameLookingForName = iota
  inGameParsingName = iota
  inGameLookingForPrice = iota
  inGameParsingPrice = iota
  // Used when done parsing to reset back on the end anchor tag.
  lookingForEndOfCurrentGame = iota
)

type game struct {
  id int
  name string

  // Game price may be -1 if none was found (unreleased games).
  price float32
  targetPrice float32
}

func steamAppURL(id int) string {
  return fmt.Sprintf("https://store.steampowered.com/app/%d", id)
}

func parseSearchResult(gameName string, reader io.Reader) (error, []game) {
  // Steam results are formatted as follows:
  // * Each result (game) is an anchor tag that contains the game id.
  // * Under each anchor, there is the name, an image and the price.
  // We ignore the image and extract the name and price.

  // We don't use html.Parse as it just generates the extra
  // tags mandated by the HTML5 page (<body>, ...).

  parsingState := lookingForGame

  games := []game{};
  parsedGame := game{0, "", -1, 0};

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
            if priceStr == "Free" {
              parsedGame.price = 0
            } else {
              // We drop the first letter as it is the price currency.
              priceStr := priceStr[1:]
              price, err := strconv.ParseFloat(priceStr, /*bitSize=*/32)
              if err != nil {
                return errors.New("Couldn't convert text to price (" + priceStr + ")"), games
              }
              parsedGame.price = float32(price)
            }

            parsingState = lookingForEndOfCurrentGame
            break
          case inGameParsingName:
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
              // TODO: Support bundle ID.
              // We can either store the bundle separately (URL: https://store.steampowered.com/bundle/%d) or store the URL.
              if string(attrName) == cGameIdAttr {
                var err error
                parsedGame.id, err = strconv.Atoi(string(attrValue))
                if err != nil {
                  return errors.New("Couldn't convert attribute to id (" + string(attrValue) + ")"), games
                }
                break
              }
              if more == false {
                break
              }
            }
            parsingState = inGameParsingName
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
          if parsedGame.id == 0 || parsedGame.name == "" {
            fmt.Fprintf(os.Stderr, "Dropping partially parsed game: %+v\n", parsedGame)
          } else {
            games = append(games, parsedGame)
          }

          parsedGame = game{0, "", -1, 0}
          parsingState = lookingForGame
        }
    }
  }
}

func selectBestMatchingGame(name string, games []game) *game {
  if len(games) == 0 {
    return nil
  }

  var bestMatchingGame *game = nil
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

    // Ignore demos.
    if game.price == 0 {
      continue
    }

    // We can't use |game| here as it is temporary variable.
    bestMatchingGame = &games[idx]

  }

  return bestMatchingGame
}

// TODO: Stop passing the full game as we populate multiple and select the one now.
func fetchAndFillGame(game *game) error {
  if debugFlag {
    fmt.Println("Fetching", game.name)
  }

  // Steam uses '+' as delimiters for word in their calls.
  searchURL := fmt.Sprintf(cSearchURLMissingKeyword, strings.Join(strings.Split(game.name, " "), "+"))
  resp, err := http.Get(searchURL)
  if err != nil {
    return err
  }
  defer resp.Body.Close()

  err, games := parseSearchResult(game.name, resp.Body)
  if err != nil {
    return err
  }

  // TODO: This is clunky, rework how we pass those arguments to allow a null return.
  bestGame := selectBestMatchingGame(game.name, games)
  if bestGame != nil {
    game.id = bestGame.id
    game.name = bestGame.name
    game.price = bestGame.price
  }

  if debugFlag {
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

    if debugFlag {
      fmt.Println("Done for", game.name)
    }
  }
}

func splitGameOnCriteria(game game, output *Output) {
  output.m.Lock()
  defer output.m.Unlock()

  if game.price == -1 {
    output.unreleasedGames = append(output.unreleasedGames, game)
    return
  }

  // Simple price point right now.
  if game.price < game.targetPrice {
    output.matchingGames = append(output.matchingGames, game)
    return
  }

  output.otherGames = append(output.otherGames, game)
}

var debugFlag bool
var gamesFlag string
var fileFlag string
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
  unreleasedGames []game
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
  return Output{[]game{}, []game{}, []game{}, sync.Mutex{}, sync.WaitGroup{}}
}

func feedGamesFromFile(fileName string, c chan game) error {
  file, err := os.Open(fileName)
  if err != nil {
    return err
  }

  go func() {
    // Ensure that we close c to avoid deadlocks in case of errors.
    defer close(c)
    defer file.Close()

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
        fmt.Fprintf(os.Stderr, "Error reading file = %s (err=%s)\n", fileName, err)
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
          fmt.Fprintf(os.Stderr, "Error reading file = %s, invalid price (err=%+v)\n", fileName, err)
          return
        }
        targetPrice = float32(tmp)
      }
      c <- game{/*id=*/0, gameName, /*price=*/0, targetPrice}
    }
  }()

  return nil
}

func feedGamesFromFlag(games string, c chan game) error {

  go func() {
    // Ensure that we close c to avoid deadlocks in case of errors.
    defer close(c)

    tokens := strings.Split(games, ",")
    idx := 0
    for ; idx < len(tokens); idx++ {
      gameName := tokens[idx];
      // Start with our default and override it if specified.
      targetPrice := cDefaultTargetPrice
      if idx < len(tokens) - 1 {
        lookAheadToken := tokens[idx + 1]
        tmp, err := strconv.ParseFloat(lookAheadToken, /*bitSize=*/32)
        if err == nil {
          targetPrice = float32(tmp)
          // Skip next token as it was an optional price.
          idx += 1
        }
      }
      c <- game{/*id=*/0, gameName, /*price=*/0, targetPrice}
    }
  }()

  return nil
}

func main() {
  flag.BoolVar(&debugFlag, "debug", false, "Enable debug statements")
  flag.StringVar(&gamesFlag, "games", "", "Commad separated list of games to fetch")
  flag.StringVar(&fileFlag, "file", "", "File containing a CSV list of games")
  flag.Parse()

  args := flag.Args()
  if gamesFlag == "" && fileFlag == "" || (gamesFlag != "" && fileFlag != "") {
    fmt.Printf("Usage: main [-debug] [-file <file>] [-games game1,7,game2,game3]\n\n\nEither -file or -games must be set, but not both.\n\n<file> contains one game name per line along with a potential target price divided by ','\nExample: Foobar, 10\n")
    return
  }

  c := make(chan game, parallelism)
  output := newOutput()
  output.wg.Add(parallelism)

  // Start the workers.
  for i := 0; i < parallelism; i++ {
      go gameWorker(c, &output)
  }

  // Feed the games as they are read.
  if (fileFlag != "") {
    err := feedGamesFromFile(fileFlag, c)
    if err != nil {
      fmt.Fprintf(os.Stderr, "Couldn't open file for reading %s (err = %+v)", args[0], err)
      return
    }
  } else {
    err := feedGamesFromFlag(gamesFlag, c)
    if err != nil {
      fmt.Fprintf(os.Stderr, "Couldn't open file for reading %s (err = %+v)", args[0], err)
      return
    }
  }

  output.wg.Wait()

  // Sort the output by price.
  sort.Sort(ByPrice(output.matchingGames))
  sort.Sort(ByPrice(output.otherGames))

  fmt.Fprintf(os.Stdout, "==================================================\n")
  fmt.Fprintf(os.Stdout, "============== Unreleased games ==================\n")
  fmt.Fprintf(os.Stdout, "==================================================\n")
  for _, game := range output.unreleasedGames {
    fmt.Fprintf(os.Stdout, "%s (target price = $%.2f) - %s \n", game.name, game.targetPrice, steamAppURL(game.id))
  }

  fmt.Fprintf(os.Stdout, "\n\n")
  fmt.Fprintf(os.Stdout, "==================================================\n")
  fmt.Fprintf(os.Stdout, "================= Matching games =================\n")
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
