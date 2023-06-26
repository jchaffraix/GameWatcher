package main

import (
  "encoding/csv"
  "fmt"
  "flag"
  "io"
  "os"
  "sort"
  "strconv"
  "strings"
  "sync"
)

// TODO: All those common structs need to go to a shared file.
type SteamInfo struct {
  // Either id (regular game) or bundleId (for bundles) will be ever set.
  id int
  bundleId int

  // Game price may be -1 if none was found (unreleased games).
  price float32
}

type FanaticalInfo struct {
  // Game price may be -1 if none was found (unreleased games).
  price float32

  // Slug is what is used to build the fanatical URL.
  // Empty if the game is unreleased or inexistent.
  slug string
}

type Game struct {
  name string

  minPrice float32
  backend string

  // Backend-specific info.
  steam SteamInfo
  fanatical FanaticalInfo
}

func (g Game) url() string {
  if g.backend == "steam" {
    return g.steamURL()
  } else {
    if g.backend != "fanatical" {
      panic(fmt.Sprintf("Unknown backend \"%s\"", g.backend))
    }

    return g.fanaticalURL()
  }
}

func (g Game) steamURL() string {
  if g.steam.bundleId != 0 && g.steam.id != 0 {
    panic(fmt.Sprintf("Game is both a regular game and a bundle: %+v", g))
  }

  if g.steam.bundleId != 0 {
    return fmt.Sprintf("https://store.steampowered.com/bundle/%d", g.steam.bundleId)
  }

  return fmt.Sprintf("https://store.steampowered.com/app/%d", g.steam.id)
}

func (g Game) fanaticalURL() string {
  if g.fanatical.slug == "" {
    panic(fmt.Sprintf("Game doesn't have a fanatical slug: %+v", g))
  }

  return fmt.Sprintf("https://www.fanatical.com/en/game/%s", g.fanatical.slug)
}

type gameCriteria struct {
  name string
  targetPrice float32
}

func fetchAndFillGame(criteria gameCriteria) (error, *Game) {
  if debugFlag {
    fmt.Println("Fetching", criteria.name)
  }

  err, game := SearchGameOnSteam(criteria.name)
  if err != nil {
    return err, nil
  }

  err = FillFanaticalInfo(game)
  if err != nil {
    return err, nil
  }

  if debugFlag {
    fmt.Println("Done Fetching", criteria.name)
  }
  return nil, game
}

func gameWorker(c chan gameCriteria, output *Output) {
  defer output.wg.Done()
  for criteria := range(c) {
    err, game := fetchAndFillGame(criteria)
    if err != nil {
      fmt.Fprintf(os.Stderr, "Error fetching game \"%s\" (err = %+v)\n", criteria.name, err)
      continue
    }

    if game == nil {
      fmt.Fprintf(os.Stderr, "No matches for \"%s\" (err = %+v)\n", criteria.name)
      continue
    }

    // Fill min price.
    if game.steam.price <= game.fanatical.price {
      game.minPrice = game.steam.price
      game.backend = "steam"
    } else {
      game.minPrice = game.fanatical.price
      game.backend = "fanatical"
    }

    splitGameOnCriteria(*game, criteria.targetPrice, output)

    if debugFlag {
      fmt.Println("Done for", criteria.name)
    }
  }
}

func splitGameOnCriteria(game Game, targetPrice float32, output *Output) {
  output.m.Lock()
  defer output.m.Unlock()

  if game.steam.price == -1 {
    output.unreleasedGames = append(output.unreleasedGames, game)
    return
  }

  // Simple price point right now.
  if game.minPrice < targetPrice {
    if debugFlag {
      fmt.Fprintf(os.Stdout, "Game \"%s\" with price = %v (backend = \"%s\") matched targetPrice = %v\n", game.name, game.minPrice, game.backend, targetPrice)
    }
    output.matchingGames = append(output.matchingGames, game)
    return
  }

  if debugFlag {
    fmt.Fprintf(os.Stdout, "Game \"%s\" with price = %v (backend = \"%s\") was over targetPrice = %v\n", game.name, game.minPrice, game.backend, targetPrice)
  }
  output.otherGames = append(output.otherGames, game)
}

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
  unreleasedGames []Game
  matchingGames []Game
  otherGames []Game
  m sync.Mutex

  wg sync.WaitGroup
}

// ByPriceThenName implements sort.Interface for []game.
type ByPriceThenName []Game
func (a ByPriceThenName) Len() int { return len(a) }
func (a ByPriceThenName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByPriceThenName) Less(i, j int) bool {
  iPrice := a[i].minPrice
  jPrice := a[j].minPrice
  if (iPrice < jPrice) {
    return true;
  }

  if (iPrice > jPrice) {
    return false;
  }

  return a[i].name < a[j].name
}

func newOutput() Output {
  return Output{[]Game{}, []Game{}, []Game{}, sync.Mutex{}, sync.WaitGroup{}}
}

func feedGamesFromFile(fileName string, c chan gameCriteria) error {
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
      c <- gameCriteria{gameName, targetPrice}
    }
  }()

  return nil
}

func feedGamesFromFlag(games string, c chan gameCriteria) error {

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
      c <- gameCriteria{gameName, targetPrice}
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

  InitFanatical()

  c := make(chan gameCriteria, parallelism)
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

  // Sort the output by price, then name.
  // This gives a stable sort for quickly assessing games.
  sort.Sort(ByPriceThenName(output.unreleasedGames))
  sort.Sort(ByPriceThenName(output.matchingGames))
  sort.Sort(ByPriceThenName(output.otherGames))

  if len(output.unreleasedGames) > 0 {
    fmt.Fprintf(os.Stdout, "==================================================\n")
    fmt.Fprintf(os.Stdout, "============== Unreleased games ==================\n")
    fmt.Fprintf(os.Stdout, "==================================================\n")
    for _, game := range output.unreleasedGames {
      fmt.Fprintf(os.Stdout, "%s - %s \n", game.name, game.steamURL())
    }
    fmt.Fprintf(os.Stdout, "\n\n")
  }

  if len(output.matchingGames) > 0 {
    fmt.Fprintf(os.Stdout, "==================================================\n")
    fmt.Fprintf(os.Stdout, "============== Games under target ================\n")
    fmt.Fprintf(os.Stdout, "==================================================\n")
    for _, game := range output.matchingGames {
      fmt.Fprintf(os.Stdout, "%s: $%.2f - %s \n", game.name, game.minPrice, game.url())
    }
    fmt.Fprintf(os.Stdout, "\n\n")
  }

  fmt.Fprintf(os.Stdout, "==================================================\n")
  fmt.Fprintf(os.Stdout, "=============== Games over target ================\n")
  fmt.Fprintf(os.Stdout, "==================================================\n")
  for _, game := range output.otherGames {
    fmt.Fprintf(os.Stdout, "%s: $%.2f - %s\n", game.name, game.minPrice, game.url())
  }
  fmt.Fprintf(os.Stdout, "==================================================\n")
}
